// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
)

func TestBuilder_Function(t *testing.T) {
	t.Parallel()

	t.Run("creates a function with the configured name and package", func(t *testing.T) {
		t.Parallel()
		b := gofixture.New().Package("users", "example.com/users").
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
		gofixture.New().Function("F", func(*gofixture.FunctionBuilder) { calls++ })
		if calls != 1 {
			t.Fatalf("callback invocation count wrong: got %d, want 1", calls)
		}
	})
}

func TestFunctionBuilder_Node(t *testing.T) {
	t.Parallel()

	t.Run("returns the function backing the builder", func(t *testing.T) {
		t.Parallel()
		got := captureFirstFunction(t, func(*gofixture.FunctionBuilder) {})
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
		got := captureFirstFunction(t, func(b *gofixture.FunctionBuilder) { b.Pos(pos) })
		if !got.SourcePos.Equal(pos) {
			t.Fatalf("Pos not applied: %v", got.SourcePos)
		}
	})
}

func TestFunctionBuilder_Docs(t *testing.T) {
	t.Parallel()

	t.Run("appends doc-comment lines in order", func(t *testing.T) {
		t.Parallel()
		got := captureFirstFunction(t, func(b *gofixture.FunctionBuilder) {
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
		d := gofixture.Directive("expose")
		got := captureFirstFunction(t, func(b *gofixture.FunctionBuilder) { b.Directive(d) })
		if !got.HasDirective("expose") {
			t.Fatalf("HasDirective should return true for expose")
		}
	})
}

func TestFunctionBuilder_Param(t *testing.T) {
	t.Parallel()

	t.Run("appends a non-variadic parameter with owner wired", func(t *testing.T) {
		t.Parallel()
		got := captureFirstFunction(t, func(b *gofixture.FunctionBuilder) {
			b.Param("ctx", gofixture.PkgNamed("context", "Context"))
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
		got := captureFirstFunction(t, func(b *gofixture.FunctionBuilder) {
			b.Variadic("args", gofixture.Named("string"))
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
		got := captureFirstFunction(t, func(b *gofixture.FunctionBuilder) {
			b.Return(gofixture.Named("error"))
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
		got := captureFirstFunction(t, func(b *gofixture.FunctionBuilder) {
			b.TypeParam("T", gofixture.Constraint(gofixture.Named("comparable")))
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

func TestFunctionBuilder_NamedReturn(t *testing.T) {
	t.Parallel()

	t.Run("records the return's name alongside its type", func(t *testing.T) {
		t.Parallel()
		got := captureFirstFunction(t, func(b *gofixture.FunctionBuilder) {
			b.NamedReturn("n", gofixture.Named("int"))
		})
		if len(got.Returns) != 1 {
			t.Fatalf("Returns = %+v, want one", got.Returns)
		}
		if got.Returns[0].Name != "n" {
			t.Errorf("Returns[0].Name = %q, want n", got.Returns[0].Name)
		}
		if ref := asNamedRef(t, got.Returns[0].Type); ref.Name != "int" {
			t.Errorf("Returns[0].Type = %q, want int", ref.Name)
		}
	})

	t.Run("composes with an unnamed Return in declaration order", func(t *testing.T) {
		t.Parallel()
		// Go allows only all-named or all-unnamed results, but the
		// fixture builds what a test asks for: a generator reading a
		// half-named signature is exactly the input worth reproducing.
		got := captureFirstFunction(t, func(b *gofixture.FunctionBuilder) {
			b.NamedReturn("n", gofixture.Named("int"))
			b.Return(gofixture.Named("error"))
		})
		if len(got.Returns) != 2 {
			t.Fatalf("Returns = %+v, want two", got.Returns)
		}
		if got.Returns[1].Name != "" {
			t.Errorf("Returns[1].Name = %q, want the unnamed return to stay unnamed",
				got.Returns[1].Name)
		}
	})
}
