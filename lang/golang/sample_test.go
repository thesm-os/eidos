// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// TestSampleRefFor_Composites pins the two shapes a string-returning
// sample could never carry: a value written as a composite literal,
// which needs both the type and the braces.
func TestSampleRefFor_Composites(t *testing.T) {
	t.Parallel()

	t.Run("a struct samples its first settable field", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"example.com/x.Point": &node.Struct{
			Name: "Point", Package: "example.com/x",
			Fields: []*node.Field{{Name: "X", Type: builtinRef("int")}},
		}}
		s, a := golang.SampleRefFor(namedTypeRef("example.com/x", "Point"), "P", r)
		if !s.OK() || !a.OK() {
			t.Fatalf("SampleRefFor derived nothing for a resolvable struct")
		}
		if !s.Composite {
			t.Errorf("a struct renders as a composite literal, not a conversion")
		}
		if s.Text != "{X: 42}" || a.Text != "{X: 7}" {
			t.Errorf("Text = %q, %q; want the field set to distinct values", s.Text, a.Text)
		}
		if s.Ref == nil {
			t.Errorf("sample carries no ref, so the import would go unregistered")
		}
	})

	t.Run("a struct skips a field it cannot sample", func(t *testing.T) {
		t.Parallel()
		// Refusing on the first field would lose a sample a later one
		// supplies, and a struct leading with an opaque type is
		// ordinary rather than exceptional.
		r := mapResolver{"example.com/x.Mixed": &node.Struct{
			Name: "Mixed", Package: "example.com/x",
			Fields: []*node.Field{
				{Name: "Opaque", Type: namedTypeRef("example.com/y", "Absent")},
				{Name: "Code", Type: builtinRef("int")},
			},
		}}
		s, _ := golang.SampleRefFor(namedTypeRef("example.com/x", "Mixed"), "M", r)
		if s.Text != "{Code: 42}" {
			t.Errorf("Text = %q, want the first field that has a sample", s.Text)
		}
	})

	t.Run("an unexported field is never sampled", func(t *testing.T) {
		t.Parallel()
		// A composite literal cannot set it from another package, so a
		// sample naming one does not compile where it lands.
		r := mapResolver{"example.com/x.Hidden": &node.Struct{
			Name: "Hidden", Package: "example.com/x",
			Fields: []*node.Field{{Name: "secret", Type: builtinRef("int")}},
		}}
		if s, _ := golang.SampleRefFor(namedTypeRef("example.com/x", "Hidden"), "H", r); s.OK() {
			t.Errorf("SampleRefFor = %+v, want nothing derivable", s)
		}
	})

	t.Run("an array samples one element", func(t *testing.T) {
		t.Parallel()
		// Enough to distinguish it from the zero array, and short
		// enough to stay readable in the generated source.
		arr := &node.TypeRef{TypeKind: node.TypeRefArray, ArrayLen: 3, Elem: builtinRef("int")}
		s, a := golang.SampleRefFor(arr, "Buf", nil)
		if !s.OK() || !a.OK() {
			t.Fatalf("SampleRefFor derived nothing for an array of builtins")
		}
		if !s.Composite || s.Text != "{42}" || a.Text != "{7}" {
			t.Errorf("SampleRefFor = %+v / %+v, want one-element composites", s, a)
		}
	})

	t.Run("an array of an unsampleable element yields nothing", func(t *testing.T) {
		t.Parallel()
		arr := &node.TypeRef{
			TypeKind: node.TypeRefArray, ArrayLen: 2,
			Elem: namedTypeRef("example.com/y", "Absent"),
		}
		if s, _ := golang.SampleRefFor(arr, "Buf", mapResolver{}); s.OK() {
			t.Errorf("SampleRefFor = %+v, want nothing derivable", s)
		}
	})

	t.Run("a nil ref and an exhausted budget yield nothing", func(t *testing.T) {
		t.Parallel()
		if s, _ := golang.SampleRefFor(nil, "F", nil); s.OK() {
			t.Errorf("a nil ref must derive nothing")
		}
		// A builtin the sample table has no entry for.
		if s, _ := golang.SampleRefFor(builtinRef("chan"), "C", nil); s.OK() {
			t.Errorf("a type admitting no distinguishable values must derive nothing")
		}
	})
}

// TestSampleRefFor_SliceAndMap covers the two composite kinds that
// derived nothing.
//
// The dispatch handled arrays and then bailed on anything not a named
// type, so every slice and every map fell through and returned the
// zero pair — for `[]byte` keys and `map[string]string` options, which
// are ordinary rather than exotic. A consumer cannot tell that from
// "this type has no sensible literal", so it drops the checks built
// from the value and, if it drops per argument rather than per check,
// the ones that never read it.
func TestSampleRefFor_SliceAndMap(t *testing.T) {
	t.Parallel()

	t.Run("a slice samples its element", func(t *testing.T) {
		t.Parallel()
		s, a := golang.SampleRefFor(sliceRef(builtinRef("string")), "key", nil)
		if !s.OK() || !a.OK() {
			t.Fatalf("SampleRefFor derived nothing for []string")
		}
		if s.Text != `{"test-key"}` || a.Text != `{"other-key"}` {
			t.Errorf("Text = %q, %q; want the element set to distinct values", s.Text, a.Text)
		}
		if !s.Composite {
			t.Errorf("a slice renders as a composite literal, not a conversion")
		}
	})

	t.Run("the pair differs in the element, not the length", func(t *testing.T) {
		t.Parallel()
		// A length-only difference is invisible to a subject that reads
		// the contents, which is the common shape.
		s, a := golang.SampleRefFor(sliceRef(builtinRef("int")), "n", nil)
		if s.Text == a.Text {
			t.Fatalf("both halves are %q; a check comparing them cannot fail", s.Text)
		}
		if len(s.Text) != len(a.Text) && (s.Text == "{}" || a.Text == "{}") {
			t.Errorf("the pair differs by emptiness rather than by element: %q, %q", s.Text, a.Text)
		}
	})

	t.Run("a map differs in the key", func(t *testing.T) {
		t.Parallel()
		// map[K]struct{} is how Go spells a set: an alternate differing
		// only in the value renders two identical literals for it.
		s, a := golang.SampleRefFor(mapRef(builtinRef("string"), builtinRef("int")), "opt", nil)
		if !s.OK() || !a.OK() {
			t.Fatalf("SampleRefFor derived nothing for map[string]int")
		}
		if s.Text != `{"test-opt": 42}` || a.Text != `{"other-opt": 42}` {
			t.Errorf("Text = %q, %q; want the key to carry the difference", s.Text, a.Text)
		}
	})

	t.Run("a slice of slices recurses", func(t *testing.T) {
		t.Parallel()
		s, a := golang.SampleRefFor(sliceRef(sliceRef(builtinRef("string"))), "key", nil)
		if s.Text != `{{"test-key"}}` || a.Text != `{{"other-key"}}` {
			t.Errorf("Text = %q, %q; want the nested element sampled", s.Text, a.Text)
		}
	})

	t.Run("an element that derives nothing yields nothing", func(t *testing.T) {
		t.Parallel()
		// `[]T{}` and `[]T{x}` are different claims and only the second
		// is a sample: a check built from an empty literal passes
		// against an implementation that reads no element at all.
		s, a := golang.SampleRefFor(sliceRef(namedTypeRef("example.com/y", "Absent")), "v", nil)
		if s.OK() || a.OK() {
			t.Fatalf("derived %q / %q for a slice whose element has no sample", s.Text, a.Text)
		}
	})

	t.Run("a map whose value derives nothing yields nothing", func(t *testing.T) {
		t.Parallel()
		s, _ := golang.SampleRefFor(
			mapRef(builtinRef("string"), namedTypeRef("example.com/y", "Absent")), "v", nil,
		)
		if s.OK() {
			t.Fatalf("derived %q for a map whose value has no sample", s.Text)
		}
	})

	t.Run("a map whose key derives nothing yields nothing", func(t *testing.T) {
		t.Parallel()
		s, _ := golang.SampleRefFor(
			mapRef(namedTypeRef("example.com/y", "Absent"), builtinRef("int")), "k", nil,
		)
		if s.OK() {
			t.Fatalf("derived %q for a map whose key has no sample", s.Text)
		}
	})

	t.Run("both carry a ref so the import is registered", func(t *testing.T) {
		t.Parallel()
		s, a := golang.SampleRefFor(sliceRef(builtinRef("byte")), "b", nil)
		if s.Ref == nil || a.Ref == nil {
			t.Errorf("a composite without a ref renders untyped braces and no import")
		}
	})
}

// TestZeroRefFor_Refusals covers every path on which a zero cannot be
// derived. Each returns false rather than an empty Sample a caller
// might render: a comparison against an undefined zero passes whatever
// the subject does, which is the assertion that never fails.
func TestZeroRefFor_Refusals(t *testing.T) {
	t.Parallel()

	t.Run("a nil type derives nothing", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.ZeroRefFor(nil, mapResolver{}); ok {
			t.Error("a nil type reported a zero")
		}
	})

	t.Run("a named type the resolver cannot reach derives nothing", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.ZeroRefFor(namedTypeRef("x", "Absent"), mapResolver{}); ok {
			t.Error("an unresolvable type reported a zero")
		}
	})

	t.Run("an alias with no target derives nothing", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"x.A": &node.Alias{Name: "A", Package: "x"}}
		if _, ok := golang.ZeroRefFor(namedTypeRef("x", "A"), r); ok {
			t.Error("an alias with no target reported a zero")
		}
	})

	t.Run("an alias over a type needing its own ref derives nothing", func(t *testing.T) {
		t.Parallel()
		// The inner zero is `x.S{}`, which already carries a ref. Wrapping
		// it in the alias's ref would spell one type and import another.
		r := mapResolver{
			"x.A": &node.Alias{Name: "A", Package: "x", Target: namedTypeRef("x", "S")},
			"x.S": &node.Struct{Name: "S", Package: "x"},
		}
		if _, ok := golang.ZeroRefFor(namedTypeRef("x", "A"), r); ok {
			t.Error("an alias over a struct reported a bare zero")
		}
	})

	t.Run("a declaration that is neither alias nor struct derives nothing", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"x.E": &node.Enum{Name: "E", Package: "x"}}
		if _, ok := golang.ZeroRefFor(namedTypeRef("x", "E"), r); ok {
			t.Error("an enum reported a zero through the named path")
		}
	})
}

// TestSampleRefFor_UnsampleableDeclaration pins the arm for a named
// type that resolves to a declaration with no sample form. An enum
// resolves through the same registry as a struct and has no composite
// literal, so it must derive nothing rather than an empty one.
func TestSampleRefFor_UnsampleableDeclaration(t *testing.T) {
	t.Parallel()

	t.Run("a declaration that is neither alias nor struct derives nothing", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"x.E": &node.Enum{Name: "E", Package: "x"}}
		s, a := golang.SampleRefFor(namedTypeRef("x", "E"), "f", r)
		if s.OK() || a.OK() {
			t.Errorf("derived %q / %q for an enum", s.Text, a.Text)
		}
	})
}

// TestSampleRefusal_Causes pins each refusal to the cause that raised
// it. The point of the enum is that a caller can tell a run that was
// too narrow from a type that has no literal, so a site reporting the
// wrong one is worse than the single signal it replaced.
func TestSampleRefusal_Causes(t *testing.T) {
	t.Parallel()

	t.Run("a derived sample refuses nothing", func(t *testing.T) {
		t.Parallel()
		s, _ := golang.SampleRefFor(builtinRef("int"), "n", nil)
		if s.Refusal != golang.RefusedNone {
			t.Errorf("Refusal = %d on a derived sample, want RefusedNone", s.Refusal)
		}
	})

	t.Run("a nil type reports the caller's input", func(t *testing.T) {
		t.Parallel()
		s, _ := golang.SampleRefFor(nil, "f", mapResolver{})
		if s.Refusal != golang.RefusedNoResolver {
			t.Errorf("Refusal = %d, want RefusedNoResolver", s.Refusal)
		}
	})

	t.Run("a named type with no resolver reports the caller's input", func(t *testing.T) {
		t.Parallel()
		s, _ := golang.SampleRefFor(namedTypeRef("x", "T"), "f", nil)
		if s.Refusal != golang.RefusedNoResolver {
			t.Errorf("Refusal = %d, want RefusedNoResolver", s.Refusal)
		}
	})

	t.Run("a type the resolver cannot reach reports unresolved", func(t *testing.T) {
		t.Parallel()
		s, _ := golang.SampleRefFor(namedTypeRef("x", "Absent"), "f", mapResolver{})
		if s.Refusal != golang.RefusedUnresolved {
			t.Errorf("Refusal = %d, want RefusedUnresolved", s.Refusal)
		}
	})

	t.Run("a builtin outside the value table reports no literal", func(t *testing.T) {
		t.Parallel()
		s, _ := golang.SampleRefFor(builtinRef("chan"), "c", mapResolver{})
		if s.Refusal != golang.RefusedNoLiteral {
			t.Errorf("Refusal = %d, want RefusedNoLiteral", s.Refusal)
		}
	})

	t.Run("a declaration with no sample form reports no literal", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"x.E": &node.Enum{Name: "E", Package: "x"}}
		s, _ := golang.SampleRefFor(namedTypeRef("x", "E"), "f", r)
		if s.Refusal != golang.RefusedNoLiteral {
			t.Errorf("Refusal = %d, want RefusedNoLiteral", s.Refusal)
		}
	})

	t.Run("both halves of the pair carry the same reason", func(t *testing.T) {
		t.Parallel()
		s, a := golang.SampleRefFor(namedTypeRef("x", "Absent"), "f", mapResolver{})
		if s.Refusal != a.Refusal {
			t.Errorf("halves disagree: %d and %d", s.Refusal, a.Refusal)
		}
	})
}

// TestSampleRefusal_Propagates pins the composite arms. A slice of an
// unloaded type is a narrow run, not a type without a literal, and
// reporting the latter would send a caller looking for a defect in a
// type that is fine.
func TestSampleRefusal_Propagates(t *testing.T) {
	t.Parallel()

	t.Run("a slice reports its element's reason", func(t *testing.T) {
		t.Parallel()
		s, _ := golang.SampleRefFor(
			sliceRef(namedTypeRef("x", "Absent")), "v", mapResolver{},
		)
		if s.Refusal != golang.RefusedUnresolved {
			t.Errorf("Refusal = %d, want the element's RefusedUnresolved", s.Refusal)
		}
	})

	t.Run("a map reports its value's reason", func(t *testing.T) {
		t.Parallel()
		s, _ := golang.SampleRefFor(
			mapRef(builtinRef("string"), namedTypeRef("x", "Absent")), "v", mapResolver{},
		)
		if s.Refusal != golang.RefusedUnresolved {
			t.Errorf("Refusal = %d, want the value's RefusedUnresolved", s.Refusal)
		}
	})

	t.Run("a struct whose only candidate was unloaded reports unresolved", func(t *testing.T) {
		t.Parallel()
		// Not RefusedNoLiteral: the struct would have sampled under a run
		// that loaded the field's type.
		r := mapResolver{"x.Holder": &node.Struct{
			Name: "Holder", Package: "x",
			Fields: []*node.Field{{Name: "Inner", Type: namedTypeRef("y", "Absent")}},
		}}
		s, _ := golang.SampleRefFor(namedTypeRef("x", "Holder"), "h", r)
		if s.Refusal != golang.RefusedUnresolved {
			t.Errorf("Refusal = %d, want RefusedUnresolved", s.Refusal)
		}
	})

	t.Run("a struct with only unexported fields reports no literal", func(t *testing.T) {
		t.Parallel()
		// Nothing a wider run would change: the fields exist and cannot
		// be set from another package.
		r := mapResolver{"x.Hidden": &node.Struct{
			Name: "Hidden", Package: "x",
			Fields: []*node.Field{{Name: "secret", Type: builtinRef("int")}},
		}}
		s, _ := golang.SampleRefFor(namedTypeRef("x", "Hidden"), "h", r)
		if s.Refusal != golang.RefusedNoLiteral {
			t.Errorf("Refusal = %d, want RefusedNoLiteral", s.Refusal)
		}
	})
}

// TestSampleRefusal_Incomplete pins the partition the enum exists to
// express: fixable input against a settled fact about the type.
func TestSampleRefusal_Incomplete(t *testing.T) {
	t.Parallel()

	t.Run("the input causes report incomplete", func(t *testing.T) {
		t.Parallel()
		for _, r := range []golang.SampleRefusal{
			golang.RefusedNoResolver, golang.RefusedDepth, golang.RefusedUnresolved,
		} {
			if !r.Incomplete() {
				t.Errorf("refusal %d does not report incomplete", r)
			}
		}
	})

	t.Run("no-literal and no-refusal are not incomplete", func(t *testing.T) {
		t.Parallel()
		if golang.RefusedNoLiteral.Incomplete() {
			t.Error("RefusedNoLiteral reported as incomplete")
		}
		if golang.RefusedNone.Incomplete() {
			t.Error("RefusedNone reported as incomplete")
		}
	})
}

// TestZeroRefFor_ReportsWhy pins the same reporting on the zero form,
// which had the identical gap behind a bare bool.
func TestZeroRefFor_ReportsWhy(t *testing.T) {
	t.Parallel()

	t.Run("an unreachable type reports unresolved", func(t *testing.T) {
		t.Parallel()
		s, ok := golang.ZeroRefFor(namedTypeRef("x", "Absent"), mapResolver{})
		if ok {
			t.Fatal("an unresolvable type reported a zero")
		}
		if s.Refusal != golang.RefusedUnresolved {
			t.Errorf("Refusal = %d, want RefusedUnresolved", s.Refusal)
		}
	})

	t.Run("a declaration with no zero form reports no literal", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"x.E": &node.Enum{Name: "E", Package: "x"}}
		s, ok := golang.ZeroRefFor(namedTypeRef("x", "E"), r)
		if ok {
			t.Fatal("an enum reported a zero through the named path")
		}
		if s.Refusal != golang.RefusedNoLiteral {
			t.Errorf("Refusal = %d, want RefusedNoLiteral", s.Refusal)
		}
	})
}

// TestSampleRefusal_Depth pins the one cause no direct call can raise.
// A type that refers to itself exhausts the budget rather than the
// stack, and the caller needs to know the walk stopped early rather
// than that the type has nothing to offer.
func TestSampleRefusal_Depth(t *testing.T) {
	t.Parallel()

	t.Run("a self-referential struct reports the exhausted budget", func(t *testing.T) {
		t.Parallel()
		loop := &node.Struct{Name: "Loop", Package: "x"}
		loop.Fields = []*node.Field{{Name: "Next", Type: namedTypeRef("x", "Loop")}}
		s, _ := golang.SampleRefFor(namedTypeRef("x", "Loop"), "l", mapResolver{"x.Loop": loop})
		if s.OK() {
			t.Fatalf("a self-referential struct derived %q", s.Text)
		}
		if s.Refusal != golang.RefusedDepth {
			t.Errorf("Refusal = %d, want RefusedDepth", s.Refusal)
		}
	})
}

// TestSampleRefFor_StdlibTable covers the curated table for named
// standard-library types, which the resolver can never answer for
// because the run never loads them.
func TestSampleRefFor_StdlibTable(t *testing.T) {
	t.Parallel()

	t.Run("a duration samples without a resolver", func(t *testing.T) {
		t.Parallel()
		// The motivating call: scheduled.At(ctx, after time.Duration)
		// lost its whole derived family to RefusedNoResolver.
		s, a := golang.SampleRefFor(namedTypeRef("time", "Duration"), "after", nil)
		if !s.OK() || !a.OK() {
			t.Fatalf("refused: sample=%+v alternate=%+v", s, a)
		}
		if s.Text != "42" || a.Text != "7" {
			t.Fatalf("texts = %q/%q, want 42/7", s.Text, a.Text)
		}
		if s.Composite {
			t.Fatal("Composite = true, want a conversion: time.Duration(42), not time.Duration{42}")
		}
	})

	t.Run("the sample carries the ref that registers the import", func(t *testing.T) {
		t.Parallel()
		// Text alone would land an unqualified name in a file that
		// never imported time; the consumer's backend registers
		// imports only through rendered references.
		s, _ := golang.SampleRefFor(namedTypeRef("time", "Duration"), "after", nil)
		if s.Ref == nil {
			t.Fatal("Ref = nil, want the time.Duration reference")
		}
	})

	t.Run("a timestamp samples as a constructor call", func(t *testing.T) {
		t.Parallel()
		// The entry #42 deferred, landed with the mechanism it was
		// waiting for: a constructor call has no conversion form, so
		// it rides Sample.Expr rather than the Ref-and-Text pair.
		s, a := golang.SampleRefFor(namedTypeRef("time", "Time"), "at", nil)
		if !s.OK() || s.Expr == nil {
			t.Fatalf("refused: %+v", s)
		}
		if s.Text != "" || s.Ref != nil {
			t.Fatalf("Text/Ref = %q/%v, want empty: Expr is the whole sample", s.Text, s.Ref)
		}
		isUnixCall := s.Expr.ExprKind == emit.ExprCall && s.Expr.Callee != nil &&
			s.Expr.Callee.Pkg == "time" && s.Expr.Callee.Name == "Unix"
		if !isUnixCall {
			t.Fatalf("Expr = %+v, want a time.Unix call", s.Expr)
		}
		distinct := len(s.Expr.Args) > 0 && len(a.Expr.Args) > 0 &&
			s.Expr.Args[0].RawText != a.Expr.Args[0].RawText
		if !distinct {
			t.Fatalf("sample and alternate share a seconds argument; want distinct values")
		}
	})

	t.Run("a stdlib type outside the table still refuses", func(t *testing.T) {
		t.Parallel()
		s, _ := golang.SampleRefFor(namedTypeRef("time", "Month"), "m", nil)
		if s.OK() {
			t.Fatalf("derived %q for time.Month, which the table omits", s.Text)
		}
		if s.Refusal != golang.RefusedNoResolver {
			t.Fatalf("Refusal = %d, want RefusedNoResolver, the pre-table answer", s.Refusal)
		}
	})

	t.Run("the string form still refuses a table entry", func(t *testing.T) {
		t.Parallel()
		// SampleFor keeps only what a string can spell, and a table
		// sample carries a Ref by construction — the import is the
		// point. Pinned so the table cannot silently widen the
		// string surface past its documented contract.
		if s, _ := golang.SampleFor(namedTypeRef("time", "Duration"), "after", nil); s != "" {
			t.Fatalf("SampleFor = %q, want empty: the sample needs an import a string cannot register", s)
		}
	})
}

// funcRef builds a func-kind TypeRef from parameter and return type
// refs — the shape frontend/golang's funcTypeRef produces.
func funcRef(params, returns []*node.TypeRef) *node.TypeRef {
	return &node.TypeRef{
		TypeKind:    node.TypeRefFunc,
		FuncParams:  params,
		FuncReturns: returns,
	}
}

// chanRef builds a channel TypeRef exactly as the frontend models
// one: a named `go`.`chan` ref with the element as the single type
// argument and the direction stamped as meta.
func chanRef(elem *node.TypeRef, dir golang.ChanDirection) *node.TypeRef {
	r := &node.TypeRef{
		TypeKind: node.TypeRefNamed,
		Package:  "go",
		Name:     "chan",
		TypeArgs: []*node.TypeRef{elem},
	}
	golang.MetaIsChannel.Set(r.EnsureMeta(), true, "test")
	golang.MetaChanDir.Set(r.EnsureMeta(), string(dir), "test")
	return r
}

// TestSampleRefFor_FuncValues covers the func arm: the no-op literal
// is the one value a caller can pass that asserts nothing.
func TestSampleRefFor_FuncValues(t *testing.T) {
	t.Parallel()

	t.Run("a hook without results samples as an empty literal", func(t *testing.T) {
		t.Parallel()
		// The corpus shape: OnEvent(fn func(event string)).
		s, _ := golang.SampleRefFor(funcRef([]*node.TypeRef{builtinRef("string")}, nil), "fn", nil)
		if !s.OK() || s.Expr == nil {
			t.Fatalf("refused: %+v", s)
		}
		if s.Expr.ExprKind != emit.ExprFuncLit {
			t.Fatalf("ExprKind = %v, want a func literal", s.Expr.ExprKind)
		}
		if len(s.Expr.FuncParams) != 1 || len(s.Expr.FuncBody) != 0 {
			t.Fatalf("params/body = %d/%d, want 1 anonymous param and an empty body",
				len(s.Expr.FuncParams), len(s.Expr.FuncBody))
		}
	})

	t.Run("results are declared vars returned, not derived zeros", func(t *testing.T) {
		t.Parallel()
		// The corpus shape: Run(ctx, body func(Tx) error). `var r0
		// error; return r0` compiles for every result type with
		// nothing but the type's reference, so no zero literal is
		// derived and no refusal can propagate from one — a func
		// answering an unloaded type still samples.
		ref := funcRef(
			[]*node.TypeRef{namedTypeRef("example.com/db", "Tx")},
			[]*node.TypeRef{builtinRef("error"), namedTypeRef("example.com/y", "Absent")},
		)
		s, _ := golang.SampleRefFor(ref, "body", nil)
		if !s.OK() || s.Expr == nil {
			t.Fatalf("refused: %+v", s)
		}
		body := s.Expr.FuncBody
		if len(body) != 3 {
			t.Fatalf("body has %d statements, want two declarations and a return", len(body))
		}
		declared := body[0].StmtKind == emit.StmtVar && body[1].StmtKind == emit.StmtVar &&
			body[2].StmtKind == emit.StmtReturn
		if !declared {
			t.Fatalf("body kinds = %v/%v/%v, want var/var/return",
				body[0].StmtKind, body[1].StmtKind, body[2].StmtKind)
		}
		if len(body[2].Returns) != 2 {
			t.Fatalf("return carries %d values, want both declared vars", len(body[2].Returns))
		}
	})

	t.Run("the alternate is an independent build", func(t *testing.T) {
		t.Parallel()
		// Funcs compare only to nil, so the pair cannot differ
		// observably — but a consumer mutating one must not reach
		// the other through a shared node.
		s, a := golang.SampleRefFor(funcRef(nil, nil), "fn", nil)
		if s.Expr == nil || a.Expr == nil || s.Expr == a.Expr {
			t.Fatalf("sample and alternate share the expression node")
		}
	})

	t.Run("the string form refuses a func sample", func(t *testing.T) {
		t.Parallel()
		if s, _ := golang.SampleFor(funcRef(nil, nil), "fn", nil); s != "" {
			t.Fatalf("SampleFor = %q, want empty: an expression is not a string", s)
		}
	})
}

// TestSampleRefFor_ChannelValues covers the chan arm, which lives in
// the named section: a channel arrives as `go`.`chan` with the
// frontend's stamp, never as a kind of its own, and left alone would
// die at the resolver like any unloaded name.
func TestSampleRefFor_ChannelValues(t *testing.T) {
	t.Parallel()

	t.Run("a receive channel samples as a bidirectional make", func(t *testing.T) {
		t.Parallel()
		// The corpus shape: Subscribe(ctx) (<-chan Value, error).
		// make is not legal on a directional channel; a `chan T`
		// assigns to both directional forms.
		recv := chanRef(namedTypeRef("example.com/bus", "Value"), golang.ChanRecv)
		s, _ := golang.SampleRefFor(recv, "ch", nil)
		if !s.OK() || s.Expr == nil {
			t.Fatalf("refused: %+v", s)
		}
		if s.Expr.ExprKind != emit.ExprMake || s.Expr.AsType == nil {
			t.Fatalf("Expr = %+v, want a make of the channel type", s.Expr)
		}
		made, ok := s.Expr.AsType.Origin().(*node.TypeRef)
		if !ok {
			t.Fatal("made type carries no origin TypeRef")
		}
		if golang.ChanDir(made) != golang.ChanBoth {
			t.Fatalf("made channel direction = %q, want both", golang.ChanDir(made))
		}
		if elem := golang.ChanElem(made); elem == nil || elem.Name != "Value" {
			t.Fatalf("made channel element = %+v, want the Value ref", elem)
		}
	})

	t.Run("a channel without an element refuses", func(t *testing.T) {
		t.Parallel()
		// Defensive: the frontend always records the element, so a
		// bare go.chan is a hand-built fixture — refusing beats
		// emitting make() of nothing.
		bare := &node.TypeRef{TypeKind: node.TypeRefNamed, Package: "go", Name: "chan"}
		golang.MetaIsChannel.Set(bare.EnsureMeta(), true, "test")
		s, _ := golang.SampleRefFor(bare, "ch", nil)
		if s.OK() {
			t.Fatal("derived a sample for a channel with no element")
		}
	})

	t.Run("the alternate is an independent build", func(t *testing.T) {
		t.Parallel()
		// Channels compare by identity: two makes are two channels,
		// and the shared-node hazard is the same as the func arm's.
		both := chanRef(builtinRef("int"), golang.ChanBoth)
		s, a := golang.SampleRefFor(both, "ch", nil)
		if s.Expr == nil || a.Expr == nil || s.Expr == a.Expr {
			t.Fatalf("sample and alternate share the expression node")
		}
	})
}

// TestSample_OKWidening pins that an Expr sample is OK — the half
// the consumer's evidence gate reads. A predicate beside OK() would
// drift; widening is the point.
func TestSample_OKWidening(t *testing.T) {
	t.Parallel()
	if (golang.Sample{Expr: &emit.Expr{ExprKind: emit.ExprFuncLit}}).OK() == false {
		t.Fatal("Sample with Expr reports !OK; the evidence gate would withhold a written defect")
	}
	if (golang.Sample{}).OK() {
		t.Fatal("empty Sample reports OK")
	}
}
