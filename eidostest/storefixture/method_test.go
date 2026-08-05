// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package storefixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/storefixture"
)

func TestMethodBuilder_Node(t *testing.T) {
	t.Parallel()

	t.Run("returns the method backing the builder", func(t *testing.T) {
		t.Parallel()
		got := captureFirstMethod(t, func(*storefixture.MethodBuilder) {})
		if got == nil || got.Name != "M" {
			t.Fatalf("Node returned wrong method: %+v", got)
		}
	})
}

func TestMethodBuilder_Pos(t *testing.T) {
	t.Parallel()

	t.Run("records the supplied position", func(t *testing.T) {
		t.Parallel()
		pos := position.At("repo.go", 12, 4)
		got := captureFirstMethod(t, func(b *storefixture.MethodBuilder) { b.Pos(pos) })
		if !got.SourcePos.Equal(pos) {
			t.Fatalf("Pos not applied: got %v, want %v", got.SourcePos, pos)
		}
	})
}

func TestMethodBuilder_Docs(t *testing.T) {
	t.Parallel()

	t.Run("appends doc-comment lines in order", func(t *testing.T) {
		t.Parallel()
		got := captureFirstMethod(t, func(b *storefixture.MethodBuilder) {
			b.Docs("one").Docs("two", "three")
		})
		if d := got.Docs(); len(d) != 3 || d[0] != "one" || d[1] != "two" || d[2] != "three" {
			t.Fatalf("Docs order wrong: %+v", d)
		}
	})
}

func TestMethodBuilder_Directive(t *testing.T) {
	t.Parallel()

	t.Run("attaches the directive", func(t *testing.T) {
		t.Parallel()
		d := storefixture.Directive("audit")
		got := captureFirstMethod(t, func(b *storefixture.MethodBuilder) { b.Directive(d) })
		if !got.HasDirective("audit") {
			t.Fatalf("HasDirective should return true for audit")
		}
	})
}

func TestMethodBuilder_Receiver(t *testing.T) {
	t.Parallel()

	t.Run("overrides the default pointer receiver", func(t *testing.T) {
		t.Parallel()
		value := storefixture.PkgNamed("example.com/test", "S")
		got := captureFirstMethod(t, func(b *storefixture.MethodBuilder) { b.Receiver(value) })
		if got.Receiver != value {
			t.Fatalf("Receiver override not applied: %+v", got.Receiver)
		}
	})
}

func TestMethodBuilder_ReceiverName(t *testing.T) {
	t.Parallel()

	t.Run("records the supplied receiver variable name", func(t *testing.T) {
		t.Parallel()
		got := captureFirstMethod(t, func(b *storefixture.MethodBuilder) { b.ReceiverName("r") })
		if got.ReceiverName != "r" {
			t.Fatalf("ReceiverName wrong: %q", got.ReceiverName)
		}
	})
}

func TestMethodBuilder_Param(t *testing.T) {
	t.Parallel()

	t.Run("appends a non-variadic parameter with owner wired", func(t *testing.T) {
		t.Parallel()
		got := captureFirstMethod(t, func(b *storefixture.MethodBuilder) {
			b.Param("ctx", storefixture.PkgNamed("context", "Context"))
		})
		if got.ParamCount() != 1 {
			t.Fatalf("expected 1 param; got %d", got.ParamCount())
		}
		p := got.Params[0]
		if p.Name != "ctx" || p.Variadic || p.Owner != got {
			t.Fatalf("Param wiring wrong: %+v", p)
		}
	})
}

func TestMethodBuilder_Variadic(t *testing.T) {
	t.Parallel()

	t.Run("appends a variadic parameter using the element type", func(t *testing.T) {
		t.Parallel()
		got := captureFirstMethod(t, func(b *storefixture.MethodBuilder) {
			b.Variadic("args", storefixture.Named("string"))
		})
		if !got.IsVariadic() {
			t.Fatalf("method should report variadic")
		}
		p := got.Params[0]
		if p.Name != "args" || !p.Variadic || p.Type.Name != "string" {
			t.Fatalf("Variadic wiring wrong: %+v", p)
		}
	})
}

func TestMethodBuilder_Return(t *testing.T) {
	t.Parallel()

	t.Run("appends a return type", func(t *testing.T) {
		t.Parallel()
		got := captureFirstMethod(t, func(b *storefixture.MethodBuilder) {
			b.Return(storefixture.Named("error"))
		})
		if got.ReturnCount() != 1 || got.Returns[0].Type.Name != "error" {
			t.Fatalf("Return wiring wrong: %+v", got.Returns)
		}
		if got.Returns[0].Name != "" {
			t.Fatalf("Return should produce an unnamed slot; got name %q", got.Returns[0].Name)
		}
	})

	t.Run("appends a named return", func(t *testing.T) {
		t.Parallel()
		got := captureFirstMethod(t, func(b *storefixture.MethodBuilder) {
			b.NamedReturn("item", storefixture.Named("string"))
		})
		if got.ReturnCount() != 1 {
			t.Fatalf("NamedReturn wiring wrong: %+v", got.Returns)
		}
		if got.Returns[0].Name != "item" || got.Returns[0].Type.Name != "string" {
			t.Fatalf("NamedReturn = %q %q, want item string",
				got.Returns[0].Name, got.Returns[0].Type.Name)
		}
	})
}

func TestMethodBuilder_TypeParam(t *testing.T) {
	t.Parallel()

	t.Run("declares a generic type parameter with owner wired", func(t *testing.T) {
		t.Parallel()
		got := captureFirstMethod(t, func(b *storefixture.MethodBuilder) {
			b.TypeParam("T", storefixture.Constraint(storefixture.PkgNamed("fmt", "Stringer")))
		})
		if !got.IsGeneric() {
			t.Fatalf("method should be generic")
		}
		tp := got.TypeParams[0]
		if tp.Name != "T" || tp.Owner != got {
			t.Fatalf("TypeParam wiring wrong: %+v", tp)
		}
		if tp.Constraint == nil || len(tp.Constraint.Embedded) != 1 {
			t.Fatalf("constraint should carry one embedded ref; got %+v", tp.Constraint)
		}
	})
}
