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
// # Type-reference helpers
//
// The package-level constructors ([Named], [PkgNamed], [Pointer],
// [Slice], [Array], [Map], [Func], [WithArgs]) build [node.TypeRef]
// values without manual struct-literal verbosity. They are pure
// functions; callers may compose freely.
//
// # Metadata and inspection
//
// To set typed metadata on a fixture node, retrieve the node via
// [Builder.PackageNode] (or via the *Builder helpers exposed by each
// sub-builder's Node accessor) and use the meta key's typed setter
// directly. Keeping meta off the builder API avoids accumulating a
// parallel surface that would inevitably lag the real meta package.
package storefixture
