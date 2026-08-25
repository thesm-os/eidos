// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// targetOf converts `type X = <expr>` and returns the target ref.
func targetOf(t *testing.T, expr string) *node.TypeRef {
	t.Helper()
	return onlyAlias(t, "type X = "+expr+";").Target
}

func TestReadonlyRef(t *testing.T) {
	t.Parallel()

	t.Run("readonly stamps the type it wraps rather than adding a layer", func(t *testing.T) {
		t.Parallel()
		// `readonly T[]` and `T[]` are the same array of the same
		// element; readonly constrains the binding. A wrapper ref
		// would make every consumer unwrap one more level to reach a
		// type it already understands.
		ref := targetOf(t, "readonly string[]")

		if !ref.IsSlice() {
			t.Fatalf("Target = %+v, want a slice", ref)
		}
		if ref.Elem == nil || ref.Elem.Name != "string" {
			t.Fatalf("Elem = %+v, want string", ref.Elem)
		}
		if ro, _ := typescript.MetaReadonly.Get(ref.Meta()); !ro {
			t.Error("readonly not stamped on the wrapped type")
		}
	})

	t.Run("a mutable array carries no readonly stamp", func(t *testing.T) {
		t.Parallel()
		if typescript.MetaReadonly.Has(targetOf(t, "string[]").Meta()) {
			t.Fatal("a mutable array was marked readonly")
		}
	})

	t.Run("a readonly tuple keeps its element list", func(t *testing.T) {
		t.Parallel()
		ref := targetOf(t, "readonly [string, number]")
		if !typescript.IsTuple(ref) {
			t.Fatalf("Target = %+v, want a tuple", ref)
		}
		if got := len(typescript.Members(ref)); got != 2 {
			t.Fatalf("elements = %d, want 2", got)
		}
	})
}

func TestTupleElements(t *testing.T) {
	t.Parallel()

	t.Run("a rest element is marked", func(t *testing.T) {
		t.Parallel()
		ref := targetOf(t, "[a: string, ...rest: number[]]")
		members := typescript.Members(ref)
		if len(members) != 2 {
			t.Fatalf("elements = %d, want 2", len(members))
		}
		if rest, _ := typescript.MetaRest.Get(members[1].Meta()); !rest {
			t.Error("the rest element was not marked")
		}
	})

	t.Run("unlabelled elements convert without modifiers", func(t *testing.T) {
		t.Parallel()
		ref := targetOf(t, "[string, number]")
		for i, m := range typescript.Members(ref) {
			if typescript.MetaOptional.Has(m.Meta()) || typescript.MetaRest.Has(m.Meta()) {
				t.Errorf("element %d gained a modifier it does not have", i)
			}
		}
	})

	t.Run("an empty tuple has no elements", func(t *testing.T) {
		t.Parallel()
		ref := targetOf(t, "[]")
		if !typescript.IsTuple(ref) {
			t.Fatalf("Target = %+v, want a tuple", ref)
		}
		if got := len(typescript.Members(ref)); got != 0 {
			t.Fatalf("elements = %d, want 0", got)
		}
	})
}

func TestObjectTypeShapes(t *testing.T) {
	t.Parallel()

	t.Run("an index-signature-only object is a map", func(t *testing.T) {
		t.Parallel()
		ref := targetOf(t, "{ [k: string]: number }")
		if !ref.IsMap() {
			t.Fatalf("Target = %+v, want a map", ref)
		}
	})

	t.Run("a readonly index signature marks the map", func(t *testing.T) {
		t.Parallel()
		ref := targetOf(t, "{ readonly [k: string]: number }")
		if !ref.IsMap() {
			t.Fatalf("Target = %+v, want a map", ref)
		}
		if ro, _ := typescript.MetaReadonly.Get(ref.Meta()); !ro {
			t.Error("a readonly index signature did not mark the map")
		}
	})

	t.Run("an empty object type is a struct with no fields", func(t *testing.T) {
		t.Parallel()
		// Distinct from an index signature: `{}` genuinely declares no
		// members, where `{[k: string]: V}` admits any key.
		ref := targetOf(t, "{}")
		if !ref.IsAnonStruct() {
			t.Fatalf("Target = %+v, want an anon struct", ref)
		}
		if len(ref.Fields) != 0 {
			t.Fatalf("Fields = %d, want 0", len(ref.Fields))
		}
	})

	t.Run("a nested object type converts its inner fields", func(t *testing.T) {
		t.Parallel()
		ref := targetOf(t, "{ inner: { deep: string } }")
		if len(ref.Fields) != 1 {
			t.Fatalf("Fields = %d, want 1", len(ref.Fields))
		}
		inner := ref.Fields[0].Type
		if inner == nil || !inner.IsAnonStruct() || len(inner.Fields) != 1 {
			t.Fatalf("inner = %+v, want an anon struct with one field", inner)
		}
	})
}

func TestTypeRefDepthLimit(t *testing.T) {
	t.Parallel()

	t.Run("a type nested past the budget stops rather than recursing", func(t *testing.T) {
		t.Parallel()
		// The budget exists so a pathological input fails one
		// declaration rather than the process. Well past what anyone
		// writes by hand.
		expr := "string"
		for range maxTypeDepth + 4 {
			expr = "Array<" + expr + ">"
		}
		ref := targetOf(t, expr)
		if ref == nil {
			t.Fatal("a deeply nested type produced no ref at all")
		}
	})
}
