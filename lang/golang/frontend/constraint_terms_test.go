// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/golang/frontend"
)

// TestConstraintTermsFromUnion exercises the union → ConstraintTerm
// conversion via the public Load surface. A generic type parameter
// constrained by `~int | ~string` produces two terms with Approximate=true
// and the expected built-in names.
func TestConstraintTermsFromUnion(t *testing.T) {
	t.Parallel()
	t.Run("approximate type-set produces tilde terms in order", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Stringable interface{ ~string | ~int }\n\ntype Box[T Stringable] struct{ V T }\n",
		})
		box := pkg.StructByName("Box")
		if box == nil || len(box.TypeParams) != 1 {
			t.Fatalf("Box[T Stringable] missing")
		}
		tp := box.TypeParams[0]
		terms, ok := frontend.MetaConstraintTerms.Get(tp.Meta())
		if !ok || len(terms) != 2 {
			t.Fatalf("expected 2 constraint terms, got ok=%v len=%d", ok, len(terms))
		}
		want := []struct {
			name        string
			approximate bool
		}{
			{"string", true},
			{"int", true},
		}
		for i, w := range want {
			if terms[i].Type == nil {
				t.Fatalf("term %d type is nil", i)
				continue
			}
			if terms[i].Type.Name != w.name {
				t.Errorf("term %d name = %q, want %q", i, terms[i].Type.Name, w.name)
			}
			if terms[i].Approximate != w.approximate {
				t.Errorf("term %d approximate = %v, want %v", i, terms[i].Approximate, w.approximate)
			}
		}
	})

	t.Run("exact type-set produces non-tilde terms", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Choice interface{ int | string }\n\ntype Box[T Choice] struct{}\n",
		})
		tp := pkg.StructByName("Box").TypeParams[0]
		terms, _ := frontend.MetaConstraintTerms.Get(tp.Meta())
		for i, term := range terms {
			if term.Approximate {
				t.Errorf("term %d unexpectedly Approximate", i)
			}
		}
	})

	t.Run("comparable constraint stamps no terms", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Box[T comparable] struct{}\n",
		})
		tp := pkg.StructByName("Box").TypeParams[0]
		if frontend.MetaConstraintTerms.Has(tp.Meta()) {
			t.Fatalf("comparable constraint must not stamp MetaConstraintTerms")
		}
	})
}

// TestMetaConstraintTerms_DirectiveParser drives the registered
// typed parser via SetDirectiveFromString. The path is exercised by
// the pipeline's directive-override step; here we ensure the
// parser registers cleanly and decodes the JSON wire form into the
// expected [ConstraintTerm] slice.
func TestMetaConstraintTerms_DirectiveParser(t *testing.T) {
	t.Parallel()
	t.Run("JSON wire form round-trips through the parser", func(t *testing.T) {
		t.Parallel()
		// Stamp a value via SetDirectiveFromString to exercise the
		// parser path. The value bytes match the JSON encoding the
		// converter would emit.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Box[T any] struct{}\n",
		})
		tp := pkg.StructByName("Box").TypeParams[0]
		raw := `[{"type":{"Name":"int"},"approximate":true}]`
		if err := frontend.MetaConstraintTerms.SetDirectiveFromString(tp.EnsureMeta(), raw, tp.Pos()); err != nil {
			t.Fatalf("SetDirectiveFromString: %v", err)
		}
		terms, ok := frontend.MetaConstraintTerms.Get(tp.Meta())
		if !ok || len(terms) != 1 {
			t.Fatalf("expected one term after directive set, got ok=%v len=%d", ok, len(terms))
		}
		if !terms[0].Approximate || terms[0].Type.Name != "int" {
			t.Fatalf("decoded term = %+v", terms[0])
		}
	})

	t.Run("empty input yields nil terms", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Box[T any] struct{}\n",
		})
		tp := pkg.StructByName("Box").TypeParams[0]
		if err := frontend.MetaConstraintTerms.SetDirectiveFromString(tp.EnsureMeta(), "", tp.Pos()); err != nil {
			t.Fatalf("SetDirectiveFromString empty: %v", err)
		}
		terms, ok := frontend.MetaConstraintTerms.Get(tp.Meta())
		if !ok || terms != nil {
			t.Fatalf("expected nil terms for empty input, got ok=%v terms=%+v", ok, terms)
		}
	})

	t.Run("malformed JSON surfaces a wrapped parse error", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Box[T any] struct{}\n",
		})
		tp := pkg.StructByName("Box").TypeParams[0]
		err := frontend.MetaConstraintTerms.SetDirectiveFromString(tp.EnsureMeta(), "{not json", tp.Pos())
		if err == nil {
			t.Fatalf("expected error for malformed JSON")
		}
	})
}

// TestConstraintRaw verifies the source-printed constraint form
// preserves the original expression for downstream generators.
func TestConstraintRaw(t *testing.T) {
	t.Parallel()
	t.Run("raw stores the source-level constraint text", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Box[T any] struct{}\n",
		})
		tp := pkg.StructByName("Box").TypeParams[0]
		if tp.Constraint == nil {
			t.Fatalf("Constraint missing")
		}
		if tp.Constraint.Raw != "any" {
			t.Fatalf("Constraint.Raw = %q, want %q", tp.Constraint.Raw, "any")
		}
	})
}
