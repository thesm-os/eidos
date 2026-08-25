// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
)

func TestBuilder_Variable(t *testing.T) {
	t.Parallel()

	t.Run("creates a variable with the configured name and package", func(t *testing.T) {
		t.Parallel()
		b := gofixture.New().Package("users", "example.com/users").
			Variable("Default", nil)
		v := b.PackageNode().VariableByName("Default")
		if v == nil {
			t.Fatalf("Variable should be reachable by name")
		}
		requireQName(t, v.QName(), "example.com/users.Default")
	})

	t.Run("invokes the configuration callback exactly once", func(t *testing.T) {
		t.Parallel()
		var calls int
		gofixture.New().Variable("V", func(*gofixture.VariableBuilder) { calls++ })
		if calls != 1 {
			t.Fatalf("callback invocation count wrong: got %d, want 1", calls)
		}
	})
}

func TestVariableBuilder_Node(t *testing.T) {
	t.Parallel()

	t.Run("returns the variable backing the builder", func(t *testing.T) {
		t.Parallel()
		got := captureFirstVariable(t, func(*gofixture.VariableBuilder) {})
		if got == nil || got.Name != "V" {
			t.Fatalf("Node returned wrong variable: %+v", got)
		}
	})
}

func TestVariableBuilder_Pos(t *testing.T) {
	t.Parallel()

	t.Run("records the supplied position", func(t *testing.T) {
		t.Parallel()
		pos := position.At("v.go", 1, 1)
		got := captureFirstVariable(t, func(b *gofixture.VariableBuilder) { b.Pos(pos) })
		if !got.SourcePos.Equal(pos) {
			t.Fatalf("Pos not applied: %v", got.SourcePos)
		}
	})
}

func TestVariableBuilder_Docs(t *testing.T) {
	t.Parallel()

	t.Run("appends doc-comment lines in order", func(t *testing.T) {
		t.Parallel()
		got := captureFirstVariable(t, func(b *gofixture.VariableBuilder) {
			b.Docs("one").Docs("two")
		})
		if d := got.Docs(); len(d) != 2 || d[0] != "one" || d[1] != "two" {
			t.Fatalf("Docs order wrong: %+v", d)
		}
	})
}

func TestVariableBuilder_Directive(t *testing.T) {
	t.Parallel()

	t.Run("attaches the directive", func(t *testing.T) {
		t.Parallel()
		d := gofixture.Directive("expose")
		got := captureFirstVariable(t, func(b *gofixture.VariableBuilder) { b.Directive(d) })
		if !got.HasDirective("expose") {
			t.Fatalf("HasDirective should return true for expose")
		}
	})
}

func TestVariableBuilder_Type(t *testing.T) {
	t.Parallel()

	t.Run("records the declared type", func(t *testing.T) {
		t.Parallel()
		typ := gofixture.Named("string")
		got := captureFirstVariable(t, func(b *gofixture.VariableBuilder) { b.Type(typ) })
		if !got.HasDeclaredType() || got.Type != typ {
			t.Fatalf("Type not applied: %+v", got.Type)
		}
	})
}

func TestVariableBuilder_InitExpr(t *testing.T) {
	t.Parallel()

	t.Run("records the verbatim initialiser", func(t *testing.T) {
		t.Parallel()
		got := captureFirstVariable(t, func(b *gofixture.VariableBuilder) {
			b.InitExpr(`"hello"`)
		})
		if !got.HasInitExpr() || got.InitExpr != `"hello"` {
			t.Fatalf("InitExpr not applied: %q", got.InitExpr)
		}
	})
}
