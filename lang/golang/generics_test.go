// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// params builds a type-parameter list from bare names.
func params(names ...string) []*node.TypeParam {
	out := make([]*node.TypeParam, 0, len(names))
	for _, n := range names {
		out = append(out, &node.TypeParam{Name: n})
	}
	return out
}

// TestTypeParamsOf pins that every generic-capable kind is
// reachable through one entry point.
//
// Five node kinds carry a type-parameter list and the rendering is
// identical for all five. Keyed on the container, a consumer
// generating over interfaces cannot reuse a struct-shaped helper
// and writes its own — which is how one downstream generator ended
// up with three copies of this.
func TestTypeParamsOf(t *testing.T) {
	t.Parallel()

	t.Run("reaches every generic-capable kind", func(t *testing.T) {
		t.Parallel()
		p := params("T")
		for name, n := range map[string]node.Node{
			"Struct":    &node.Struct{Name: "S", TypeParams: p},
			"Interface": &node.Interface{Name: "I", TypeParams: p},
			"Function":  &node.Function{Name: "F", TypeParams: p},
			"Method":    &node.Method{Name: "M", TypeParams: p},
			"Alias":     &node.Alias{Name: "A", TypeParams: p},
		} {
			if got := golang.TypeParamsOf(n); len(got) != 1 {
				t.Errorf("TypeParamsOf(%s) = %v, want one parameter", name, got)
			}
		}
	})

	t.Run("a non-generic declaration carries none", func(t *testing.T) {
		t.Parallel()
		if got := golang.TypeParamsOf(&node.Struct{Name: "S"}); got != nil {
			t.Fatalf("TypeParamsOf = %v, want nil", got)
		}
	})

	t.Run("a kind that cannot be generic carries none", func(t *testing.T) {
		t.Parallel()
		if got := golang.TypeParamsOf(&node.Field{Name: "ID"}); got != nil {
			t.Fatalf("TypeParamsOf(Field) = %v, want nil", got)
		}
	})

	t.Run("a nil declaration carries none", func(t *testing.T) {
		t.Parallel()
		if got := golang.TypeParamsOf(nil); got != nil {
			t.Fatalf("TypeParamsOf(nil) = %v, want nil", got)
		}
	})
}

func TestIsGeneric(t *testing.T) {
	t.Parallel()

	t.Run("a parameterised interface is generic", func(t *testing.T) {
		t.Parallel()
		// The struct-only helper could not answer this, which is the
		// gap that forced a second copy downstream.
		if !golang.IsGeneric(&node.Interface{Name: "I", TypeParams: params("T")}) {
			t.Fatalf("a parameterised interface must read as generic")
		}
	})

	t.Run("a plain struct is not generic", func(t *testing.T) {
		t.Parallel()
		if golang.IsGeneric(&node.Struct{Name: "S"}) {
			t.Fatalf("a plain struct must not read as generic")
		}
	})
}

func TestTypeParamDecls(t *testing.T) {
	t.Parallel()

	t.Run("lifts each parameter in order", func(t *testing.T) {
		t.Parallel()
		got := golang.TypeParamDecls(params("K", "V"))
		if len(got) != 2 || got[0].Name != "K" || got[1].Name != "V" {
			t.Fatalf("TypeParamDecls = %+v, want [K V]", got)
		}
	})

	t.Run("carries the constraint through", func(t *testing.T) {
		t.Parallel()
		p := []*node.TypeParam{{
			Name:       "K",
			Constraint: &node.Constraint{Embedded: []*node.TypeRef{{TypeKind: node.TypeRefNamed, Name: "comparable"}}},
		}}
		if got := golang.TypeParamDecls(p); got[0].Constraint == nil {
			t.Fatalf("TypeParamDecls dropped the constraint")
		}
	})

	t.Run("an empty list lifts to nil", func(t *testing.T) {
		t.Parallel()
		// An empty-but-non-nil slice renders `[]`, which does not
		// compile; nil is what makes the template emit nothing.
		if got := golang.TypeParamDecls(nil); got != nil {
			t.Fatalf("TypeParamDecls(nil) = %+v, want nil", got)
		}
	})
}

func TestTypeParamNames(t *testing.T) {
	t.Parallel()

	t.Run("renders the use spelling", func(t *testing.T) {
		t.Parallel()
		if got := golang.TypeParamNames(params("K", "V")); got != "[K, V]" {
			t.Fatalf("TypeParamNames = %q, want [K, V]", got)
		}
	})

	t.Run("a non-generic list renders nothing", func(t *testing.T) {
		t.Parallel()
		if got := golang.TypeParamNames(nil); got != "" {
			t.Fatalf("TypeParamNames(nil) = %q, want empty", got)
		}
	})

	t.Run("agrees with the concrete spelling on separator", func(t *testing.T) {
		t.Parallel()
		// The two appear side by side in one generated file, so a
		// difference reads as a bug in the generator.
		if a, b := golang.TypeParamNames(params("K", "V")), golang.Instantiation("K", "V"); a != b {
			t.Fatalf("use = %q, concrete = %q; the spellings must agree", a, b)
		}
	})
}

func TestTypeParamRefs(t *testing.T) {
	t.Parallel()

	t.Run("renders each parameter as a bare identifier", func(t *testing.T) {
		t.Parallel()
		// Inside the declaration's own scope the parameter names
		// itself; resolving it to a package-qualified type would name
		// something that does not exist.
		got := golang.TypeParamRefs(params("T"))
		b, ok := got[0].(*emit.BuiltinRef)
		if !ok || b.Name != "T" {
			t.Fatalf("TypeParamRefs = %#v, want BuiltinRef{T}", got[0])
		}
	})

	t.Run("an empty list yields no refs", func(t *testing.T) {
		t.Parallel()
		if got := golang.TypeParamRefs(nil); got != nil {
			t.Fatalf("TypeParamRefs(nil) = %v, want nil", got)
		}
	})
}

func TestSelfRef(t *testing.T) {
	t.Parallel()

	t.Run("a non-generic type names itself bare", func(t *testing.T) {
		t.Parallel()
		got, ok := golang.SelfRef("example.com/x", "Store", nil).(*emit.ExternalRef)
		if !ok || got.Name != "Store" || len(got.TypeArgs) != 0 {
			t.Fatalf("SelfRef = %#v, want a bare ExternalRef", got)
		}
	})

	t.Run("a generic type carries its parameters", func(t *testing.T) {
		t.Parallel()
		// A generic type referenced bare does not compile, and a
		// helper naming one would fail at the point of use rather
		// than where the generator made the choice.
		got, ok := golang.SelfRef("example.com/x", "Container", params("T", "K")).(*emit.ExternalRef)
		if !ok || len(got.TypeArgs) != 2 {
			t.Fatalf("SelfRef = %#v, want two type args", got)
		}
	})
}

// TestContainerHelpersDelegate pins that the published
// struct-shaped entry points and the parameter-keyed ones cannot
// diverge.
//
// Keeping both is what lets existing callers compile; keeping two
// implementations is what let a downstream consumer end up with
// three copies of the same rendering.
func TestContainerHelpersDelegate(t *testing.T) {
	t.Parallel()

	s := &node.Struct{Name: "Container", Package: "example.com/x", TypeParams: params("K", "V")}

	t.Run("TypeArgs matches TypeParamNames", func(t *testing.T) {
		t.Parallel()
		if a, b := golang.TypeArgs(s), golang.TypeParamNames(s.TypeParams); a != b {
			t.Fatalf("TypeArgs = %q, TypeParamNames = %q", a, b)
		}
	})

	t.Run("TypeParams matches TypeParamDecls", func(t *testing.T) {
		t.Parallel()
		a, b := golang.TypeParams(s), golang.TypeParamDecls(s.TypeParams)
		if len(a) != len(b) || a[0].Name != b[0].Name {
			t.Fatalf("TypeParams = %+v, TypeParamDecls = %+v", a, b)
		}
	})

	t.Run("SelfType matches SelfRef", func(t *testing.T) {
		t.Parallel()
		a, _ := golang.SelfType(s).(*emit.ExternalRef)
		b, _ := golang.SelfRef(s.Package, s.Name, s.TypeParams).(*emit.ExternalRef)
		if a.Name != b.Name || len(a.TypeArgs) != len(b.TypeArgs) {
			t.Fatalf("SelfType = %#v, SelfRef = %#v", a, b)
		}
	})
}
