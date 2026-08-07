// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
)

// TestVariableBuilder_Accessors covers the Pos / Docs / Directive
// / Target accessors on the variable builder. The Variable
// constructor itself is exercised via the [TestPackageBuilder]
// happy-path test; this stanza focuses on the accessor surface.
func TestVariableBuilder_Accessors(t *testing.T) {
	t.Parallel()

	t.Run("Pos / Docs / Directive / Target / Origin thread through", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		other := otherTarget()
		d := fixtureDirective()
		pos := fixturePos()
		origin := fixtureOrigin()
		var n *emit.Variable
		c.Package("p", "p").
			Variable("V", emit.Builtin("int"), nil, func(b *builder.VariableBuilder) {
				n = b.Node()
				b.Pos(pos).Docs("docs").Directive(d).Target(other).Origin(origin)
			})
		assertCommon(t, n.SourcePos, n.DocLines, n.DirectiveList, pos, d)
		if n.Target != other {
			t.Fatalf("variable target override failed; got %v", n.Target)
		}
		if n.Origin() != origin {
			t.Fatalf("Origin not threaded; got %v, want %v", n.Origin(), origin)
		}
	})
}
