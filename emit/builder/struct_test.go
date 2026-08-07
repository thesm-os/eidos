// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
)

// TestStructBuilder_NestedShape covers the nested decl-construction
// surface: fields, methods, embeds, and type parameters wire Owner
// back-pointers automatically as nested callbacks return.
func TestStructBuilder_NestedShape(t *testing.T) {
	t.Parallel()

	t.Run("Field / Method / Embed / TypeParam wire Owner back-pointers", func(t *testing.T) {
		t.Parallel()
		c := builder.For("repogen").WithTarget(defaultTarget)
		var s *emit.Struct
		c.Package("users", "example.com/users").
			Struct("User", func(sb *builder.StructBuilder) {
				s = sb.Node()
				sb.Field("ID", emit.Builtin("string"), nil)
				sb.Field("Email", emit.Builtin("string"), func(f *builder.FieldBuilder) {
					f.Tag(`json:"email"`).LineComment("user email")
				})
				sb.Embed(emit.Internal(s), nil)
				sb.Method("Validate", func(m *builder.MethodBuilder) {
					m.Receiver("u", emit.Ptr(emit.Internal(s)))
					m.Return(emit.Builtin("error"))
				})
				sb.TypeParam("T", nil)
			})

		if len(s.Fields) != 2 {
			t.Fatalf("expected 2 fields, got %d", len(s.Fields))
		}
		for _, f := range s.Fields {
			if f.Owner != s {
				t.Fatalf("field %q Owner not wired", f.Name)
			}
		}
		if s.Fields[1].Tag != `json:"email"` {
			t.Fatalf("field tag not threaded; got %q", s.Fields[1].Tag)
		}
		if s.Fields[1].LineComment != "user email" {
			t.Fatalf("line comment not threaded; got %q", s.Fields[1].LineComment)
		}
		if len(s.Methods) != 1 || s.Methods[0].Owner != s {
			t.Fatalf("method Owner not wired")
		}
		if len(s.Embeds) != 1 || s.Embeds[0].Owner != s {
			t.Fatalf("embed Owner not wired")
		}
		if len(s.TypeParams) != 1 || s.TypeParams[0].Owner != s {
			t.Fatalf("type-param Owner not wired")
		}
	})
}

// TestStructBuilder_Accessors covers the Pos / Docs / Directive /
// Target / Node accessors and confirms TypeParam appends a typed
// entry to the struct.
func TestStructBuilder_Accessors(t *testing.T) {
	t.Parallel()

	t.Run("Pos / Docs / Directive / Target / TypeParam / Origin thread through", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		other := otherTarget()
		d := fixtureDirective()
		pos := fixturePos()
		origin := fixtureOrigin()
		var n *emit.Struct
		c.Package("p", "p").
			Struct("S", func(b *builder.StructBuilder) {
				n = b.Node()
				b.Pos(pos).Docs("docs").Directive(d).Target(other).TypeParam("T", nil).Origin(origin)
			})
		assertCommon(t, n.SourcePos, n.DocLines, n.DirectiveList, pos, d)
		if n.Target != other {
			t.Fatalf("target override failed; got %v", n.Target)
		}
		if len(n.TypeParams) != 1 || n.TypeParams[0].Name != "T" {
			t.Fatalf("type param not appended; got %+v", n.TypeParams)
		}
		if n.Origin() != origin {
			t.Fatalf("Origin not threaded; got %v, want %v", n.Origin(), origin)
		}
	})
}
