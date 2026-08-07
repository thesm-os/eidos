// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
)

// TestEnumVariantBuilder_Accessors covers Pos / Docs / Directive
// on an enum variant and the Node accessor.
func TestEnumVariantBuilder_Accessors(t *testing.T) {
	t.Parallel()

	t.Run("Pos / Docs / Directive / Node thread through", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		d := fixtureDirective()
		pos := fixturePos()
		var node *emit.EnumVariant
		c.Package("p", "p").
			Enum("E", emit.Builtin("int"), func(eb *builder.EnumBuilder) {
				eb.Variant("V", nil, func(b *builder.EnumVariantBuilder) {
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
