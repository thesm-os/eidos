// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
)

// TestParamBuilder_Accessors covers Pos / Docs / Directive on a
// parameter and the Variadic toggle. The Variadic-on-function path
// is exercised by [TestFunctionBuilder]; this stanza focuses on
// the per-builder accessor surface.
func TestParamBuilder_Accessors(t *testing.T) {
	t.Parallel()

	t.Run("Pos / Docs / Directive / Variadic thread through", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		d := fixtureDirective()
		pos := fixturePos()
		var node *emit.Param
		c.Package("p", "p").
			Function("F", func(fb *builder.FunctionBuilder) {
				fb.Param("p", emit.Builtin("int"), func(b *builder.ParamBuilder) {
					node = b.Node()
					b.Pos(pos).Docs("docs").Directive(d).Variadic()
				})
			})
		assertCommon(t, node.SourcePos, node.DocLines, node.DirectiveList, pos, d)
		if !node.Variadic {
			t.Fatalf("Variadic toggle did not stick")
		}
	})
}
