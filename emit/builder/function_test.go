// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
)

// TestFunctionBuilder_ParamsAndReturns covers the function surface
// — Params wire Owner; Return appends accumulate; named-return
// names thread onto the underlying [emit.Return].
func TestFunctionBuilder_ParamsAndReturns(t *testing.T) {
	t.Parallel()

	t.Run("variadic param marks the underlying Param.Variadic", func(t *testing.T) {
		t.Parallel()
		c := builder.For("repogen").WithTarget(defaultTarget)
		var f *emit.Function
		c.Package("io", "example.com/io").
			Function("Sum", func(fb *builder.FunctionBuilder) {
				f = fb.Node()
				fb.Param("nums", emit.Builtin("int"), func(pb *builder.ParamBuilder) {
					pb.Variadic()
				})
				fb.Return(emit.Builtin("int"))
			})
		if len(f.Params) != 1 || !f.Params[0].Variadic {
			t.Fatalf("expected variadic param; got %+v", f.Params)
		}
		if f.Params[0].Owner != f {
			t.Fatalf("param Owner not wired on function")
		}
		if len(f.Returns) != 1 {
			t.Fatalf("expected one return slot; got %d", len(f.Returns))
		}
	})

	t.Run("Return accepts an optional name and stamps it on the slot", func(t *testing.T) {
		t.Parallel()
		c := builder.For("repogen").WithTarget(defaultTarget)
		var f *emit.Function
		c.Package("io", "example.com/io").
			Function("F", func(fb *builder.FunctionBuilder) {
				f = fb.Node()
				fb.Return(emit.Builtin("int"), "result")
				fb.Return(emit.Builtin("error"), "err")
			})
		if len(f.Returns) != 2 || f.Returns[0].Name != "result" || f.Returns[1].Name != "err" {
			t.Fatalf("named returns not threaded; got %+v", f.Returns)
		}
	})
}

// TestFunctionBuilder_Accessors covers the Pos / Docs / Directive /
// Target / TypeParam / Body accessors.
func TestFunctionBuilder_Accessors(t *testing.T) {
	t.Parallel()

	t.Run("Pos / Docs / Directive / Target / TypeParam / Body / Origin thread through", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		other := otherTarget()
		d := fixtureDirective()
		pos := fixturePos()
		origin := fixtureOrigin()
		body := emit.NewRawStmt(`return nil`)
		var n *emit.Function
		c.Package("p", "p").
			Function("F", func(b *builder.FunctionBuilder) {
				n = b.Node()
				b.Pos(pos).Docs("docs").Directive(d).Target(other).
					TypeParam("T", nil).Body(body).Origin(origin)
			})
		assertCommon(t, n.SourcePos, n.DocLines, n.DirectiveList, pos, d)
		if n.Target != other {
			t.Fatalf("target override failed")
		}
		if len(n.TypeParams) != 1 || len(n.Body) != 1 {
			t.Fatalf("type param / body mis-applied; %+v %+v", n.TypeParams, n.Body)
		}
		if n.Origin() != origin {
			t.Fatalf("Origin not threaded; got %v, want %v", n.Origin(), origin)
		}
	})
}
