// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package tsfixture programmatically constructs a populated
// [store.Store] for unit-testing plugins in isolation, without going
// through a real frontend.
//
// The package is the unit-level layer of the TypeScript testing
// surface: where [pipelinetest] runs a full pipeline through a
// synthetic frontend, tsfixture stops at the store boundary and hands
// a plugin's Annotate / Generate code the same view it would receive
// after a TypeScript frontend run. [gofixture] is the same package
// for Go, and the conformance suites in `eidostest` take a
// [store.Store] and never ask where it came from — which is what lets
// each language answer that question separately.
//
// # Why this is a TypeScript package rather than a neutral one
//
// The type-reference constructors below are TypeScript's type grammar
// — [Union], [Tuple], [Record], [Nullable] — and
// [Builder.TSSource] projects the graph back into TypeScript source.
// Both ends are TypeScript, so there is no neutral core to keep
// behind. Running the real frontend over that projection and getting
// the graph back is the round trip the package exists for.
//
// # API shape
//
// A [Builder] is created with [New]; declarations are added via
// methods that accept a configuration callback to populate nested
// shape (properties on an interface, methods on a class, parameters
// on a function). [Builder.Build] returns a fresh [store.Store] every
// call; the builder remains reusable.
//
//	store := tsfixture.New().
//	    Interface("User", func(i *tsfixture.InterfaceBuilder) {
//	        i.Field("id", tsfixture.Named("string"), nil)
//	        i.Field("tags", tsfixture.Array(tsfixture.Named("string")), func(f *tsfixture.FieldBuilder) {
//	            f.Optional()
//	        })
//	        i.Method("greet", func(m *tsfixture.MethodBuilder) {
//	            m.Param("loud", tsfixture.Named("boolean"))
//	            m.Return(tsfixture.Named("string"))
//	        })
//	    }).
//	    Build()
//
// # Interfaces carry properties
//
// [InterfaceBuilder.Field] is not a mistake. A TypeScript interface
// declares properties and methods alike, which is why [node.Interface]
// has a field list — see ADR-0008. A fixture that could only put
// methods on one would be unable to spell the common case.
//
// # Source positions are routing input
//
// Every declaration the fixture builds carries a synthetic source
// position, `<pkg>/<lowercased-name>.ts`, and properties and methods
// inherit their enclosing declaration's. This is not cosmetic: the
// Layout phase composes a generated file's name as
// `<origin-basename><plugin-suffix>`, so a positionless node routes
// its output to the bare suffix. Fixture-driven pipelines therefore
// route to production-shaped filenames by default. Call a
// sub-builder's Pos to pin a specific one; see [ClassBuilder.Pos].
//
// Routing overrides are the other half of that story, and the
// `+gen:out` directive has a trap sharp enough to warrant its own
// constructor: its value is a path, so a directory spelled without a
// trailing separator routes the output to a *file* of that name, and
// no diagnostic is raised. Build the directive with [RouteTo], which
// cannot get the separator wrong.
//
// # Modifiers are metadata, and some of them are grammar
//
// TypeScript's declaration syntax carries modifiers the neutral model
// has no field for — `?`, `readonly`, `static`, `private`, `async` —
// so the frontend records them under the `ts.*` keys and the fixture
// has to as well. The ones a source author *writes* have builder
// methods ([FieldBuilder.Optional], [MethodBuilder.Async] and their
// siblings), because a fixture that cannot spell `name?: string` is
// not describing TypeScript.
//
// Everything else does not. Retrieve the node via
// [Builder.PackageNode] (or a sub-builder's Node accessor) and use
// the meta key's typed setter directly. Keeping the rest off the
// builder API avoids a parallel surface that would lag
// `lang/typescript`'s vocabulary.
//
// # The TypeScript a consumer would have written
//
// A render fixture needs two things that describe the same
// declarations: the node graph that drives the run, and the
// TypeScript the generated output imports from.
// [Builder.TSSource] projects the second from the first, so the pair
// is correct by construction rather than by review:
//
//	gen := typescripttest.Rendered(t, run).
//	    WithSource(typescripttest.TSFile(fixture().TSSource()))
//
// It returns plain strings rather than a typescripttest value on
// purpose: this package builds a run's input and typescripttest
// asserts on its output, and neither should have to compile the
// other. A construct the projection cannot spell stops the test
// naming it, rather than producing a support module the fixture no
// longer describes.
package tsfixture
