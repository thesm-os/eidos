// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

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

	t.Run("a stdlib type outside the table still refuses", func(t *testing.T) {
		t.Parallel()
		// time.Time stays out until a corpus needs it: its writable
		// values are constructor calls, and Sample has no
		// verbatim-expression form to carry one.
		s, _ := golang.SampleRefFor(namedTypeRef("time", "Time"), "at", nil)
		if s.OK() {
			t.Fatalf("derived %q for time.Time, which the table deliberately omits", s.Text)
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
