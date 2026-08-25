// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package-internal benchmarks for the type-expression renderer.
//
// [renderState.renderType] is unexported and reachable from outside
// the package only through [Backend.Render], where template
// execution, gofmt, and the goimports pass together cost three
// orders of magnitude more. Measuring it at all therefore requires
// the internal test package; the behavioural coverage stays
// blackbox in render_type_test.go.
package backend

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/writer"
)

// BenchmarkRenderType measures one type-expression render per
// operation, one Ref kind per sub-benchmark.
//
// renderType is the single hottest function in the backend by call
// count: every field, parameter, return, type argument, and
// constraint term in every generated file routes through it, so a
// per-call regression multiplies by the size of the emit graph
// rather than by the number of files. The existing
// BenchmarkBackend_Render cannot see that — a renderType that got
// twice as slow moves the whole-pass number by noise.
//
// Deliberately outside the timed region: [loadTemplates] and the
// per-Target [template.Template.Clone] that [newRenderState]
// performs. Neither is reachable from renderType — the state here is
// built directly with only the [writer.ImportSet] the function
// actually touches, so the numbers carry no template-tree cost at
// all.
//
// The import set is reused across iterations rather than rebuilt.
// [writer.ImportSet.Imp] is idempotent per path and every
// sub-benchmark uses a fixed path set, so the set reaches its final
// size on the first iteration and the reported allocations are
// renderType's own rather than the fixture's growth.
//
// A zero-alloc report is the correct answer for the "builtin" and
// "internal same-package" cases: both return a stored string
// unchanged, and Go's runtime returns the original backing array
// when concatenating with an empty string. Every other case builds
// a new string and must report non-zero allocations; a zero there
// means the loop body was eliminated.
func BenchmarkRenderType(b *testing.B) {
	b.ReportAllocs()

	// A same-package target carries no resolved ImportPath, so the
	// TypeRef branch returns the bare name; the cross-package twin
	// carries one and therefore pays for an Imp call plus the alias
	// qualification.
	samePackage := &emit.Struct{Name: "Inner", Package: "example.com/bench"}
	crossPackage := &emit.Struct{
		Name:    "SearcherMock",
		Package: "example.com/bench/store",
		Target: emit.Target{
			Dir: "bench/store", Filename: "store_mock.go",
			Package: "store", ImportPath: "example.com/bench/store",
		},
	}

	cases := []struct {
		name string
		ref  emit.Ref
	}{
		{"builtin", emit.Builtin("string")},
		{"external stdlib", emit.External("context", "Context")},
		{"external dot-joined name", emit.External("github.com/example/users", "User.Profile")},
		{"internal same-package", emit.Internal(samePackage)},
		{"internal cross-package", emit.Internal(crossPackage)},
		{"composite pointer", emit.Ptr(emit.Builtin("int"))},
		{"composite slice", emit.SliceOf(emit.Builtin("byte"))},
		{"composite array", emit.ArrayOf(emit.Builtin("byte"), 16)},
		{"composite map", emit.MapOf(emit.Builtin("string"), emit.Builtin("int"))},
		{
			name: "composite func",
			ref: emit.FuncOf(
				[]emit.Ref{emit.Builtin("int"), emit.Builtin("string")},
				[]emit.Ref{emit.Builtin("int"), emit.Builtin("error")},
			),
		},
		{
			name: "composite union",
			ref: emit.Union(
				emit.UnionTerm{Type: emit.Builtin("int"), Approx: true},
				emit.UnionTerm{Type: emit.Builtin("float64"), Approx: true},
				emit.UnionTerm{Type: emit.Builtin("string")},
			),
		},
		{
			name: "composite nested map of slice of pointer",
			ref: emit.MapOf(
				emit.Builtin("string"),
				emit.SliceOf(emit.Ptr(emit.External("github.com/example/users", "User"))),
			),
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			s := benchRenderState()
			var got string
			for b.Loop() {
				var err error
				got, err = s.renderType(tc.ref)
				if err != nil {
					b.Fatalf("renderType: %v", err)
				}
			}
			if got == "" {
				b.Fatalf("renderType produced no spelling for %s", tc.name)
			}
		})
	}
}

// BenchmarkRenderType_Depth measures one render of a pointer chain
// per operation, over chains of increasing depth.
//
// Composite refs recurse: each level concatenates its own prefix
// onto the fully rendered element below it, so a chain of depth n
// copies O(n²) bytes in total. Nesting that deep does not come from
// hand-written Go, but it does come from generated emit graphs
// where a plugin wraps a type once per capability, and the whole
// chain renders on every field that uses it.
//
// The scaling axis is what makes the number readable: linear growth
// in the reported ns/op would mean the concatenation is being
// absorbed somewhere, and superlinear growth quantifies exactly how
// much depth the renderer tolerates before the quadratic term
// dominates.
//
// Chain construction is hoisted per size, outside the timed region;
// only the render is measured.
func BenchmarkRenderType_Depth(b *testing.B) {
	b.ReportAllocs()

	for _, depth := range []int{1, 10, 100, 1000} {
		b.Run(strconv.Itoa(depth), func(b *testing.B) {
			b.ReportAllocs()
			s := benchRenderState()
			ref := pointerChain(depth)
			var got string
			for b.Loop() {
				var err error
				got, err = s.renderType(ref)
				if err != nil {
					b.Fatalf("renderType: %v", err)
				}
			}
			if len(got) != depth+len("int") {
				b.Fatalf("depth %d rendered %d chars, want %d", depth, len(got), depth+len("int"))
			}
		})
	}
}

// benchRenderState builds the minimal [renderState] renderType
// reads: an import set for the External and cross-package TypeRef
// branches. The template tree is deliberately absent — renderType
// and everything it dispatches to render into strings without ever
// executing a template, so cloning one would only add setup cost
// the benchmark then has to explain away.
func benchRenderState() *renderState {
	return &renderState{imports: writer.NewImportSet(nil)}
}

// pointerChain returns a Ref nesting depth pointer levels over a
// builtin `int`, so the rendered spelling is depth asterisks
// followed by "int". Used by [BenchmarkRenderType_Depth] to vary
// recursion depth without varying anything else about the ref.
func pointerChain(depth int) emit.Ref {
	var ref emit.Ref = emit.Builtin("int")
	for range depth {
		ref = emit.Ptr(ref)
	}
	return ref
}

// chanRef builds the reference shape the Go frontend produces for a
// channel: a named ref in the synthetic `go` package with the
// element as its single type argument, and the real facts stamped as
// meta.
func chanRef(t *testing.T, dir string, elem emit.Ref) emit.Ref {
	t.Helper()
	origin := &node.TypeRef{TypeKind: node.TypeRefNamed, Package: "go", Name: "chan"}
	goIsChannelKey.SetAt(origin.EnsureMeta(), true, meta.AuthorityPlugin, "golang", origin.Pos())
	if dir != "" {
		goChanDirKey.SetAt(origin.EnsureMeta(), dir, meta.AuthorityPlugin, "golang", origin.Pos())
	}
	ref := emit.External("go", "chan", elem)
	ref.OriginNode = origin
	return ref
}

// TestRenderType_Channel pins channel rendering.
//
// The frontend models a channel as a named ref in a package called
// `go`, with direction in meta. Unhandled, that fell through to the
// external-reference path and produced `import "go"` — a path that
// resolves against the standard library — plus a `go.chan[T]`
// qualifier that does not parse. The direction was lost too, so even
// with the import fixed a receive-only channel became bidirectional
// and the generated type no longer satisfied the interface.
func TestRenderType_Channel(t *testing.T) {
	t.Parallel()

	render := func(t *testing.T, dir string, elem emit.Ref) (string, error) {
		t.Helper()
		s := newRenderState(loadTemplates(), nil, nil, nil)
		return s.renderType(chanRef(t, dir, elem))
	}

	for name, tc := range map[string]struct{ dir, want string }{
		"bidirectional":                      {"both", "chan string"},
		"send-only":                          {"send", "chan<- string"},
		"receive-only":                       {"recv", "<-chan string"},
		"missing direction is bidirectional": {"", "chan string"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := render(t, tc.dir, emit.Builtin("string"))
			if err != nil {
				t.Fatalf("renderType: %v", err)
			}
			if got != tc.want {
				t.Fatalf("renderType = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("the element registers its own import", func(t *testing.T) {
		t.Parallel()
		// The property a string-shaped workaround could not have: a
		// builtin ref carrying "<-chan pkg.T" is a leaf and would
		// leave the import unregistered.
		s := newRenderState(loadTemplates(), nil, nil, nil)
		got, err := s.renderType(chanRef(t, "recv", emit.External("example.com/w", "Value")))
		if err != nil {
			t.Fatalf("renderType: %v", err)
		}
		if got != "<-chan w.Value" {
			t.Fatalf("renderType = %q, want <-chan w.Value", got)
		}
		if s.imports.Len() != 1 {
			t.Fatalf("element import not registered; set holds %d", s.imports.Len())
		}
	})

	t.Run("a channel with no element is reported", func(t *testing.T) {
		t.Parallel()
		// A ref claiming go.isChannel without the structure is a
		// plugin bug; naming it beats rendering "chan " and letting
		// the formatter report a syntax error with no attribution.
		origin := &node.TypeRef{TypeKind: node.TypeRefNamed, Package: "go", Name: "chan"}
		goIsChannelKey.SetAt(origin.EnsureMeta(), true, meta.AuthorityPlugin, "golang", origin.Pos())
		ref := emit.External("go", "chan")
		ref.OriginNode = origin

		s := newRenderState(loadTemplates(), nil, nil, nil)
		if _, err := s.renderType(ref); !errors.Is(err, ErrUnsupportedRef) {
			t.Fatalf("err = %v, want ErrUnsupportedRef", err)
		}
	})
}

// refRenderComposite is a concatenating reference implementation of
// the composite spelling — the shape every arm had before the append
// pass landed.
//
// Kept so the buffered path can be checked against the behaviour it
// claims to preserve rather than against a restatement of its own
// rules. Do not tidy it; its value is that it is the old code.
func refRenderComposite(s *renderState, r *emit.CompositeRef) (string, error) {
	switch r.Shape {
	case emit.ShapePointer:
		elem, err := s.renderType(r.Elem)
		if err != nil {
			return "", err
		}
		return "*" + elem, nil
	case emit.ShapeSlice:
		elem, err := s.renderType(r.Elem)
		if err != nil {
			return "", err
		}
		return "[]" + elem, nil
	case emit.ShapeArray:
		elem, err := s.renderType(r.Elem)
		if err != nil {
			return "", err
		}
		return "[" + strconv.Itoa(r.ArrayLen) + "]" + elem, nil
	case emit.ShapeMap:
		key, err := s.renderType(r.MapKey)
		if err != nil {
			return "", err
		}
		val, err := s.renderType(r.MapValue)
		if err != nil {
			return "", err
		}
		return "map[" + key + "]" + val, nil
	case emit.ShapeFunc:
		paramParts := make([]string, 0, len(r.FuncParams))
		for _, p := range r.FuncParams {
			t, err := s.renderType(p)
			if err != nil {
				return "", err
			}
			paramParts = append(paramParts, t)
		}
		retText, err := s.renderReturns(emit.AnonReturns(r.FuncReturns...))
		if err != nil {
			return "", err
		}
		out := "func(" + strings.Join(paramParts, ", ") + ")"
		if retText != "" {
			out += " " + retText
		}
		return out, nil
	case emit.ShapeUnion:
		parts := make([]string, 0, len(r.UnionTerms))
		for _, t := range r.UnionTerms {
			rendered, err := s.renderType(t.Type)
			if err != nil {
				return "", err
			}
			if t.Approx {
				rendered = "~" + rendered
			}
			parts = append(parts, rendered)
		}
		return strings.Join(parts, " | "), nil
	default:
		return "", fmt.Errorf("%w: composite shape %s", ErrUnsupportedRef, r.Shape)
	}
}

// compositeCorpus returns the refs the differential covers: every
// shape, nested past the depth at which the buffered path engages,
// with external refs in positions whose visit order decides import
// aliasing.
func compositeCorpus() []*emit.CompositeRef {
	ext := func(path, name string) emit.Ref { return emit.External(path, name) }
	// Two packages sharing a last segment, so the second to be
	// imported takes a collision suffix. Which one that is depends
	// entirely on visit order.
	a, b := ext("example.com/one/users", "A"), ext("example.com/two/users", "B")

	return []*emit.CompositeRef{
		emit.Ptr(emit.Builtin("int")),
		emit.SliceOf(emit.Ptr(a)),
		emit.ArrayOf(emit.Ptr(emit.Builtin("byte")), 16),
		emit.MapOf(a, b),
		// Non-colliding last segments, nested so the buffered path
		// is the one taken: both sides render with their own derived
		// alias regardless of visit order, so the spelling is
		// identical either way and only the import set's order can
		// reveal a transposed traversal.
		emit.MapOf(emit.Ptr(ext("example.com/alpha", "A")), emit.Ptr(ext("example.com/beta", "B"))),
		emit.MapOf(emit.Ptr(a), emit.SliceOf(emit.Ptr(b))),
		emit.MapOf(emit.Builtin("string"), emit.SliceOf(emit.Ptr(emit.MapOf(a, b)))),
		emit.FuncOf([]emit.Ref{a, emit.Builtin("string")}, []emit.Ref{b, emit.Builtin("error")}),
		emit.FuncOf(nil, nil),
		emit.FuncOf([]emit.Ref{emit.Builtin("int")}, []emit.Ref{emit.Builtin("error")}),
		emit.SliceOf(emit.FuncOf([]emit.Ref{a}, []emit.Ref{b})),
		emit.Union(
			emit.UnionTerm{Type: emit.Builtin("int")},
			emit.UnionTerm{Type: emit.Builtin("int64"), Approx: true},
			emit.UnionTerm{Type: a},
		),
	}
}

// TestRenderComposite_MatchesConcatenation is the differential on the
// append pass.
//
// Two assertions, and the second is the one that matters. Equal
// strings prove the bytes did not move. Equal import sets prove the
// traversal order did not: the external and cross-package arms call
// ImportSet.Imp, collision suffixes are handed out in first-import
// order, and a path that visited a map value before its key would
// produce identical text while silently swapping `users` and
// `users2` in any file importing two packages that share a last
// segment. No string comparison can see that.
// TestAppendAnonStruct_Spelling pins the renderer's own output for an
// inline anonymous struct, before format.Source sees it.
//
// The end-to-end tests in render_type_test.go assert on the finalised
// body, which means gofmt has already normalised the spelling —
// `struct{  }` and `struct{}` are indistinguishable there, and so is
// any other whitespace slip. Everything this renderer decides that
// the formatter would launder has to be pinned at this level or not
// at all.
//
// The empty case is the one that matters most: `map[K]struct{}` is
// how Go spells a set, which is what made this the most-hit of the
// type-rendering gaps.
func TestAppendAnonStruct_Spelling(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		fields []emit.AnonField
		embeds []emit.Ref
		want   string
	}{
		{
			name: "empty renders with no interior space",
			want: "struct{}",
		},
		{
			name:   "one field",
			fields: []emit.AnonField{{Name: "A", Type: emit.Builtin("int")}},
			want:   "struct{ A int }",
		},
		{
			name: "two fields are semicolon-separated",
			fields: []emit.AnonField{
				{Name: "A", Type: emit.Builtin("int")},
				{Name: "B", Type: emit.Builtin("string")},
			},
			want: "struct{ A int; B string }",
		},
		{
			name:   "a tag is backtick-quoted after the type",
			fields: []emit.AnonField{{Name: "B", Type: emit.Builtin("string"), Tag: `json:"b"`}},
			want:   "struct{ B string `json:\"b\"` }",
		},
		{
			name:   "an embed carries no name",
			embeds: []emit.Ref{emit.Builtin("error")},
			want:   "struct{ error }",
		},
		{
			name:   "embeds follow fields, separated",
			fields: []emit.AnonField{{Name: "A", Type: emit.Builtin("int")}},
			embeds: []emit.Ref{emit.Builtin("error")},
			want:   "struct{ A int; error }",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newRenderState(loadTemplates(), nil, nil, nil)
			got, err := s.renderType(emit.AnonStructOf(tc.fields, tc.embeds))
			if err != nil {
				t.Fatalf("renderType: %v", err)
			}
			if got != tc.want {
				t.Fatalf("renderType = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("a field type registers its own import", func(t *testing.T) {
		t.Parallel()
		// The property a string-shaped workaround could not have — the
		// same one renderChan documents. A builtin ref carrying the
		// whole `struct{ T time.Time }` spelling is a leaf and would
		// leave `time` unregistered, producing a file that does not
		// compile.
		s := newRenderState(loadTemplates(), nil, nil, nil)
		got, err := s.renderType(emit.AnonStructOf(
			[]emit.AnonField{{Name: "T", Type: emit.External("time", "Time")}}, nil,
		))
		if err != nil {
			t.Fatalf("renderType: %v", err)
		}
		if got != "struct{ T time.Time }" {
			t.Fatalf("renderType = %q, want struct{ T time.Time }", got)
		}
		if s.imports.Len() != 1 {
			t.Fatalf("field-type import not registered; set holds %d", s.imports.Len())
		}
	})

	t.Run("an embedded type registers its own import", func(t *testing.T) {
		t.Parallel()
		s := newRenderState(loadTemplates(), nil, nil, nil)
		if _, err := s.renderType(emit.AnonStructOf(
			nil, []emit.Ref{emit.External("io", "Reader")},
		)); err != nil {
			t.Fatalf("renderType: %v", err)
		}
		if s.imports.Len() != 1 {
			t.Fatalf("embed import not registered; set holds %d", s.imports.Len())
		}
	})
}

func TestRenderComposite_MatchesConcatenation(t *testing.T) {
	t.Parallel()

	for i, ref := range compositeCorpus() {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			got := newRenderState(loadTemplates(), nil, nil, nil)
			gotText, gotErr := got.renderComposite(ref)

			want := newRenderState(loadTemplates(), nil, nil, nil)
			wantText, wantErr := refRenderComposite(want, ref)

			if (gotErr == nil) != (wantErr == nil) {
				t.Fatalf("error disagreement: got %v, reference %v", gotErr, wantErr)
			}
			if gotText != wantText {
				t.Fatalf("spelling = %q, reference = %q", gotText, wantText)
			}
			if g, w := got.imports.Imports(), want.imports.Imports(); !slices.Equal(g, w) {
				t.Fatalf("import set diverged:\ngot  %+v\nwant %+v", g, w)
			}
		})
	}
}

// TestRenderType_AllocationBudget enforces what
// BenchmarkRenderType_Depth records.
//
// The defect was allocations linear in nesting depth. The budget is
// therefore stated as a relationship, not an absolute: a
// thousand-deep chain must stay within a small constant of a
// hundred-deep one. Linear growth is the quadratic returning.
//
//nolint:paralleltest // testing.AllocsPerRun panics in a parallel test.
func TestRenderType_AllocationBudget(t *testing.T) {
	chain := func(depth int) emit.Ref {
		var r emit.Ref = emit.Builtin("int")
		for range depth {
			r = emit.Ptr(r)
		}
		return r
	}

	measure := func(depth int) float64 {
		s := newRenderState(loadTemplates(), nil, nil, nil)
		r := chain(depth)
		return testing.AllocsPerRun(20, func() {
			if _, err := s.renderType(r); err != nil {
				t.Fatalf("renderType: %v", err)
			}
		})
	}

	at100, at1000 := measure(100), measure(1000)
	if at1000 > at100+8 {
		t.Fatalf("depth 1000 allocated %v against depth 100's %v; "+
			"growth with depth means the per-level concatenation is back", at1000, at100)
	}

	// The shallow case's budget. A composite whose children are not
	// composites takes one concatenation, which is one allocation
	// and is the floor; routing it through the buffer costs two,
	// because the buffer escapes and the result copies out of it.
	// *T, []T and map[K]V are most of what real code declares, so a
	// split that stopped applying would be a regression on the
	// common path in exchange for a win on a pathological one.
	shallow := newRenderState(loadTemplates(), nil, nil, nil)
	ptr := emit.Ptr(emit.Builtin("int"))
	if got := testing.AllocsPerRun(20, func() {
		if _, err := shallow.renderType(ptr); err != nil {
			t.Fatalf("renderType: %v", err)
		}
	}); got > 1 {
		t.Fatalf("shallow composite allocated %v times, budget 1", got)
	}

	// The func shape's own budget: reaching renderReturns wrapped
	// every return type in a 176-byte emit.Return to match a
	// signature, which no budget of three can absorb.
	s := newRenderState(loadTemplates(), nil, nil, nil)
	fn := emit.FuncOf([]emit.Ref{emit.Builtin("int"), emit.Builtin("string")},
		[]emit.Ref{emit.Builtin("int"), emit.Builtin("error")})
	if got := testing.AllocsPerRun(20, func() {
		if _, err := s.renderType(fn); err != nil {
			t.Fatalf("renderType: %v", err)
		}
	}); got > 3 {
		t.Fatalf("func-type render allocated %v times, budget 3", got)
	}
}
