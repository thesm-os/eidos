// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package storefixture programmatically constructs a populated
// [store.Store] for unit-testing plugins in isolation, without going
// through a real frontend.
//
// The package is the unit-level layer of the testing surface: where
// [pipelinetest] runs a full pipeline through a synthetic frontend,
// storefixture stops at the store boundary and hands a plugin's
// Annotate / Generate code the same view it would receive after a
// frontend run.
//
// # API shape
//
// A [Builder] is created with [New]; declarations are added via
// methods that accept a configuration callback to populate nested
// shape (fields on a struct, methods on an interface, parameters on
// a function). [Builder.Build] returns a fresh [store.Store] every
// call; the builder remains reusable.
//
//	store := storefixture.New().
//	    Struct("User", func(s *storefixture.StructBuilder) {
//	        s.Field("ID", storefixture.Named("string"), nil)
//	        s.Method("Validate", func(m *storefixture.MethodBuilder) {
//	            m.Param("ctx", storefixture.PkgNamed("context", "Context"))
//	            m.Return(storefixture.Named("error"))
//	        })
//	    }).
//	    Build()
//
// # Source positions are routing input
//
// Every declaration the fixture builds carries a synthetic source
// position, `<pkg>/<lowercased-name>.go`, and fields and methods
// inherit their enclosing declaration's. This is not cosmetic: the
// Layout phase composes a generated file's name as
// `<origin-basename><plugin-suffix>`, so a positionless node routes
// its output to the bare suffix — `_repo.go`, a basename the Go
// toolchain discards before it ever loads the file. Fixture-driven
// pipelines therefore route to production-shaped filenames by
// default. Call a sub-builder's Pos to pin a specific one; see
// [StructBuilder.Pos].
//
// Routing overrides are the other half of that story, and the
// `+gen:out` directive has a trap sharp enough to warrant its own
// constructor: its value is a path, so a directory spelled without a
// trailing separator routes the output to a *file* of that name, and
// no diagnostic is raised. Build the directive with [RouteTo], which
// cannot get the separator wrong.
//
// # Type-reference helpers
//
// The package-level constructors ([Named], [PkgNamed], [Pointer],
// [Slice], [Array], [Map], [Func], [WithArgs]) build [node.TypeRef]
// values without manual struct-literal verbosity. They are pure
// functions; callers may compose freely.
//
// # The Go a consumer would have written
//
// A render fixture needs two things that describe the same
// declarations: the node graph that drives the run, and the Go source
// the generated output references. [Builder.GoSource] projects the
// second from the first, so the pair is correct by construction
// rather than by review:
//
//	gen := golangtest.Rendered(t, run).
//	    WithSource(golangtest.GoFile(fixture().GoSource()))
//
// It returns plain strings rather than a golangtest value on purpose:
// this package builds a run's input and golangtest asserts on its
// output, and neither should have to compile the other. A construct
// the projection cannot spell stops the test naming it, rather than
// producing a support package the fixture no longer describes.
//
// # Metadata and inspection
//
// To set typed metadata on a fixture node, retrieve the node via
// [Builder.PackageNode] (or via the *Builder helpers exposed by each
// sub-builder's Node accessor) and use the meta key's typed setter
// directly. Keeping meta off the builder API avoids accumulating a
// parallel surface that would inevitably lag the real meta package.
package storefixture
