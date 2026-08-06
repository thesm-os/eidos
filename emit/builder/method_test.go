// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/node"
)

// TestMethodBuilder_ParamCallbackAndNamedReturn covers the
// optional Param-callback and the named-Return forms on Method —
// these are method-specific because struct/interface/alias methods
// all flow through the same MethodBuilder.
func TestMethodBuilder_ParamCallbackAndNamedReturn(t *testing.T) {
	t.Parallel()

	t.Run("Param with a callback runs it; named Return threads the name", func(t *testing.T) {
		t.Parallel()
		c := builder.For("repogen", defaultTarget)
		var m *emit.Method
		c.Package("p", "p").
			Struct("S", func(sb *builder.StructBuilder) {
				sb.Method("M", func(mb *builder.MethodBuilder) {
					m = mb.Node()
					mb.Param("opts", emit.Builtin("Options"), func(pb *builder.ParamBuilder) {
						pb.Variadic()
					})
					mb.Return(emit.Builtin("int"), "n")
				})
			})
		if !m.Params[0].Variadic {
			t.Fatalf("variadic flag not threaded through method.Param callback")
		}
		if m.Returns[0].Name != "n" {
			t.Fatalf("named return not threaded; got %+v", m.Returns)
		}
	})
}

// TestMethodBuilder_Accessors covers the Pos / Docs / Directive /
// TypeParam / Body / Receiver accessors. Methods inherit their
// target from the host decl rather than carrying their own.
func TestMethodBuilder_Accessors(t *testing.T) {
	t.Parallel()

	t.Run("Pos / Docs / Directive / TypeParam / Body / Receiver / Origin thread through", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test", defaultTarget)
		d := fixtureDirective()
		pos := fixturePos()
		origin := fixtureOrigin()
		body := emit.NewRawStmt(`return nil`)
		var n *emit.Method
		c.Package("p", "p").
			Struct("S", func(sb *builder.StructBuilder) {
				sb.Method("M", func(b *builder.MethodBuilder) {
					n = b.Node()
					b.Pos(pos).
						Docs("docs").
						Directive(d).
						Receiver("r", emit.Builtin("R")).
						TypeParam("T", nil).
						Body(body).
						Origin(origin)
				})
			})
		assertCommon(t, n.SourcePos, n.DocLines, n.DirectiveList, pos, d)
		if len(n.TypeParams) != 1 || len(n.Body) != 1 {
			t.Fatalf("method type param / body mis-applied")
		}
		if n.ReceiverName != "r" || n.Receiver == nil {
			t.Fatalf("receiver not threaded; got name=%q type=%v", n.ReceiverName, n.Receiver)
		}
		if n.Origin() != origin {
			t.Fatalf("Origin not threaded; got %v, want %v", n.Origin(), origin)
		}
	})
}

// TestPackageBuilder_Method covers the top-level Method
// constructor. The decl lands on [emit.Package.Methods] (not
// nested under a Struct/Interface/Alias); the Anchor's default
// origin is stamped as the method's Owner so the framework's
// downstream routing and rewire passes can resolve the receiver
// type. OwnerRef is populated in lock-step.
func TestPackageBuilder_Method(t *testing.T) {
	t.Parallel()

	t.Run("Method appends to Package.Methods", func(t *testing.T) {
		t.Parallel()
		anchor := &node.Enum{Name: "Status", Package: "example.com/store"}
		pkg, err := builder.For("enum").Anchor(anchor).
			Method("String", nil).
			Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if got := len(pkg.Methods); got != 1 {
			t.Fatalf("Methods len = %d, want 1", got)
		}
		if got := pkg.Methods[0].Name; got != "String" {
			t.Fatalf("Methods[0].Name = %q, want %q", got, "String")
		}
	})

	t.Run("Method stamps Owner from Anchor's default origin", func(t *testing.T) {
		t.Parallel()
		anchor := &node.Enum{Name: "Status", Package: "example.com/store"}
		pkg, err := builder.For("enum").Anchor(anchor).
			Method("String", nil).
			Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		m := pkg.Methods[0]
		if m.Owner == nil {
			t.Fatalf("Owner not stamped")
		}
		if got, want := m.Owner.OwnerName(), "Status"; got != want {
			t.Fatalf("Owner.OwnerName = %q, want %q", got, want)
		}
	})

	t.Run("Method stamps OwnerRef in lock-step with Owner", func(t *testing.T) {
		t.Parallel()
		anchor := &node.Enum{Name: "Status", Package: "example.com/store"}
		pkg, err := builder.For("enum").Anchor(anchor).
			Method("String", nil).
			Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		m := pkg.Methods[0]
		if m.OwnerRef.IsZero() {
			t.Fatalf("OwnerRef not stamped")
		}
		if got, want := m.OwnerRef.QName, "example.com/store.Status"; got != want {
			t.Fatalf("OwnerRef.QName = %q, want %q", got, want)
		}
	})

	t.Run("Method sets Package to the anchored package path", func(t *testing.T) {
		t.Parallel()
		anchor := &node.Enum{Name: "Status", Package: "example.com/store"}
		pkg, err := builder.For("enum").Anchor(anchor).
			Method("String", nil).
			Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if got, want := pkg.Methods[0].Package, "example.com/store"; got != want {
			t.Fatalf("Method.Package = %q, want %q", got, want)
		}
	})

	t.Run("MethodBuilder Receiver / Body / Returns flow through", func(t *testing.T) {
		t.Parallel()
		anchor := &node.Enum{Name: "Status", Package: "example.com/store"}
		pkg, err := builder.For("enum").Anchor(anchor).
			Method("String", func(m *builder.MethodBuilder) {
				m.Receiver("e", emit.External("example.com/store", "Status"))
				m.Return(emit.Builtin("string"))
				m.Body(emit.NewReturn(emit.NewLiteralString("active")))
			}).
			Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		m := pkg.Methods[0]
		if m.ReceiverName != "e" || m.Receiver == nil {
			t.Fatalf("receiver not threaded; name=%q type=%v", m.ReceiverName, m.Receiver)
		}
		if len(m.Returns) != 1 || len(m.Body) != 1 {
			t.Fatalf("returns/body not threaded: returns=%d body=%d", len(m.Returns), len(m.Body))
		}
	})
}
