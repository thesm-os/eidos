// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
)

// TestEnumBuilder_VariantsCarryOwner covers the enum-variant Owner
// back-pointer wiring.
func TestEnumBuilder_VariantsCarryOwner(t *testing.T) {
	t.Parallel()

	t.Run("variants carry enum Owner", func(t *testing.T) {
		t.Parallel()
		c := builder.For("repogen").WithTarget(defaultTarget)
		var e *emit.Enum
		c.Package("status", "example.com/status").
			Enum("State", emit.Builtin("int"), func(eb *builder.EnumBuilder) {
				e = eb.Node()
				eb.Variant("Active", nil, nil)
				eb.Variant("Inactive", nil, nil)
			})
		if len(e.Variants) != 2 {
			t.Fatalf("expected 2 variants; got %d", len(e.Variants))
		}
		for _, v := range e.Variants {
			if v.Owner != e {
				t.Fatalf("variant %q Owner not wired", v.Name)
			}
		}
	})
}

// TestEnumBuilder_Accessors covers the Pos / Docs / Directive /
// Target accessors on the enum builder and the nested EnumVariant
// builder accessors.
func TestEnumBuilder_Accessors(t *testing.T) {
	t.Parallel()

	t.Run("Pos / Docs / Directive / Target / Origin thread through; nested Variant accessors", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		other := otherTarget()
		d := fixtureDirective()
		pos := fixturePos()
		origin := fixtureOrigin()
		var n *emit.Enum
		c.Package("p", "p").
			Enum("E", emit.Builtin("int"), func(b *builder.EnumBuilder) {
				n = b.Node()
				b.Pos(pos).Docs("docs").Directive(d).Target(other).Origin(origin).
					Variant("V", nil, func(vb *builder.EnumVariantBuilder) {
						vb.Pos(pos).Docs("vd").Directive(d)
					})
			})
		assertCommon(t, n.SourcePos, n.DocLines, n.DirectiveList, pos, d)
		if n.Target != other {
			t.Fatalf("enum target override failed")
		}
		if n.Origin() != origin {
			t.Fatalf("Origin not threaded; got %v, want %v", n.Origin(), origin)
		}
		v := n.Variants[0]
		assertCommon(t, v.SourcePos, v.DocLines, v.DirectiveList, pos, d)
	})
}
