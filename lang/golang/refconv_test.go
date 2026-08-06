// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	refconv "go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// TestFromNode covers every [node.TypeRef] variant the lifter
// recognises plus the External fallback for named cross-package
// refs. The conversion is referentially transparent — equivalent
// inputs produce equivalent emit shapes — so each case asserts
// shape + relevant payload.
func TestFromNode(t *testing.T) {
	t.Parallel()

	t.Run("builtin scalar lifts to BuiltinRef", func(t *testing.T) {
		t.Parallel()
		got := refconv.FromNode(&node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"})
		b, ok := got.(*emit.BuiltinRef)
		if !ok || b.Name != "string" {
			t.Fatalf("FromNode(string) = %T %v, want BuiltinRef{string}", got, got)
		}
	})

	t.Run("pointer wraps the element", func(t *testing.T) {
		t.Parallel()
		elem := &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "int"}
		ptr := &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: elem}
		got := refconv.FromNode(ptr)
		c, ok := got.(*emit.CompositeRef)
		if !ok || c.Shape != emit.ShapePointer {
			t.Fatalf("FromNode(*int) = %T %v, want pointer CompositeRef", got, got)
		}
	})

	t.Run("slice wraps the element", func(t *testing.T) {
		t.Parallel()
		slice := &node.TypeRef{
			TypeKind: node.TypeRefSlice,
			Elem:     &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"},
		}
		got := refconv.FromNode(slice)
		c, ok := got.(*emit.CompositeRef)
		if !ok || c.Shape != emit.ShapeSlice {
			t.Fatalf("FromNode([]string) = %T %v, want slice CompositeRef", got, got)
		}
	})

	t.Run("array preserves the length", func(t *testing.T) {
		t.Parallel()
		array := &node.TypeRef{
			TypeKind: node.TypeRefArray,
			Elem:     &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "byte"},
			ArrayLen: 16,
		}
		got := refconv.FromNode(array)
		c, ok := got.(*emit.CompositeRef)
		if !ok || c.Shape != emit.ShapeArray || c.ArrayLen != 16 {
			t.Fatalf("FromNode([16]byte) = %T %v, want array CompositeRef len=16", got, got)
		}
	})

	t.Run("map lifts key and value", func(t *testing.T) {
		t.Parallel()
		m := &node.TypeRef{
			TypeKind: node.TypeRefMap,
			MapKey:   &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"},
			MapValue: &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "int"},
		}
		got := refconv.FromNode(m)
		c, ok := got.(*emit.CompositeRef)
		if !ok || c.Shape != emit.ShapeMap {
			t.Fatalf("FromNode(map[string]int) = %T %v, want map CompositeRef", got, got)
		}
	})

	t.Run("type parameter renders as a bare identifier", func(t *testing.T) {
		t.Parallel()
		tp := &node.TypeRef{TypeKind: node.TypeRefTypeParam, Name: "T"}
		got := refconv.FromNode(tp)
		b, ok := got.(*emit.BuiltinRef)
		if !ok || b.Name != "T" {
			t.Fatalf("FromNode(typeparam T) = %T %v, want BuiltinRef{T}", got, got)
		}
	})

	t.Run("anon interface lifts to the any builtin", func(t *testing.T) {
		t.Parallel()
		got := refconv.FromNode(&node.TypeRef{TypeKind: node.TypeRefAnonInterface})
		b, ok := got.(*emit.BuiltinRef)
		if !ok || b.Name != "any" {
			t.Fatalf("FromNode(anon interface) = %T %v, want BuiltinRef{any}", got, got)
		}
	})

	t.Run("named cross-package ref lifts to ExternalRef with type args", func(t *testing.T) {
		t.Parallel()
		named := &node.TypeRef{
			TypeKind: node.TypeRefNamed,
			Package:  "example.com/pkg",
			Name:     "Box",
			TypeArgs: []*node.TypeRef{
				{TypeKind: node.TypeRefNamed, Name: "string"},
			},
		}
		got := refconv.FromNode(named)
		ext, ok := got.(*emit.ExternalRef)
		if !ok {
			t.Fatalf("FromNode(pkg.Box[string]) = %T %v, want ExternalRef", got, got)
		}
		if ext.Package != "example.com/pkg" || ext.Name != "Box" {
			t.Fatalf("ExternalRef payload mismatch: %+v", ext)
		}
		if len(ext.TypeArgs) != 1 {
			t.Fatalf("expected one type arg; got %v", ext.TypeArgs)
		}
	})
}

// TestConstraintFromNode covers the constraint lifter: nil and
// any-shape inputs lift to nil; embedded refs lift through
// FromNode preserving Raw.
func TestConstraintFromNode(t *testing.T) {
	t.Parallel()

	t.Run("nil receiver lifts to nil", func(t *testing.T) {
		t.Parallel()
		if got := refconv.ConstraintFromNode(nil); got != nil {
			t.Fatalf("nil constraint should lift to nil; got %v", got)
		}
	})

	t.Run("any-shape constraint lifts to nil", func(t *testing.T) {
		t.Parallel()
		c := &node.Constraint{}
		if got := refconv.ConstraintFromNode(c); got != nil {
			t.Fatalf("any-shape constraint should lift to nil; got %v", got)
		}
	})

	t.Run("embedded refs lift through FromNode preserving Raw", func(t *testing.T) {
		t.Parallel()
		c := &node.Constraint{
			Raw: "comparable",
			Embedded: []*node.TypeRef{
				{TypeKind: node.TypeRefNamed, Name: "comparable"},
			},
		}
		got := refconv.ConstraintFromNode(c)
		if got == nil {
			t.Fatalf("constraint with embedded ref should lift to non-nil")
		}
		if got.Raw != "comparable" {
			t.Fatalf("Raw = %q, want comparable", got.Raw)
		}
		if len(got.Embedded) != 1 {
			t.Fatalf("expected one embedded ref; got %v", got.Embedded)
		}
	})
}

// TestFromNode_FuncType pins that a func-typed reference converts
// structurally rather than falling through to an external reference.
//
// A func type names no package, so the fall-through built
// emit.External with an empty import path and the backend rejected
// it — aborting the whole render with "writer: empty import path"
// rather than emitting bad output. Callback parameters are ordinary:
// options, hooks, visitors.
func TestFromNode_FuncType(t *testing.T) {
	t.Parallel()

	fn := &node.TypeRef{
		TypeKind:    node.TypeRefFunc,
		FuncParams:  []*node.TypeRef{{TypeKind: node.TypeRefNamed, Name: "string"}},
		FuncReturns: []*node.TypeRef{{TypeKind: node.TypeRefNamed, Name: "error"}},
	}

	t.Run("converts to a func composite", func(t *testing.T) {
		t.Parallel()
		got, ok := refconv.FromNode(fn).(*emit.CompositeRef)
		if !ok {
			t.Fatalf("FromNode returned %T, want *emit.CompositeRef", refconv.FromNode(fn))
		}
		if got.Shape != emit.ShapeFunc {
			t.Fatalf("Shape = %v, want ShapeFunc", got.Shape)
		}
	})

	t.Run("carries its parameters and returns", func(t *testing.T) {
		t.Parallel()
		// A composite with the right shape but empty signature would
		// render `func()` and silently change the type.
		got := refconv.FromNode(fn).(*emit.CompositeRef) //nolint:forcetypeassert // pinned by the subtest above
		if len(got.FuncParams) != 1 || len(got.FuncReturns) != 1 {
			t.Fatalf("params=%d returns=%d, want 1 and 1", len(got.FuncParams), len(got.FuncReturns))
		}
	})

	t.Run("a func with no signature is still a func composite", func(t *testing.T) {
		t.Parallel()
		bare := &node.TypeRef{TypeKind: node.TypeRefFunc}
		got, ok := refconv.FromNode(bare).(*emit.CompositeRef)
		if !ok || got.Shape != emit.ShapeFunc {
			t.Fatalf("FromNode(bare func) = %T, want a ShapeFunc composite", refconv.FromNode(bare))
		}
	})
}
