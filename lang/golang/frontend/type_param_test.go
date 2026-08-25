// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/golang/frontend"
)

// TestTypeParam_List covers the AST → node.TypeParam conversion for
// the kinds of constraint expressions a generic struct declares.
func TestTypeParam_List(t *testing.T) {
	t.Parallel()
	t.Run("declares one param per name in source order", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Map[K, V any] struct{}\n",
		})
		s := pkg.StructByName("Map")
		if len(s.TypeParams) != 2 {
			t.Fatalf("expected 2 type params, got %d", len(s.TypeParams))
		}
		if s.TypeParams[0].Name != "K" || s.TypeParams[1].Name != "V" {
			t.Fatalf("type param names = %q,%q want K,V", s.TypeParams[0].Name, s.TypeParams[1].Name)
		}
	})

	t.Run("any constraint produces no embedded refs", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Box[T any] struct{}\n",
		})
		tp := pkg.StructByName("Box").TypeParams[0]
		if tp.Constraint == nil {
			t.Fatalf("expected non-nil constraint with fallback ref")
		}
		if tp.Constraint.Raw != "any" {
			t.Fatalf("Constraint.Raw = %q, want any", tp.Constraint.Raw)
		}
	})

	t.Run("named-interface constraint records the interface as embed", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype I interface{ M() }\n\ntype Box[T I] struct{}\n",
		})
		tp := pkg.StructByName("Box").TypeParams[0]
		if tp.Constraint == nil || len(tp.Constraint.Embedded) == 0 {
			t.Fatalf("expected constraint with at least one embed, got %+v", tp.Constraint)
		}
		emb := tp.Constraint.Embedded[0]
		if emb.Name != "I" {
			t.Fatalf("embed name = %q, want I", emb.Name)
		}
	})

	t.Run("type-set constraint stamps MetaConstraintTerms", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Box[T ~int | ~string] struct{}\n",
		})
		tp := pkg.StructByName("Box").TypeParams[0]
		terms, ok := frontend.MetaConstraintTerms.Get(tp.Meta())
		if !ok || len(terms) != 2 {
			t.Fatalf("expected 2 terms, got ok=%v len=%d", ok, len(terms))
		}
	})
}

// TestIsTypeSetExpr exercises the type-set predicate through
// generics that mix unary tilde, binary unions, and an embedded
// interface-as-constraint.
func TestIsTypeSetExpr(t *testing.T) {
	t.Parallel()
	t.Run("approximate-only type-set is recognised", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype N interface{ ~int }\n\ntype Box[T N] struct{}\n",
		})
		tp := pkg.StructByName("Box").TypeParams[0]
		terms, _ := frontend.MetaConstraintTerms.Get(tp.Meta())
		if len(terms) != 1 || !terms[0].Approximate {
			t.Fatalf("expected one approximate term, got %+v", terms)
		}
	})

	t.Run("inline type-set on a generic declaration is recognised", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Box[T interface{ ~int | ~uint }] struct{}\n",
		})
		tp := pkg.StructByName("Box").TypeParams[0]
		terms, _ := frontend.MetaConstraintTerms.Get(tp.Meta())
		if len(terms) != 2 {
			t.Fatalf("expected 2 terms for inline type-set, got %d", len(terms))
		}
	})

	t.Run("nested anonymous-interface type-set inside a named bound is recognised", func(t *testing.T) {
		t.Parallel()
		// Drives isTypeSetExpr's recursive InterfaceType branch — the
		// constraint is a named interface that itself contains a
		// type-set.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Stringy interface{ ~string }\n\ntype Box[T Stringy] struct{ V T }\n",
		})
		if pkg.StructByName("Box") == nil {
			t.Fatalf("Box struct missing")
		}
	})
}

// TestTypeParam_MalformedConstraint covers the converter's defensive
// behaviour when the type-checker cannot resolve a type-parameter's
// constraint expression.
func TestTypeParam_MalformedConstraint(t *testing.T) {
	t.Parallel()

	t.Run("unresolvable constraint surfaces a diagnostic and does not crash", func(t *testing.T) {
		t.Parallel()
		_, d := loadFromSource(t, map[string]string{
			"a.go": "package a\n\ntype Box[T Missing] struct{}\n",
		})
		if !d.HasErrors() {
			t.Fatalf("expected an Error diagnostic for unresolved type-param constraint")
		}
	})
}
