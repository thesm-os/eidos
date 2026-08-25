// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
)

func TestBuilder_Constant(t *testing.T) {
	t.Parallel()

	t.Run("creates a constant with the configured name and package", func(t *testing.T) {
		t.Parallel()
		b := gofixture.New().Package("users", "example.com/users").
			Constant("Pi", nil)
		c := b.PackageNode().ConstantByName("Pi")
		if c == nil {
			t.Fatalf("Constant should be reachable by name")
		}
		requireQName(t, c.QName(), "example.com/users.Pi")
	})

	t.Run("invokes the configuration callback exactly once", func(t *testing.T) {
		t.Parallel()
		var calls int
		gofixture.New().Constant("C", func(*gofixture.ConstantBuilder) { calls++ })
		if calls != 1 {
			t.Fatalf("callback invocation count wrong: got %d, want 1", calls)
		}
	})
}

func TestConstantBuilder_Node(t *testing.T) {
	t.Parallel()

	t.Run("returns the constant backing the builder", func(t *testing.T) {
		t.Parallel()
		got := captureFirstConstant(t, func(*gofixture.ConstantBuilder) {})
		if got == nil || got.Name != "C" {
			t.Fatalf("Node returned wrong constant: %+v", got)
		}
	})
}

func TestConstantBuilder_Pos(t *testing.T) {
	t.Parallel()

	t.Run("records the supplied position", func(t *testing.T) {
		t.Parallel()
		pos := position.At("c.go", 1, 1)
		got := captureFirstConstant(t, func(b *gofixture.ConstantBuilder) { b.Pos(pos) })
		if !got.SourcePos.Equal(pos) {
			t.Fatalf("Pos not applied: %v", got.SourcePos)
		}
	})
}

func TestConstantBuilder_Docs(t *testing.T) {
	t.Parallel()

	t.Run("appends doc-comment lines in order", func(t *testing.T) {
		t.Parallel()
		got := captureFirstConstant(t, func(b *gofixture.ConstantBuilder) {
			b.Docs("one").Docs("two")
		})
		if d := got.Docs(); len(d) != 2 || d[0] != "one" || d[1] != "two" {
			t.Fatalf("Docs order wrong: %+v", d)
		}
	})
}

func TestConstantBuilder_Directive(t *testing.T) {
	t.Parallel()

	t.Run("attaches the directive", func(t *testing.T) {
		t.Parallel()
		d := gofixture.Directive("expose")
		got := captureFirstConstant(t, func(b *gofixture.ConstantBuilder) { b.Directive(d) })
		if !got.HasDirective("expose") {
			t.Fatalf("HasDirective should return true for expose")
		}
	})
}

func TestConstantBuilder_Type(t *testing.T) {
	t.Parallel()

	t.Run("records the declared type", func(t *testing.T) {
		t.Parallel()
		typ := gofixture.Named("float64")
		got := captureFirstConstant(t, func(b *gofixture.ConstantBuilder) { b.Type(typ) })
		if !got.HasDeclaredType() || got.Type != typ {
			t.Fatalf("Type not applied: %+v", got.Type)
		}
	})
}

func TestConstantBuilder_Value(t *testing.T) {
	t.Parallel()

	t.Run("records the verbatim value", func(t *testing.T) {
		t.Parallel()
		got := captureFirstConstant(t, func(b *gofixture.ConstantBuilder) {
			b.Value("3.14159")
		})
		if got.Value != "3.14159" {
			t.Fatalf("Value not applied: %q", got.Value)
		}
	})
}
