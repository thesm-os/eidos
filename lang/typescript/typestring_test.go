// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

func TestTypeString(t *testing.T) {
	t.Parallel()

	t.Run("parenthesises an array of union", func(t *testing.T) {
		t.Parallel()
		// `A | B[]` binds as `A | (B[])`, which is a different type.
		ref := &node.TypeRef{
			TypeKind: node.TypeRefSlice,
			Elem:     marker(typescript.RefUnion, named("A"), named("B")),
		}
		if got := typescript.TypeString(ref); got != "(A | B)[]" {
			t.Fatalf("TypeString = %q", got)
		}
	})

	t.Run("spells an operator type from its recorded text", func(t *testing.T) {
		t.Parallel()
		op := marker(typescript.RefOperator)
		typescript.MetaTypeText.Set(op.EnsureMeta(), "T extends string ? A : B", "test")
		if got := typescript.TypeString(op); got != "T extends string ? A : B" {
			t.Fatalf("TypeString = %q", got)
		}
	})

	t.Run("spells a tuple and an intersection", func(t *testing.T) {
		t.Parallel()
		tuple := marker(typescript.RefTuple, named("string"), named("number"))
		if got := typescript.TypeString(tuple); got != "[string, number]" {
			t.Errorf("tuple = %q", got)
		}
		inter := marker(typescript.RefIntersection, named("A"), named("B"))
		if got := typescript.TypeString(inter); got != "A & B" {
			t.Errorf("intersection = %q", got)
		}
	})

	t.Run("spells a function type with positional names", func(t *testing.T) {
		t.Parallel()
		fn := &node.TypeRef{
			TypeKind:    node.TypeRefFunc,
			FuncParams:  []*node.TypeRef{named("string")},
			FuncReturns: []*node.TypeRef{named("boolean")},
		}
		if got := typescript.TypeString(fn); got != "(arg0: string) => boolean" {
			t.Fatalf("TypeString = %q", got)
		}
	})

	t.Run("spells an inline object with its members", func(t *testing.T) {
		t.Parallel()
		// An inline object is identified by its members; eliding them
		// tells a reader nothing the context did not already say.
		opt := &node.Field{Name: "b", Type: named("number")}
		typescript.MetaOptional.Set(opt.EnsureMeta(), true, "test")

		obj := &node.TypeRef{
			TypeKind: node.TypeRefAnonStruct,
			Fields:   []*node.Field{{Name: "a", Type: named("string")}, opt},
		}
		if got := typescript.TypeString(obj); got != "{ a: string; b?: number }" {
			t.Fatalf("TypeString = %q", got)
		}
	})

	t.Run("a qualified name keeps its module", func(t *testing.T) {
		t.Parallel()
		ref := &node.TypeRef{TypeKind: node.TypeRefNamed, Package: "./models", Name: "User"}
		if got := typescript.TypeString(ref); got != "./models.User" {
			t.Fatalf("TypeString = %q", got)
		}
	})

	t.Run("an empty union is never and nil is empty", func(t *testing.T) {
		t.Parallel()
		if got := typescript.TypeString(marker(typescript.RefUnion)); got != typescript.TypeNever {
			t.Errorf("empty union = %q, want never", got)
		}
		// Empty rather than a placeholder: a caller interpolating the
		// result needs no guard, and an empty type in a message is
		// visibly wrong where a partial one reads as correct.
		if got := typescript.TypeString(nil); got != "" {
			t.Errorf("TypeString(nil) = %q, want empty", got)
		}
	})

	t.Run("a type nested past the budget renders as nothing", func(t *testing.T) {
		t.Parallel()
		// Empty rather than truncated. A rendering cut off at the
		// budget would be a different type that reads as correct —
		// `string[][]` where the source said sixteen more levels — and
		// a caller interpolating it into a message would report the
		// wrong thing confidently. Empty is visibly wrong.
		deep := named("string")
		for range 20 {
			deep = &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: deep}
		}
		if got := typescript.TypeString(deep); got != "" {
			t.Fatalf("TypeString = %q, want empty", got)
		}

		// A type within the budget still renders in full, so the
		// bound is not simply refusing everything nested.
		shallow := &node.TypeRef{
			TypeKind: node.TypeRefSlice,
			Elem:     &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: named("string")},
		}
		if got := typescript.TypeString(shallow); !strings.Contains(got, "string[][]") {
			t.Fatalf("TypeString = %q, want string[][]", got)
		}
	})
}

func TestTypeStringRemainingShapes(t *testing.T) {
	t.Parallel()

	t.Run("spells a pointer as nullable and a nil element as nothing", func(t *testing.T) {
		t.Parallel()
		ptr := &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: named("string")}
		if got := typescript.TypeString(ptr); got != "string | null" {
			t.Errorf("pointer = %q", got)
		}
		empty := &node.TypeRef{TypeKind: node.TypeRefPointer}
		if got := typescript.TypeString(empty); got != "" {
			t.Errorf("pointer with no element = %q, want empty", got)
		}
	})

	t.Run("spells a fixed-length array as an array", func(t *testing.T) {
		t.Parallel()
		// TypeScript's only fixed-length sequence is a tuple, and a
		// tuple of N identical elements is not what an array of N
		// means to the languages that distinguish them.
		arr := &node.TypeRef{TypeKind: node.TypeRefArray, ArrayLen: 3, Elem: named("number")}
		if got := typescript.TypeString(arr); got != "number[]" {
			t.Fatalf("array = %q", got)
		}
	})

	t.Run("spells an empty and an anonymous interface", func(t *testing.T) {
		t.Parallel()
		empty := &node.TypeRef{TypeKind: node.TypeRefAnonStruct}
		if got := typescript.TypeString(empty); got != "{}" {
			t.Errorf("empty object = %q", got)
		}
		iface := &node.TypeRef{TypeKind: node.TypeRefAnonInterface}
		if got := typescript.TypeString(iface); got != "object" {
			t.Errorf("anon interface = %q", got)
		}
	})

	t.Run("spells a function with no return and with several", func(t *testing.T) {
		t.Parallel()
		none := &node.TypeRef{TypeKind: node.TypeRefFunc}
		if got := typescript.TypeString(none); got != "() => void" {
			t.Errorf("no return = %q", got)
		}
		several := &node.TypeRef{
			TypeKind:    node.TypeRefFunc,
			FuncReturns: []*node.TypeRef{named("string"), named("Error")},
		}
		if got := typescript.TypeString(several); got != "() => [string, Error]" {
			t.Errorf("several returns = %q", got)
		}
	})

	t.Run("an intersection of one member needs no parentheses in an array", func(t *testing.T) {
		t.Parallel()
		// `A & B[]` binds as `A & (B[])`, the same trap the union case
		// has.
		inter := marker(typescript.RefIntersection, named("A"), named("B"))
		arr := &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: inter}
		if got := typescript.TypeString(arr); got != "(A & B)[]" {
			t.Fatalf("array of intersection = %q", got)
		}
	})

	t.Run("an empty tuple and an empty intersection", func(t *testing.T) {
		t.Parallel()
		if got := typescript.TypeString(marker(typescript.RefTuple)); got != "[]" {
			t.Errorf("empty tuple = %q", got)
		}
		if got := typescript.TypeString(marker(typescript.RefIntersection)); got != typescript.TypeNever {
			t.Errorf("empty intersection = %q, want never", got)
		}
	})
}
