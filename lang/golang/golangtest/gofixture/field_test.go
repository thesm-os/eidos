// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
)

func TestFieldBuilder_Node(t *testing.T) {
	t.Parallel()

	t.Run("returns the field backing the builder", func(t *testing.T) {
		t.Parallel()
		got := captureFirstField(t, func(*gofixture.FieldBuilder) {})
		if got == nil || got.Name != "F" {
			t.Fatalf("Node returned wrong field: %+v", got)
		}
	})
}

func TestFieldBuilder_Pos(t *testing.T) {
	t.Parallel()

	t.Run("records the supplied position", func(t *testing.T) {
		t.Parallel()
		pos := position.At("s.go", 5, 2)
		got := captureFirstField(t, func(b *gofixture.FieldBuilder) { b.Pos(pos) })
		if !got.SourcePos.Equal(pos) {
			t.Fatalf("Pos not applied: %v", got.SourcePos)
		}
	})
}

func TestFieldBuilder_Docs(t *testing.T) {
	t.Parallel()

	t.Run("appends doc-comment lines in order", func(t *testing.T) {
		t.Parallel()
		got := captureFirstField(t, func(b *gofixture.FieldBuilder) {
			b.Docs("one").Docs("two")
		})
		if d := got.Docs(); len(d) != 2 || d[0] != "one" || d[1] != "two" {
			t.Fatalf("Docs order wrong: %+v", d)
		}
	})
}

func TestFieldBuilder_Tag(t *testing.T) {
	t.Parallel()

	t.Run("records the struct-tag string verbatim", func(t *testing.T) {
		t.Parallel()
		tag := "`json:\"id\"`"
		got := captureFirstField(t, func(b *gofixture.FieldBuilder) { b.Tag(tag) })
		if got.Tag != tag {
			t.Fatalf("Tag not applied: got %q, want %q", got.Tag, tag)
		}
	})
}

func TestFieldBuilder_Directive(t *testing.T) {
	t.Parallel()

	t.Run("attaches the directive to the field", func(t *testing.T) {
		t.Parallel()
		d := gofixture.Directive("validate")
		got := captureFirstField(t, func(b *gofixture.FieldBuilder) { b.Directive(d) })
		if !got.HasDirective("validate") {
			t.Fatalf("HasDirective should return true for validate")
		}
	})
}
