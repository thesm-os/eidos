// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
)

// TestEmbedBuilder_Accessors covers Pos / Docs / Directive on an
// embed and confirms Node returns the underlying pointer.
func TestEmbedBuilder_Accessors(t *testing.T) {
	t.Parallel()

	t.Run("Pos / Docs / Directive / Node thread through", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		d := fixtureDirective()
		pos := fixturePos()
		var node *emit.Embed
		c.Package("p", "p").
			Struct("S", func(sb *builder.StructBuilder) {
				sb.Embed(emit.Builtin("Reader"), func(b *builder.EmbedBuilder) {
					node = b.Node()
					b.Pos(pos).Docs("docs").Directive(d)
				})
			})
		assertCommon(t, node.SourcePos, node.DocLines, node.DirectiveList, pos, d)
		if node == nil {
			t.Fatalf("Node returned nil")
		}
	})
}
