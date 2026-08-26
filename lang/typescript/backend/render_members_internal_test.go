// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/typescript"
)

func TestMemberFailurePropagates(t *testing.T) {
	t.Parallel()

	// Every member position that spells a type. A failure swallowed at
	// any of them renders the member with no annotation — `id;` rather
	// than `id: string;` — which is legal TypeScript declaring an
	// implicit `any`, so nothing downstream reports it.
	spellings := map[string]func(*renderState) (string, error){
		"a property's type": func(s *renderState) (string, error) {
			return s.renderMembers(&emit.Interface{
				Name:   "I",
				Fields: []*emit.Field{{Name: "id", Type: badRef()}},
			})
		},
		"a method's parameter": func(s *renderState) (string, error) {
			return s.renderMethods(&emit.Interface{
				Name: "I",
				Methods: []*emit.Method{{
					Name:   "m",
					Params: []*emit.Param{{Name: "x", Type: badRef()}},
				}},
			})
		},
		"a method's return": func(s *renderState) (string, error) {
			return s.renderMethods(&emit.Interface{
				Name: "I",
				Methods: []*emit.Method{{
					Name:    "m",
					Returns: []*emit.Return{{Type: badRef()}},
				}},
			})
		},
		"a method's type parameter bound": func(s *renderState) (string, error) {
			return s.renderMethods(&emit.Interface{
				Name: "I",
				Methods: []*emit.Method{{
					Name: "m",
					TypeParams: []*emit.TypeParam{{
						Name:       "T",
						Constraint: &emit.Constraint{Embedded: []emit.Ref{badRef()}},
					}},
				}},
			})
		},
		"a heritage clause": func(s *renderState) (string, error) {
			return s.renderHeritage(&emit.Interface{
				Name:   "I",
				Embeds: []*emit.Embed{{Type: badRef()}},
			})
		},
		"an enum member's value": func(s *renderState) (string, error) {
			return s.renderVariants(&emit.Enum{
				Name: "E",
				Variants: []*emit.EnumVariant{{
					Name:  "A",
					Value: &emit.Expr{ExprKind: emit.ExprCall},
				}},
			})
		},
	}

	for name, spell := range spellings {
		t.Run(name+" surfaces", func(t *testing.T) {
			t.Parallel()
			got, err := spell(exprState(t))
			if err == nil {
				t.Fatalf("rendered %q rather than reporting the failure", got)
			}
		})
	}
}

func TestMemberModifiers(t *testing.T) {
	t.Parallel()

	t.Run("a readonly optional property carries both markers", func(t *testing.T) {
		t.Parallel()
		// `?` and `| undefined` are different claims — an optional
		// property may be absent from the object entirely — so the
		// marker is what survives exactOptionalPropertyTypes.
		f := &emit.Field{Name: "kind", Type: emit.Builtin("string")}
		typescript.MetaReadonly.Set(f.EnsureMeta(), true, "test")
		typescript.MetaOptional.Set(f.EnsureMeta(), true, "test")

		got, err := exprState(t).member(f)
		if err != nil {
			t.Fatalf("member: %v", err)
		}
		if got != "readonly kind?: string;\n" {
			t.Fatalf("member = %q", got)
		}
	})

	t.Run("an async method is spelled", func(t *testing.T) {
		t.Parallel()
		m := &emit.Method{Name: "load", Returns: []*emit.Return{{Type: emit.Builtin("string")}}}
		typescript.MetaAsync.Set(m.EnsureMeta(), true, "test")

		got, err := exprState(t).method(m)
		if err != nil {
			t.Fatalf("method: %v", err)
		}
		if got != "async load(): string;\n" {
			t.Fatalf("method = %q", got)
		}
	})

	t.Run("a member with no type renders no annotation", func(t *testing.T) {
		t.Parallel()
		// Legal TypeScript: the compiler infers it. A generator that
		// declared no type meant that.
		got, err := exprState(t).member(&emit.Field{Name: "id"})
		if err != nil {
			t.Fatalf("member: %v", err)
		}
		if got != "id;\n" {
			t.Fatalf("member = %q", got)
		}
	})

	t.Run("a method returning nothing is void", func(t *testing.T) {
		t.Parallel()
		// Spelled rather than omitted: a signature with no return
		// annotation has its type inferred, and an implementation that
		// returned something would silently widen the contract.
		got, err := exprState(t).method(&emit.Method{Name: "reset"})
		if err != nil {
			t.Fatalf("method: %v", err)
		}
		if got != "reset(): void;\n" {
			t.Fatalf("method = %q", got)
		}
	})

	t.Run("an enum member with no value renders bare", func(t *testing.T) {
		t.Parallel()
		got, err := exprState(t).renderVariants(&emit.Enum{
			Name:     "E",
			Variants: []*emit.EnumVariant{{Name: "A"}},
		})
		if err != nil {
			t.Fatalf("renderVariants: %v", err)
		}
		if got != "A,\n" {
			t.Fatalf("renderVariants = %q", got)
		}
	})
}

func TestArrayOf(t *testing.T) {
	t.Parallel()

	t.Run("a compound element is parenthesised", func(t *testing.T) {
		t.Parallel()
		// `A | B[]` binds as `A | (B[])`, which is a different type.
		if got := arrayOf("A | B"); got != "(A | B)[]" {
			t.Errorf("arrayOf = %q", got)
		}
		if got := arrayOf("string"); got != "string[]" {
			t.Errorf("arrayOf = %q", got)
		}
	})
}

func TestConstraintOf(t *testing.T) {
	t.Parallel()

	t.Run("several bounds become an intersection", func(t *testing.T) {
		t.Parallel()
		// TypeScript's `extends` takes one type, and a parameter
		// required to satisfy two is bounded by the type that is both.
		got, err := exprState(t).constraintOf(&emit.TypeParam{
			Name: "T",
			Constraint: &emit.Constraint{Embedded: []emit.Ref{
				emit.External("./a", "A"), emit.External("./b", "B"),
			}},
		})
		if err != nil {
			t.Fatalf("constraintOf: %v", err)
		}
		if got != "A & B" {
			t.Fatalf("constraintOf = %q", got)
		}
	})

	t.Run("an unbounded parameter spells nothing", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		for _, p := range []*emit.TypeParam{
			{Name: "T"},
			{Name: "T", Constraint: &emit.Constraint{}},
		} {
			if got, err := s.constraintOf(p); got != "" || err != nil {
				t.Errorf("constraintOf = %q, %v", got, err)
			}
		}
	})
}
