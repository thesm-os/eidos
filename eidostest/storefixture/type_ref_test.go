// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package storefixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

func TestNamed(t *testing.T) {
	t.Parallel()

	t.Run("produces a Named ref with no package", func(t *testing.T) {
		t.Parallel()
		r := asNamedRef(t, storefixture.Named("string"))
		if r.Name != "string" || r.Package != "" {
			t.Fatalf("Named ref shape wrong: %+v", r)
		}
		if !r.IsBuiltin() {
			t.Fatalf("Named with no package should be a builtin")
		}
	})
}

func TestPkgNamed(t *testing.T) {
	t.Parallel()

	t.Run("produces a Named ref with package and name", func(t *testing.T) {
		t.Parallel()
		r := asNamedRef(t, storefixture.PkgNamed("context", "Context"))
		if r.Package != "context" || r.Name != "Context" {
			t.Fatalf("PkgNamed ref shape wrong: %+v", r)
		}
		if r.IsBuiltin() {
			t.Fatalf("PkgNamed should not be a builtin")
		}
	})
}

func TestWithArgs(t *testing.T) {
	t.Parallel()

	t.Run("returns a generic ref carrying the supplied type args", func(t *testing.T) {
		t.Parallel()
		base := storefixture.PkgNamed("foo", "Map")
		r := storefixture.WithArgs(base, storefixture.Named("string"), storefixture.Named("int"))
		if !r.IsGeneric() {
			t.Fatalf("WithArgs result should be generic; got %+v", r)
		}
		if len(r.TypeArgs) != 2 || r.TypeArgs[0].Name != "string" || r.TypeArgs[1].Name != "int" {
			t.Fatalf("type args wrong: %+v", r.TypeArgs)
		}
		if base.IsGeneric() {
			t.Fatalf("WithArgs must not mutate its input; base became generic: %+v", base)
		}
	})

	t.Run("panics for nil ref", func(t *testing.T) {
		t.Parallel()
		requirePanic(t, func() {
			storefixture.WithArgs(nil, storefixture.Named("string"))
		})
	})

	t.Run("panics for non-Named ref", func(t *testing.T) {
		t.Parallel()
		requirePanic(t, func() {
			storefixture.WithArgs(storefixture.Slice(storefixture.Named("string")))
		})
	})
}

func TestPointer(t *testing.T) {
	t.Parallel()

	t.Run("produces a Pointer ref over elem", func(t *testing.T) {
		t.Parallel()
		r := storefixture.Pointer(storefixture.Named("int"))
		if !r.IsPointer() {
			t.Fatalf("expected pointer ref, got %+v", r)
		}
		if r.Elem == nil || r.Elem.Name != "int" {
			t.Fatalf("pointer element wrong: %+v", r.Elem)
		}
	})
}

func TestSlice(t *testing.T) {
	t.Parallel()

	t.Run("produces a Slice ref over elem", func(t *testing.T) {
		t.Parallel()
		r := storefixture.Slice(storefixture.Named("byte"))
		if !r.IsSlice() {
			t.Fatalf("expected slice ref, got %+v", r)
		}
		if r.Elem == nil || r.Elem.Name != "byte" {
			t.Fatalf("slice element wrong: %+v", r.Elem)
		}
	})
}

func TestArray(t *testing.T) {
	t.Parallel()

	t.Run("produces an Array ref with element and length", func(t *testing.T) {
		t.Parallel()
		r := storefixture.Array(storefixture.Named("int"), 8)
		if !r.IsArray() {
			t.Fatalf("expected array ref, got %+v", r)
		}
		if r.ArrayLen != 8 {
			t.Fatalf("array length wrong: got %d, want 8", r.ArrayLen)
		}
		if r.Elem == nil || r.Elem.Name != "int" {
			t.Fatalf("array element wrong: %+v", r.Elem)
		}
	})
}

func TestMap(t *testing.T) {
	t.Parallel()

	t.Run("produces a Map ref with key and value types", func(t *testing.T) {
		t.Parallel()
		r := storefixture.Map(storefixture.Named("string"), storefixture.Named("int"))
		if !r.IsMap() {
			t.Fatalf("expected map ref, got %+v", r)
		}
		if r.MapKey == nil || r.MapKey.Name != "string" {
			t.Fatalf("map key wrong: %+v", r.MapKey)
		}
		if r.MapValue == nil || r.MapValue.Name != "int" {
			t.Fatalf("map value wrong: %+v", r.MapValue)
		}
	})
}

func TestTypeParamRef(t *testing.T) {
	t.Parallel()

	t.Run("produces a TypeParam-kind ref carrying the parameter name", func(t *testing.T) {
		t.Parallel()
		r := storefixture.TypeParamRef("T")
		if !r.IsTypeParam() {
			t.Fatalf("expected TypeParam ref, got %+v", r)
		}
		if r.Name != "T" {
			t.Fatalf("Name = %q, want %q", r.Name, "T")
		}
	})
}

func TestAnonStruct(t *testing.T) {
	t.Parallel()

	t.Run("produces an AnonStruct ref with fields wired to the ref", func(t *testing.T) {
		t.Parallel()
		field := &node.Field{Name: "ID", Type: storefixture.Named("string")}
		embed := &node.Embed{Type: storefixture.PkgNamed("io", "Reader")}
		r := storefixture.AnonStruct([]*node.Field{field}, []*node.Embed{embed})
		if !r.IsAnonStruct() {
			t.Fatalf("expected AnonStruct ref, got %+v", r)
		}
		if field.Owner != r {
			t.Fatalf("Field.Owner should be wired to the AnonStruct ref")
		}
		if embed.Owner != r {
			t.Fatalf("Embed.Owner should be wired to the AnonStruct ref")
		}
	})
}

func TestAnonInterface(t *testing.T) {
	t.Parallel()

	t.Run("produces an AnonInterface ref with methods wired to the ref", func(t *testing.T) {
		t.Parallel()
		method := &node.Method{Name: "Read"}
		embed := &node.Embed{Type: storefixture.PkgNamed("io", "Reader")}
		r := storefixture.AnonInterface([]*node.Method{method}, []*node.Embed{embed})
		if !r.IsAnonInterface() {
			t.Fatalf("expected AnonInterface ref, got %+v", r)
		}
		if method.Owner != r {
			t.Fatalf("Method.Owner should be wired to the AnonInterface ref")
		}
		if embed.Owner != r {
			t.Fatalf("Embed.Owner should be wired to the AnonInterface ref")
		}
	})
}

func TestConstraint(t *testing.T) {
	t.Parallel()

	t.Run("produces a Constraint with the supplied embeds", func(t *testing.T) {
		t.Parallel()
		c := storefixture.Constraint(
			storefixture.PkgNamed("fmt", "Stringer"),
			storefixture.Named("comparable"),
		)
		if c.IsAny() {
			t.Fatalf("constraint with embeds should not be IsAny")
		}
		if !c.IsComparable() {
			t.Fatalf("constraint should reflect comparable bound")
		}
		if len(c.Embedded) != 2 {
			t.Fatalf("expected 2 embeds, got %d", len(c.Embedded))
		}
	})

	t.Run("zero embeds produces an IsAny constraint", func(t *testing.T) {
		t.Parallel()
		c := storefixture.Constraint()
		if !c.IsAny() {
			t.Fatalf("constraint with no embeds should be IsAny")
		}
	})
}

func TestFunc(t *testing.T) {
	t.Parallel()

	t.Run("produces a Func ref with params and returns", func(t *testing.T) {
		t.Parallel()
		params := []*node.TypeRef{storefixture.Named("string")}
		returns := []*node.TypeRef{storefixture.Named("int"), storefixture.Named("error")}
		r := storefixture.Func(params, returns)
		if !r.IsFunc() {
			t.Fatalf("expected func ref, got %+v", r)
		}
		if len(r.FuncParams) != 1 || r.FuncParams[0].Name != "string" {
			t.Fatalf("func params wrong: %+v", r.FuncParams)
		}
		switch {
		case len(r.FuncReturns) != 2,
			r.FuncReturns[0].Name != "int",
			r.FuncReturns[1].Name != "error":
			t.Fatalf("func returns wrong: %+v", r.FuncReturns)
		}
	})
}

// TestChan pins that the fixture's channel is the one the Go frontend
// produces, field for field and stamp for stamp.
//
// Asserted against the readers in lang/golang rather than against the
// literal shape, because those are what every consumer goes through —
// a fixture that satisfied a struct comparison but not [golang.IsChannel]
// would be a channel nothing in the pipeline recognises as one.
func TestChan(t *testing.T) {
	t.Parallel()

	t.Run("reads back through the channel accessors", func(t *testing.T) {
		t.Parallel()
		ref := storefixture.Chan(storefixture.Named("int"))
		if !golang.IsChannel(ref) {
			t.Fatal("Chan produced a ref IsChannel does not recognise")
		}
		if got := golang.ChanElem(ref); got == nil || got.Name != "int" {
			t.Fatalf("ChanElem = %+v, want the int element", got)
		}
		if got := golang.ChanDir(ref); got != golang.ChanBoth {
			t.Fatalf("ChanDir = %q, want both", got)
		}
	})

	t.Run("carries the synthetic qualified name the backend keys on", func(t *testing.T) {
		t.Parallel()
		// The backend's channel arm only runs for a ref whose origin
		// carries go.isChannel; everything else falls through to the
		// external-ref path, which emits `import "go"`.
		ref := storefixture.Chan(storefixture.Named("int"))
		if got := golang.QName(ref); got != "go.chan" {
			t.Fatalf("QName = %q, want go.chan", got)
		}
		if len(ref.TypeArgs) != 1 {
			t.Fatalf("TypeArgs = %d, want the element on exactly one", len(ref.TypeArgs))
		}
	})

	t.Run("the directional forms differ only in direction", func(t *testing.T) {
		t.Parallel()
		for dir, build := range map[golang.ChanDirection]func(*node.TypeRef) *node.TypeRef{
			golang.ChanRecv: storefixture.RecvChan,
			golang.ChanSend: storefixture.SendChan,
			golang.ChanBoth: storefixture.Chan,
		} {
			ref := build(storefixture.Named("int"))
			if got := golang.ChanDir(ref); got != dir {
				t.Fatalf("ChanDir = %q, want %q", got, dir)
			}
			if !golang.IsChannel(ref) || golang.ChanElem(ref) == nil {
				t.Fatalf("the %q form lost its structure: %+v", dir, ref)
			}
		}
	})

	t.Run("stamps the element's fully-qualified printed form", func(t *testing.T) {
		t.Parallel()
		// go.chanElem is the templates-friendly view of TypeArgs[0].
		// The unqualified spelling reads as an in-package type, which
		// is the reading that makes a generated reference wrong.
		ref := storefixture.RecvChan(storefixture.PkgNamed("example.com/gen/shopv1", "Event"))
		got, ok := golang.MetaChanElem.Get(ref.Meta())
		if !ok || got != "example.com/gen/shopv1.Event" {
			t.Fatalf("go.chanElem = %q (%v), want the qualified form", got, ok)
		}
	})

	t.Run("refuses a channel with no element", func(t *testing.T) {
		t.Parallel()
		// A ref claiming go.isChannel with nothing on TypeArgs renders
		// as an error attributed to whichever plugin built it, which
		// for a fixture-built ref names the wrong culprit entirely.
		defer func() {
			if recover() == nil {
				t.Fatal("Chan(nil) did not panic")
			}
		}()
		storefixture.Chan(nil)
	})
}

// TestBound pins that a fixture constraint carries what a frontend's
// does. Constraint populates Embedded alone, and a consumer reading
// Raw as authoritative — which is the only field able to express a
// type set — sees a constraint that states no bound.
func TestBound(t *testing.T) {
	t.Parallel()

	t.Run("carries the printed source form alongside the bounds", func(t *testing.T) {
		t.Parallel()
		c := storefixture.Bound("fmt.Stringer", storefixture.PkgNamed("fmt", "Stringer"))
		if c.Raw != "fmt.Stringer" {
			t.Fatalf("Raw = %q, want the printed source form", c.Raw)
		}
		if len(c.Embedded) != 1 || c.Embedded[0].Name != "Stringer" {
			t.Fatalf("Embedded = %+v, want the structured bound too", c.Embedded)
		}
	})

	t.Run("expresses a type set, which Embedded cannot", func(t *testing.T) {
		t.Parallel()
		// `~int | ~string` has no Embedded representation, so the
		// structured field is empty and IsAny reads the constraint as
		// unbounded. Raw is the only record that the author bounded it.
		c := storefixture.Bound("~int | ~string")
		if !c.IsAny() {
			t.Fatal("a type-set constraint has no embedded bounds; IsAny should read it as unbounded")
		}
		if c.Raw != "~int | ~string" {
			t.Fatalf("Raw = %q, want the type-set form", c.Raw)
		}
	})

	t.Run("Constraint still leaves Raw empty", func(t *testing.T) {
		t.Parallel()
		// Pinned so the two constructors stay distinguishable: a test
		// asserting on Raw must be able to tell which one built the
		// fixture it was handed.
		if got := storefixture.Constraint(storefixture.Named("comparable")).Raw; got != "" {
			t.Fatalf("Constraint set Raw to %q; Bound is the constructor that populates it", got)
		}
	})
}
