// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/lang/typescript/typescripttest/tsfixture"
	"go.thesmos.sh/eidos/node"
)

func TestTSSourceTypes(t *testing.T) {
	t.Parallel()

	t.Run("each shape spells what TypeScript would", func(t *testing.T) {
		t.Parallel()
		for want, ref := range map[string]*node.TypeRef{
			"string":                    tsfixture.Named("string"),
			"T":                         tsfixture.TypeParamRef("T"),
			"Box<string>":               tsfixture.WithArgs(tsfixture.Named("Box"), tsfixture.Named("string")),
			"string[]":                  tsfixture.Array(tsfixture.Named("string")),
			"Record<string, number>":    tsfixture.Record(tsfixture.Named("string"), tsfixture.Named("number")),
			"A | B":                     tsfixture.Union(tsfixture.Named("A"), tsfixture.Named("B")),
			"A & B":                     tsfixture.Intersection(tsfixture.Named("A"), tsfixture.Named("B")),
			"[A, B]":                    tsfixture.Tuple(tsfixture.Named("A"), tsfixture.Named("B")),
			"string | null":             tsfixture.Nullable(tsfixture.Named("string")),
			"string | undefined":        tsfixture.Undefinable(tsfixture.Named("string")),
			"keyof T":                   tsfixture.Operator("keyof T"),
			"'admin'":                   tsfixture.Literal("'admin'"),
			"{}":                        tsfixture.Object(),
			"object":                    {TypeKind: node.TypeRefAnonInterface},
			"(arg0: string) => boolean": tsfixture.Func(refs("string"), refs("boolean")),
			"() => void":                tsfixture.Func(nil, nil),
			"() => [string, number]":    tsfixture.Func(nil, refs("string", "number")),
		} {
			t.Run(want, func(t *testing.T) {
				t.Parallel()
				assertType(t, ref, want)
			})
		}
	})

	t.Run("an inline object lists its members", func(t *testing.T) {
		t.Parallel()
		assertType(t, tsfixture.Object(
			tsfixture.Prop("a", tsfixture.Named("string")),
			tsfixture.Prop("b", tsfixture.Named("number")),
		), "{ a: string; b: number }")
	})

	t.Run("a compound element is parenthesised in an array", func(t *testing.T) {
		t.Parallel()
		// `A | B[]` binds as `A | (B[])`, which is a different type.
		assertType(t, tsfixture.Array(tsfixture.Union(
			tsfixture.Named("A"), tsfixture.Named("B"),
		)), "(A | B)[]")
		assertType(t, tsfixture.Array(tsfixture.Func(nil, nil)), "(() => void)[]")
	})

	t.Run("an empty union and intersection are never", func(t *testing.T) {
		t.Parallel()
		assertType(t, tsfixture.Union(), "never")
		assertType(t, tsfixture.Intersection(), "never")
		assertType(t, tsfixture.Tuple(), "[]")
	})

	t.Run("a pointer from another language's graph is nullable", func(t *testing.T) {
		t.Parallel()
		// Spelled rather than refused, because that is the projection
		// the backend makes and a support file disagreeing with it
		// would not type-check against the output.
		assertType(t, &node.TypeRef{
			TypeKind: node.TypeRefPointer, Elem: tsfixture.Named("string"),
		}, "string | null")
	})

	t.Run("an array kind renders as an array", func(t *testing.T) {
		t.Parallel()
		// TypeScript's only fixed-length sequence is a tuple, and a
		// tuple of N identical elements is not what an array of N means
		// to the languages that distinguish them.
		assertType(t, &node.TypeRef{
			TypeKind: node.TypeRefArray, ArrayLen: 3, Elem: tsfixture.Named("number"),
		}, "number[]")
	})

	t.Run("a property key that is not an identifier is quoted", func(t *testing.T) {
		t.Parallel()
		assertType(t, tsfixture.Object(
			tsfixture.Prop("content-type", tsfixture.Named("string")),
		), "{ 'content-type': string }")
	})
}

func TestTSSourceTypeRefusals(t *testing.T) {
	t.Parallel()

	t.Run("each unspellable type names itself", func(t *testing.T) {
		t.Parallel()
		for name, ref := range map[string]*node.TypeRef{
			"a declaration with no type":        nil,
			"a named type with no name":         {TypeKind: node.TypeRefNamed},
			"an operator carrying no text":      bareOperator(),
			"a marker this package does not":    bareMarker(),
			"a kind this model does not name":   {TypeKind: node.TypeRefKind(99)},
			"an object member with no name":     tsfixture.Object(tsfixture.Prop("", tsfixture.Named("string"))),
			"a type nested past the projection": deep(),
		} {
			t.Run(name+" is refused", func(t *testing.T) {
				t.Parallel()
				assertUnspellable(t, func() {
					tsfixture.New().Alias("X", func(a *tsfixture.AliasBuilder) {
						a.Target(ref)
					}).TSSource()
				})
			})
		}
	})
}

// assertType renders a type through an alias and checks the spelling.
func assertType(t *testing.T, ref *node.TypeRef, want string) {
	t.Helper()
	_, src := tsfixture.New().Alias("X", func(a *tsfixture.AliasBuilder) {
		a.Target(ref)
	}).TSSource()
	assertLine(t, src, "export type X = "+want+";")
}

// refs builds a list of named references.
func refs(names ...string) []*node.TypeRef {
	out := make([]*node.TypeRef, 0, len(names))
	for _, n := range names {
		out = append(out, tsfixture.Named(n))
	}
	return out
}

// bareOperator returns an operator marker with no recorded text,
// which is what a bridge writing the `ts` qualifier itself produces.
func bareOperator() *node.TypeRef {
	op := tsfixture.Tuple()
	op.Name = typescript.RefOperator
	return op
}

// bareMarker returns a structural marker this package does not
// declare, which a bridge writing the `ts` qualifier by hand produces.
func bareMarker() *node.TypeRef {
	m := tsfixture.Union()
	m.Name = "invented"
	return m
}

// deep returns a type nested past the projection's budget.
func deep() *node.TypeRef {
	t := tsfixture.Named("string")
	for range 40 {
		t = tsfixture.Array(t)
	}
	return t
}

func TestBoundProjection(t *testing.T) {
	t.Parallel()

	t.Run("a raw bound spells its text", func(t *testing.T) {
		t.Parallel()
		// The projection renders what the author wrote rather than
		// reconstructing from references it does not have.
		_, src := tsfixture.New().Alias("Box", func(a *tsfixture.AliasBuilder) {
			a.TypeParam("T", tsfixture.Bound("keyof U", tsfixture.Named("U"))).
				Target(tsfixture.TypeParamRef("T"))
		}).TSSource()
		assertLine(t, src, "export type Box<T extends U> = T;")
	})

	t.Run("the empty object type spells object", func(t *testing.T) {
		t.Parallel()
		assertType(t, tsfixture.AnonObject(), "object")
	})
}
