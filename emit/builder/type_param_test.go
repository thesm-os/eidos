// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
)

// TestTypeParamBuilder_Accessors covers Pos / Docs / Directive /
// Constraint / Node on the TypeParam sub-builder. Owner wiring is
// covered by the spawning host's nested-shape test
// ([TestStructBuilder_NestedShape], etc.); this stanza focuses on
// the per-builder accessor surface.
func TestTypeParamBuilder_Accessors(t *testing.T) {
	t.Parallel()

	t.Run("Pos / Docs / Directive / Constraint / Node thread through", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		d := fixtureDirective()
		pos := fixturePos()
		var node *emit.TypeParam
		bound := &emit.Constraint{Embedded: []emit.Ref{emit.Builtin("comparable")}}
		c.Package("p", "p").
			Struct("S", func(sb *builder.StructBuilder) {
				sb.TypeParam("T", nil, func(b *builder.TypeParamBuilder) {
					node = b.Node()
					b.Pos(pos).Docs("docs").Directive(d).Constraint(bound)
				})
			})
		assertCommon(t, node.SourcePos, node.DocLines, node.DirectiveList, pos, d)
		if node.Constraint != bound {
			t.Fatalf("Constraint override failed; got %+v want %+v", node.Constraint, bound)
		}
	})

	t.Run("Variadic-nil fn callback is a no-op", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		var s *emit.Struct
		c.Package("p", "p").
			Struct("S", func(sb *builder.StructBuilder) {
				s = sb.Node()
				sb.TypeParam("T", nil)
			})
		if len(s.TypeParams) != 1 || s.TypeParams[0].Name != "T" {
			t.Fatalf("nil fn should still append the type param; got %+v", s.TypeParams)
		}
	})

	t.Run("TypeParamBuilder wires up on Function / Method / Alias / Interface hosts too", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		var (
			fnTP     *emit.TypeParam
			methodTP *emit.TypeParam
			ifaceTP  *emit.TypeParam
			aliasTP  *emit.TypeParam
		)
		c.Package("p", "p").
			Function("F", func(fb *builder.FunctionBuilder) {
				fb.TypeParam("T", nil, func(b *builder.TypeParamBuilder) { fnTP = b.Node() })
			}).
			Interface("I", func(ib *builder.InterfaceBuilder) {
				ib.TypeParam("T", nil, func(b *builder.TypeParamBuilder) { ifaceTP = b.Node() })
			}).
			NamedType("N", emit.Builtin("int"), func(ab *builder.AliasBuilder) {
				ab.TypeParam("T", nil, func(b *builder.TypeParamBuilder) { aliasTP = b.Node() })
			}).
			Struct("S", func(sb *builder.StructBuilder) {
				sb.Method("M", func(mb *builder.MethodBuilder) {
					mb.TypeParam("T", nil, func(b *builder.TypeParamBuilder) { methodTP = b.Node() })
				})
			})
		for _, tp := range []*emit.TypeParam{fnTP, methodTP, ifaceTP, aliasTP} {
			if tp == nil {
				t.Fatalf("TypeParam callback did not fire on one of the host kinds")
			}
		}
	})
}
