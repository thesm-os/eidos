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

// TestFromNode_AnonStruct covers the anonymous-struct arm.
//
// Without it the ref fell through to the named-reference fallback and
// produced an ExternalRef with an empty Package, which the backend
// rejects outright — `map[string]struct{}`, the idiomatic Go set,
// aborted the whole render rather than degrading its output. The
// assertions below are on the *lifted shape* rather than on rendered
// text, so they fail for the original reason even if the backend
// later spells the type differently.
func TestFromNode_AnonStruct(t *testing.T) {
	t.Parallel()

	t.Run("an empty anonymous struct lifts to a composite, not an external ref", func(t *testing.T) {
		t.Parallel()
		got := refconv.FromNode(&node.TypeRef{TypeKind: node.TypeRefAnonStruct})
		comp, ok := got.(*emit.CompositeRef)
		if !ok {
			t.Fatalf("FromNode(struct{}) = %T, want *emit.CompositeRef", got)
		}
		if comp.Shape != emit.ShapeAnonStruct {
			t.Fatalf("Shape = %v, want ShapeAnonStruct", comp.Shape)
		}
	})

	t.Run("no arm means an empty import path, which is the bug", func(t *testing.T) {
		t.Parallel()
		// The fallback builds emit.External("", "") and the backend
		// answers with writer.ErrEmptyPath. Pinning the negative keeps
		// a future refactor from quietly restoring the fall-through.
		got := refconv.FromNode(&node.TypeRef{TypeKind: node.TypeRefAnonStruct})
		if ext, ok := got.(*emit.ExternalRef); ok {
			t.Fatalf("anon struct must not lift to an ExternalRef; got package %q name %q",
				ext.Package, ext.Name)
		}
	})

	t.Run("inline fields carry name, type and tag in order", func(t *testing.T) {
		t.Parallel()
		src := &node.TypeRef{
			TypeKind: node.TypeRefAnonStruct,
			Fields: []*node.Field{
				{Name: "A", Type: &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "int"}},
				{Name: "B", Type: &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"}, Tag: `json:"b"`},
			},
		}
		got, ok := refconv.FromNode(src).(*emit.CompositeRef)
		if !ok {
			t.Fatalf("FromNode returned %T, want *emit.CompositeRef", refconv.FromNode(src))
		}
		if len(got.StructFields) != 2 {
			t.Fatalf("StructFields len = %d, want 2", len(got.StructFields))
		}
		if got.StructFields[0].Name != "A" || got.StructFields[1].Name != "B" {
			t.Fatalf("field order not preserved; got %+v", got.StructFields)
		}
		if got.StructFields[1].Tag != `json:"b"` {
			// Tags are part of Go's struct type identity: dropping one
			// makes two distinct types render identically.
			t.Fatalf("tag = %q, want json:\"b\"", got.StructFields[1].Tag)
		}
		if got.StructFields[0].Type == nil || got.StructFields[1].Type == nil {
			t.Fatalf("field types must be lifted, not dropped")
		}
	})

	t.Run("embedded types are lifted in order", func(t *testing.T) {
		t.Parallel()
		src := &node.TypeRef{
			TypeKind: node.TypeRefAnonStruct,
			Embeds: []*node.Embed{
				{Type: &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "error"}},
			},
		}
		got, ok := refconv.FromNode(src).(*emit.CompositeRef)
		if !ok {
			t.Fatalf("FromNode returned %T, want *emit.CompositeRef", refconv.FromNode(src))
		}
		if len(got.StructEmbeds) != 1 {
			t.Fatalf("StructEmbeds len = %d, want 1", len(got.StructEmbeds))
		}
		if b, ok := got.StructEmbeds[0].(*emit.BuiltinRef); !ok || b.Name != "error" {
			t.Fatalf("embed = %+v, want BuiltinRef{error}", got.StructEmbeds[0])
		}
	})

	t.Run("a nested anonymous struct lifts recursively", func(t *testing.T) {
		t.Parallel()
		src := &node.TypeRef{
			TypeKind: node.TypeRefMap,
			MapKey:   &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"},
			MapValue: &node.TypeRef{TypeKind: node.TypeRefAnonStruct},
		}
		got, ok := refconv.FromNode(src).(*emit.CompositeRef)
		if !ok {
			t.Fatalf("FromNode returned %T, want *emit.CompositeRef", refconv.FromNode(src))
		}
		val, ok := got.MapValue.(*emit.CompositeRef)
		if !ok || val.Shape != emit.ShapeAnonStruct {
			t.Fatalf("map value = %+v, want a ShapeAnonStruct composite", got.MapValue)
		}
	})

	t.Run("the origin node is threaded onto the lifted ref", func(t *testing.T) {
		t.Parallel()
		src := &node.TypeRef{TypeKind: node.TypeRefAnonStruct}
		if got := refconv.FromNode(src).Origin(); got != node.Node(src) {
			t.Fatalf("Origin = %v, want the source ref", got)
		}
	})
}

// The lift is total. Eight accessors in this package answer nil for
// "not applicable", four lifts here read a field that may be absent,
// and every crossing between them used to panic — which is why the
// first real consumer wrote a private guard rather than using them.
func TestFromNodeIsTotal(t *testing.T) {
	t.Parallel()

	t.Run("a nil reference lifts to a nil ref", func(t *testing.T) {
		t.Parallel()
		if got := refconv.FromNode(nil); got != nil {
			t.Fatalf("FromNode(nil) = %v, want nil", got)
		}
	})

	t.Run("composes with the accessors that answer nil", func(t *testing.T) {
		t.Parallel()
		// Each of these is the natural pairing a generator writes, and
		// each panicked before: the accessor's "not applicable" answer
		// was an argument the lift refused.
		notAPointer := namedTypeRef("x", "User")
		for name, got := range map[string]emit.Ref{
			"PointerElem on a non-pointer": refconv.FromNode(refconv.PointerElem(notAPointer)),
			"SliceElem on a non-slice":     refconv.FromNode(refconv.SliceElem(notAPointer)),
			"MapKey on a non-map":          refconv.FromNode(refconv.MapKey(notAPointer)),
			"IteratorElem on a non-seq":    refconv.FromNode(refconv.IteratorElem(notAPointer)),
			"IteratorSecond on a non-seq":  refconv.FromNode(refconv.IteratorSecond(notAPointer)),
		} {
			if got != nil {
				t.Fatalf("%s = %v, want nil", name, got)
			}
		}
	})

	t.Run("the emit-side lifts are total too", func(t *testing.T) {
		t.Parallel()
		// They read the field directly, so a type that carries none —
		// or a field the model recorded without one — reached the lift
		// as nil. An untyped field is reachable: a fixture builds one.
		notASlice := namedTypeRef("x", "User")
		if got := refconv.ElemType(notASlice); got != nil {
			t.Fatalf("ElemType on a non-slice = %v, want nil", got)
		}
		if got := refconv.MapKeyType(notASlice); got != nil {
			t.Fatalf("MapKeyType on a non-map = %v, want nil", got)
		}
		if got := refconv.FieldType(&node.Field{Name: "Untyped"}); got != nil {
			t.Fatalf("FieldType on an untyped field = %v, want nil", got)
		}
	})
}
