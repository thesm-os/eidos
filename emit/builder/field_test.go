// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/node"
)

// TestFieldBuilder_Accessors covers Pos / Docs / Directive plus
// the field-specific Tag and LineComment setters.
func TestFieldBuilder_Accessors(t *testing.T) {
	t.Parallel()

	t.Run("Pos / Docs / Directive / Tag / LineComment thread through", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		d := fixtureDirective()
		pos := fixturePos()
		var node *emit.Field
		c.Package("p", "p").
			Struct("S", func(sb *builder.StructBuilder) {
				sb.Field("F", emit.Builtin("int"), func(b *builder.FieldBuilder) {
					node = b.Node()
					b.Pos(pos).Docs("docs").Directive(d).Tag(`json:"f"`).LineComment("hi")
				})
			})
		assertCommon(t, node.SourcePos, node.DocLines, node.DirectiveList, pos, d)
		if node.Tag != `json:"f"` {
			t.Fatalf("Tag override failed; got %q", node.Tag)
		}
		if node.LineComment != "hi" {
			t.Fatalf("LineComment override failed; got %q", node.LineComment)
		}
	})
}

// TestFieldBuilder_Origin pins the source back-pointer a field
// carries. Downstream consumers — the backend's render-site
// lookups and the `explain` command — reach the source-side meta
// bag through it, so a field built without one is invisible to
// both.
func TestFieldBuilder_Origin(t *testing.T) {
	t.Parallel()

	t.Run("records the source node on the built field", func(t *testing.T) {
		t.Parallel()
		src := &node.Field{Name: "ID"}
		var f *emit.Field
		builder.For("mg").Package("x", "example.com/x").
			Struct("S", func(sb *builder.StructBuilder) {
				sb.Field("ID", emit.Builtin("string"), func(fb *builder.FieldBuilder) {
					fb.Origin(src)
					f = fb.Node()
				})
			})
		if f.Origin() != src {
			t.Fatalf("Origin = %v, want %v", f.Origin(), src)
		}
	})

	t.Run("returns the builder so the chain continues", func(t *testing.T) {
		t.Parallel()
		var same bool
		builder.For("mg").Package("x", "example.com/x").
			Struct("S", func(sb *builder.StructBuilder) {
				sb.Field("ID", emit.Builtin("string"), func(fb *builder.FieldBuilder) {
					same = fb.Origin(&node.Field{Name: "ID"}) == fb
				})
			})
		if !same {
			t.Fatalf("Origin must return the same FieldBuilder for chaining")
		}
	})
}
