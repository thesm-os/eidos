// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
)

// TestImportBuilder_Accessors covers Pos / Docs / Directive on an
// import plus the Alias setter; the Node accessor returns the
// underlying [*emit.Import].
func TestImportBuilder_Accessors(t *testing.T) {
	t.Parallel()

	t.Run("Pos / Docs / Directive / Alias / Node thread through", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		d := fixtureDirective()
		pos := fixturePos()
		var node *emit.Import
		c.Package("p", "p").
			AddFile(emit.Target{}, func(fb *builder.FileBuilder) {
				fb.Import("fmt", func(b *builder.ImportBuilder) {
					node = b.Node()
					b.Pos(pos).Docs("docs").Directive(d).Alias("fmtalias")
				})
			})
		assertCommon(t, node.SourcePos, node.DocLines, node.DirectiveList, pos, d)
		if node.Alias != "fmtalias" {
			t.Fatalf("Alias not threaded; got %q", node.Alias)
		}
	})
}
