// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
)

func TestBuilder_Alias(t *testing.T) {
	t.Parallel()

	t.Run("creates an alias with the configured name and package", func(t *testing.T) {
		t.Parallel()
		b := gofixture.New().Package("users", "example.com/users").
			Alias("UserID", nil)
		a := b.PackageNode().AliasByName("UserID")
		if a == nil {
			t.Fatalf("Alias should be reachable by name")
		}
		requireQName(t, a.QName(), "example.com/users.UserID")
	})

	t.Run("invokes the configuration callback exactly once", func(t *testing.T) {
		t.Parallel()
		var calls int
		gofixture.New().Alias("A", func(*gofixture.AliasBuilder) { calls++ })
		if calls != 1 {
			t.Fatalf("callback invocation count wrong: got %d, want 1", calls)
		}
	})
}

func TestAliasBuilder_Node(t *testing.T) {
	t.Parallel()

	t.Run("returns the alias backing the builder", func(t *testing.T) {
		t.Parallel()
		got := captureFirstAlias(t, func(*gofixture.AliasBuilder) {})
		if got == nil || got.Name != "A" {
			t.Fatalf("Node returned wrong alias: %+v", got)
		}
	})
}

func TestAliasBuilder_Pos(t *testing.T) {
	t.Parallel()

	t.Run("records the supplied position", func(t *testing.T) {
		t.Parallel()
		pos := position.At("a.go", 1, 1)
		got := captureFirstAlias(t, func(b *gofixture.AliasBuilder) { b.Pos(pos) })
		if !got.SourcePos.Equal(pos) {
			t.Fatalf("Pos not applied: %v", got.SourcePos)
		}
	})
}

func TestAliasBuilder_Docs(t *testing.T) {
	t.Parallel()

	t.Run("appends doc-comment lines in order", func(t *testing.T) {
		t.Parallel()
		got := captureFirstAlias(t, func(b *gofixture.AliasBuilder) {
			b.Docs("one").Docs("two")
		})
		if d := got.Docs(); len(d) != 2 || d[0] != "one" || d[1] != "two" {
			t.Fatalf("Docs order wrong: %+v", d)
		}
	})
}

func TestAliasBuilder_Directive(t *testing.T) {
	t.Parallel()

	t.Run("attaches the directive", func(t *testing.T) {
		t.Parallel()
		d := gofixture.Directive("expose")
		got := captureFirstAlias(t, func(b *gofixture.AliasBuilder) { b.Directive(d) })
		if !got.HasDirective("expose") {
			t.Fatalf("HasDirective should return true for expose")
		}
	})
}

func TestAliasBuilder_Target(t *testing.T) {
	t.Parallel()

	t.Run("records the alias target type", func(t *testing.T) {
		t.Parallel()
		typ := gofixture.Named("string")
		got := captureFirstAlias(t, func(b *gofixture.AliasBuilder) { b.Target(typ) })
		if got.Target != typ {
			t.Fatalf("Target not applied: %+v", got.Target)
		}
	})
}

func TestAliasBuilder_True(t *testing.T) {
	t.Parallel()

	t.Run("default is the new-named-type form", func(t *testing.T) {
		t.Parallel()
		got := captureFirstAlias(t, func(*gofixture.AliasBuilder) {})
		if got.IsAlias {
			t.Fatalf("default alias should be a new-named-type form, not a true alias")
		}
	})

	t.Run("True marks the declaration as a true alias", func(t *testing.T) {
		t.Parallel()
		got := captureFirstAlias(t, func(b *gofixture.AliasBuilder) { b.True() })
		if !got.IsAlias {
			t.Fatalf("True() should mark the alias as a true alias")
		}
	})
}

func TestAliasBuilder_TypeParam(t *testing.T) {
	t.Parallel()

	t.Run("declares a generic type parameter with owner wired", func(t *testing.T) {
		t.Parallel()
		got := captureFirstAlias(t, func(b *gofixture.AliasBuilder) {
			b.TypeParam("T", gofixture.Constraint(gofixture.Named("comparable")))
		})
		if !got.IsGeneric() {
			t.Fatalf("alias should be generic")
		}
		tp := got.TypeParams[0]
		if tp.Name != "T" || tp.Owner != got {
			t.Fatalf("TypeParam wiring wrong: %+v", tp)
		}
		if !tp.Constraint.IsComparable() {
			t.Fatalf("constraint should reflect comparable bound")
		}
	})
}
