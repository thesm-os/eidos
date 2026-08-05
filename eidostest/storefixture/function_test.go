// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package storefixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/storefixture"
)

func TestBuilder_Function(t *testing.T) {
	t.Parallel()

	t.Run("creates a function with the configured name and package", func(t *testing.T) {
		t.Parallel()
		b := storefixture.New().Package("users", "example.com/users").
			Function("Open", nil)
		f := b.PackageNode().FunctionByName("Open")
		if f == nil {
			t.Fatalf("Function should be reachable by name")
		}
		requireQName(t, f.QName(), "example.com/users.Open")
	})

	t.Run("invokes the configuration callback exactly once", func(t *testing.T) {
		t.Parallel()
		var calls int
		storefixture.New().Function("F", func(*storefixture.FunctionBuilder) { calls++ })
		if calls != 1 {
			t.Fatalf("callback invocation count wrong: got %d, want 1", calls)
		}
	})
}

func TestFunctionBuilder_Node(t *testing.T) {
	t.Parallel()

	t.Run("returns the function backing the builder", func(t *testing.T) {
		t.Parallel()
		got := captureFirstFunction(t, func(*storefixture.FunctionBuilder) {})
		if got == nil || got.Name != "F" {
			t.Fatalf("Node returned wrong function: %+v", got)
		}
	})
}

func TestFunctionBuilder_Pos(t *testing.T) {
	t.Parallel()

	t.Run("records the supplied position", func(t *testing.T) {
		t.Parallel()
		pos := position.At("f.go", 11, 1)
		got := captureFirstFunction(t, func(b *storefixture.FunctionBuilder) { b.Pos(pos) })
		if !got.SourcePos.Equal(pos) {
			t.Fatalf("Pos not applied: %v", got.SourcePos)
		}
	})
}

func TestFunctionBuilder_Docs(t *testing.T) {
	t.Parallel()

	t.Run("appends doc-comment lines in order", func(t *testing.T) {
		t.Parallel()
		got := captureFirstFunction(t, func(b *storefixture.FunctionBuilder) {
			b.Docs("alpha").Docs("beta")
		})
		if d := got.Docs(); len(d) != 2 || d[0] != "alpha" || d[1] != "beta" {
			t.Fatalf("Docs order wrong: %+v", d)
		}
	})
}

func TestFunctionBuilder_Directive(t *testing.T) {
	t.Parallel()

	t.Run("attaches the directive", func(t *testing.T) {
		t.Parallel()
		d := storefixture.Directive("expose")
		got := captureFirstFunction(t, func(b *storefixture.FunctionBuilder) { b.Directive(d) })
		if !got.HasDirective("expose") {
			t.Fatalf("HasDirective should return true for expose")
		}
	})
}

func TestFunctionBuilder_Param(t *testing.T) {
	t.Parallel()

	t.Run("appends a non-variadic parameter with owner wired", func(t *testing.T) {
		t.Parallel()
		got := captureFirstFunction(t, func(b *storefixture.FunctionBuilder) {
			b.Param("ctx", storefixture.PkgNamed("context", "Context"))
		})
		if len(got.Params) != 1 {
			t.Fatalf("expected 1 param; got %d", len(got.Params))
		}
		p := got.Params[0]
		if p.Name != "ctx" || p.Variadic || p.Owner != got {
			t.Fatalf("Param wiring wrong: %+v", p)
		}
	})
}

func TestFunctionBuilder_Variadic(t *testing.T) {
	t.Parallel()

	t.Run("appends a variadic parameter using the element type", func(t *testing.T) {
		t.Parallel()
		got := captureFirstFunction(t, func(b *storefixture.FunctionBuilder) {
			b.Variadic("args", storefixture.Named("string"))
		})
		p := got.Params[0]
		if !p.Variadic || p.Type.Name != "string" {
			t.Fatalf("Variadic wiring wrong: %+v", p)
		}
	})
}

func TestFunctionBuilder_Return(t *testing.T) {
	t.Parallel()

	t.Run("appends a return type", func(t *testing.T) {
		t.Parallel()
		got := captureFirstFunction(t, func(b *storefixture.FunctionBuilder) {
			b.Return(storefixture.Named("error"))
		})
		if len(got.Returns) != 1 || got.Returns[0].Type.Name != "error" {
			t.Fatalf("Return wiring wrong: %+v", got.Returns)
		}
	})
}

func TestFunctionBuilder_TypeParam(t *testing.T) {
	t.Parallel()

	t.Run("declares a generic type parameter with owner wired", func(t *testing.T) {
		t.Parallel()
		got := captureFirstFunction(t, func(b *storefixture.FunctionBuilder) {
			b.TypeParam("T", storefixture.Constraint(storefixture.Named("comparable")))
		})
		tp := got.TypeParams[0]
		if tp.Name != "T" || tp.Owner != got {
			t.Fatalf("TypeParam wiring wrong: %+v", tp)
		}
		if !tp.Constraint.IsComparable() {
			t.Fatalf("constraint should reflect comparable bound")
		}
	})
}
