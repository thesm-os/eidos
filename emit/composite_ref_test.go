// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
)

func TestCompositeShape_String(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		s    emit.CompositeShape
		want string
	}{
		{"Pointer", emit.ShapePointer, "pointer"},
		{"Slice", emit.ShapeSlice, "slice"},
		{"Array", emit.ShapeArray, "array"},
		{"Map", emit.ShapeMap, "map"},
		{"Func", emit.ShapeFunc, "func"},
		{"Union", emit.ShapeUnion, "union"},
		{"AnonStruct", emit.ShapeAnonStruct, "anon_struct"},
		{"unknown stringifies with a marker", emit.CompositeShape(99), "composite_shape(?)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertEqualString(t, tc.s.String(), tc.want)
		})
	}
}

func TestPtr(t *testing.T) {
	t.Parallel()

	t.Run("wraps elem in a pointer composite", func(t *testing.T) {
		t.Parallel()
		r := emit.Ptr(emit.Builtin("int"))
		if r.Shape != emit.ShapePointer {
			t.Fatalf("Shape = %s, want pointer", r.Shape)
		}
		if r.Elem == nil {
			t.Fatalf("Elem should be the supplied ref")
		}
	})
}

func TestSliceOf(t *testing.T) {
	t.Parallel()

	t.Run("wraps elem in a slice composite", func(t *testing.T) {
		t.Parallel()
		r := emit.SliceOf(emit.Builtin("byte"))
		if r.Shape != emit.ShapeSlice {
			t.Fatalf("Shape = %s, want slice", r.Shape)
		}
	})
}

func TestArrayOf(t *testing.T) {
	t.Parallel()

	t.Run("wraps elem in a fixed-length array composite", func(t *testing.T) {
		t.Parallel()
		r := emit.ArrayOf(emit.Builtin("byte"), 16)
		if r.Shape != emit.ShapeArray {
			t.Fatalf("Shape = %s, want array", r.Shape)
		}
		if r.ArrayLen != 16 {
			t.Fatalf("ArrayLen = %d, want 16", r.ArrayLen)
		}
	})
}

func TestMapOf(t *testing.T) {
	t.Parallel()

	t.Run("wraps key and value refs in a map composite", func(t *testing.T) {
		t.Parallel()
		r := emit.MapOf(emit.Builtin("string"), emit.Builtin("int"))
		if r.Shape != emit.ShapeMap {
			t.Fatalf("Shape = %s, want map", r.Shape)
		}
		if r.MapKey == nil || r.MapValue == nil {
			t.Fatalf("MapKey/MapValue must be populated")
		}
	})
}

func TestFuncOf(t *testing.T) {
	t.Parallel()

	t.Run("constructs a function composite from params and returns", func(t *testing.T) {
		t.Parallel()
		r := emit.FuncOf(
			[]emit.Ref{emit.Builtin("int")},
			[]emit.Ref{emit.Builtin("error")},
		)
		if r.Shape != emit.ShapeFunc {
			t.Fatalf("Shape = %s, want func", r.Shape)
		}
		if len(r.FuncParams) != 1 || len(r.FuncReturns) != 1 {
			t.Fatalf("FuncParams/Returns mismatch: %+v", r)
		}
	})

	t.Run("normalises nil slices to empty slices", func(t *testing.T) {
		t.Parallel()
		r := emit.FuncOf(nil, nil)
		if r.FuncParams == nil || r.FuncReturns == nil {
			t.Fatalf("nil slices should normalise to empty, not nil")
		}
		if len(r.FuncParams) != 0 || len(r.FuncReturns) != 0 {
			t.Fatalf("expected empty slices; got %+v", r)
		}
	})
}

func TestUnion(t *testing.T) {
	t.Parallel()

	t.Run("constructs a union composite with terms and approx flags", func(t *testing.T) {
		t.Parallel()
		r := emit.Union(
			emit.UnionTerm{Type: emit.Builtin("int")},
			emit.UnionTerm{Type: emit.Builtin("string"), Approx: true},
		)
		if r.Shape != emit.ShapeUnion {
			t.Fatalf("Shape = %s, want union", r.Shape)
		}
		if len(r.UnionTerms) != 2 {
			t.Fatalf("UnionTerms len = %d, want 2", len(r.UnionTerms))
		}
		if r.UnionTerms[0].Approx {
			t.Fatalf("first term should not be approx")
		}
		if !r.UnionTerms[1].Approx {
			t.Fatalf("second term should be approx")
		}
		if r.UnionTerms[0].Type == nil || r.UnionTerms[1].Type == nil {
			t.Fatalf("term Type refs must be populated")
		}
	})

	t.Run("zero terms produce a non-nil empty slice", func(t *testing.T) {
		t.Parallel()
		r := emit.Union()
		if r.Shape != emit.ShapeUnion {
			t.Fatalf("Shape = %s, want union", r.Shape)
		}
		if r.UnionTerms == nil {
			t.Fatalf("UnionTerms should not be nil even with zero terms")
		}
		if len(r.UnionTerms) != 0 {
			t.Fatalf("expected empty UnionTerms; got %+v", r.UnionTerms)
		}
	})

	t.Run("reports KindCompositeRef", func(t *testing.T) {
		t.Parallel()
		r := emit.Union(emit.UnionTerm{Type: emit.Builtin("int")})
		if r.Kind() != emit.KindCompositeRef {
			t.Fatalf("Kind = %s, want %s", r.Kind(), emit.KindCompositeRef)
		}
	})

	t.Run("satisfies the Ref interface", func(t *testing.T) {
		t.Parallel()
		var _ emit.Ref = emit.Union(emit.UnionTerm{Type: emit.Builtin("int")})
	})
}

func TestAnonStructOf(t *testing.T) {
	t.Parallel()

	t.Run("no fields and no embeds is the struct{} shape", func(t *testing.T) {
		t.Parallel()
		r := emit.AnonStructOf(nil, nil)
		if r.Shape != emit.ShapeAnonStruct {
			t.Fatalf("Shape = %s, want anon_struct", r.Shape)
		}
		if len(r.StructFields) != 0 || len(r.StructEmbeds) != 0 {
			t.Fatalf("expected an empty shape; got %+v", r)
		}
	})

	t.Run("nil slices are not normalised to empty ones", func(t *testing.T) {
		t.Parallel()
		// Unlike FuncOf: `struct{}` is the common case here, not an
		// unset builder seed, so allocating two empty slices for every
		// map[K]struct{} in a graph would be pure waste.
		r := emit.AnonStructOf(nil, nil)
		if r.StructFields != nil {
			t.Fatalf("StructFields should stay nil; got %+v", r.StructFields)
		}
		if r.StructEmbeds != nil {
			t.Fatalf("StructEmbeds should stay nil; got %+v", r.StructEmbeds)
		}
	})

	t.Run("carries fields in order with names, types and tags", func(t *testing.T) {
		t.Parallel()
		r := emit.AnonStructOf([]emit.AnonField{
			{Name: "A", Type: emit.Builtin("int")},
			{Name: "B", Type: emit.Builtin("string"), Tag: `json:"b"`},
		}, nil)
		if len(r.StructFields) != 2 {
			t.Fatalf("StructFields len = %d, want 2", len(r.StructFields))
		}
		if r.StructFields[0].Name != "A" || r.StructFields[1].Name != "B" {
			t.Fatalf("field order not preserved; got %+v", r.StructFields)
		}
		if r.StructFields[0].Tag != "" {
			t.Fatalf("first field should carry no tag; got %q", r.StructFields[0].Tag)
		}
		if r.StructFields[1].Tag != `json:"b"` {
			t.Fatalf("second field tag = %q, want json:\"b\"", r.StructFields[1].Tag)
		}
		if r.StructFields[0].Type == nil || r.StructFields[1].Type == nil {
			t.Fatalf("field Type refs must be populated")
		}
	})

	t.Run("carries embeds in order", func(t *testing.T) {
		t.Parallel()
		r := emit.AnonStructOf(nil, []emit.Ref{
			emit.Builtin("error"),
			emit.External("io", "Reader"),
		})
		if len(r.StructEmbeds) != 2 {
			t.Fatalf("StructEmbeds len = %d, want 2", len(r.StructEmbeds))
		}
		if got, ok := r.StructEmbeds[0].(*emit.BuiltinRef); !ok || got.Name != "error" {
			t.Fatalf("embed order not preserved; got %+v", r.StructEmbeds)
		}
	})

	t.Run("reports KindCompositeRef", func(t *testing.T) {
		t.Parallel()
		if got := emit.AnonStructOf(nil, nil).Kind(); got != emit.KindCompositeRef {
			t.Fatalf("Kind = %s, want %s", got, emit.KindCompositeRef)
		}
	})

	t.Run("satisfies the Ref interface", func(t *testing.T) {
		t.Parallel()
		var _ emit.Ref = emit.AnonStructOf(nil, nil)
	})
}

// TestCompositeShape_SerialisedOrdinals pins the integer value of
// every shape.
//
// [emit.CompositeRef.Shape] encodes as a bare int, so these ordinals
// are the serialised contract: a variant inserted anywhere but the
// end of the const block silently reinterprets every emit graph
// already written to disk — a persisted 4 would stop meaning func.
// New variants append, and this test is what says so out loud.
func TestCompositeShape_SerialisedOrdinals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		s    emit.CompositeShape
		want int
	}{
		{"Pointer", emit.ShapePointer, 0},
		{"Slice", emit.ShapeSlice, 1},
		{"Array", emit.ShapeArray, 2},
		{"Map", emit.ShapeMap, 3},
		{"Func", emit.ShapeFunc, 4},
		{"Union", emit.ShapeUnion, 5},
		{"AnonStruct", emit.ShapeAnonStruct, 6},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if int(tc.s) != tc.want {
				t.Fatalf("%s = %d, want %d — reordering breaks every persisted graph",
					tc.name, int(tc.s), tc.want)
			}
		})
	}
}

func TestCompositeRef_Kind(t *testing.T) {
	t.Parallel()

	t.Run("reports KindCompositeRef", func(t *testing.T) {
		t.Parallel()
		r := emit.Ptr(emit.Builtin("int"))
		if r.Kind() != emit.KindCompositeRef {
			t.Fatalf("Kind = %s, want %s", r.Kind(), emit.KindCompositeRef)
		}
	})
}

func TestCompositeRef_SatisfiesRef(t *testing.T) {
	t.Parallel()

	t.Run("CompositeRef satisfies the Ref interface", func(t *testing.T) {
		t.Parallel()
		var _ emit.Ref = emit.Ptr(emit.Builtin("int"))
	})
}
