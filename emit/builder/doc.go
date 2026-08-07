// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package builder offers a fluent construction API for the emit
// graph. It is the production surface plugin authors use inside
// [plugin.Generator.Generate] / [plugin.Annotator.Annotate] to
// assemble [emit.Package], [emit.Struct], [emit.Function], … and
// to contribute to other generators' cross-cutting slots.
//
// The package is opinionated about three things every plugin
// otherwise has to remember by hand:
//
//   - **Target threading.** Every decl going into a given file
//     carries the same [emit.Target]. A [Context] bound to a default
//     Target spreads it across every decl produced through it, so a
//     plugin emitting 30 entities into one file declares the Target
//     once.
//   - **Owner back-pointers.** Sub-elements ([emit.Field],
//     [emit.Method], [emit.Embed], [emit.EnumVariant],
//     [emit.Param], [emit.TypeParam]) carry `Owner` fields that
//     the emit-store re-wires after JSON deserialisation but
//     plugin authors otherwise have to populate by hand. The
//     builders wire them automatically as nested callbacks return.
//   - **Slot provenance.** Cross-cutting contributions stamp
//     [emit.Provenance.SetBy] with the contributing plugin's name.
//     A Context bound to that name once threads it onto every slot
//     append, so an annotator can't accidentally land an unsigned
//     contribution.
//
// # Quick start
//
//	func (g *RepoGen) Generate(gctx *plugin.GeneratorContext) error {
//	    target := emit.Target{Dir: "users", Filename: "repo_gen.go", Package: "users"}
//	    c := builder.For(g.Name())
//	    pkg := c.Package("users", "example.com/users").
//	        Struct("UserRepo", func(s *builder.StructBuilder) {
//	            s.Field("DB", emit.External("database/sql", "DB"), nil)
//	            s.Method("Get", func(m *builder.MethodBuilder) {
//	                m.Receiver("r", emit.Ptr(emit.Internal(s.Node())))
//	                m.Param("ctx", emit.External("context", "Context"))
//	                m.Param("id", emit.Builtin("string"))
//	                m.Return(emit.Ptr(emit.Internal(s.Node())), nil)
//	                m.Return(emit.Builtin("error"), nil)
//	            })
//	        }).
//	        Build()
//	    return gctx.Store.Emit().AddPackage(pkg)
//	}
//
// # Cross-cutting contributions
//
// An annotator or generator contributing into an existing host
// emits through the slot-append API on [Context], which stamps the
// host's slot with `Provenance{SetBy: c.SetBy()}`:
//
//	c := builder.For("validation")
//	if err := c.AppendField(userStruct, &emit.Field{
//	    Name: "Email",
//	    Type: emit.Builtin("string"),
//	    Tag:  `validate:"email"`,
//	}); err != nil {
//	    return err
//	}
//	if err := c.AppendInit(file, emit.NewExprStmt(/* validators.Register(...) */)); err != nil {
//	    return err
//	}
//
// # Layering
//
// The package depends only on `emit` and standard library — it
// never imports `store`, `plugin`, or `pipeline`. Plugin authors
// retrieve the live [emit.File] for a Target via
// `gctx.Store.Emit().FileFor(target)` and pass it into the slot
// helpers; the builder does not reach into the store directly.
package builder
