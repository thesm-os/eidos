// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package golang is the Go-language frontend for eidos. It loads Go
// source packages via [golang.org/x/tools/go/packages] and converts
// their declarations into the language-agnostic [node] model — every
// struct, interface, method, field, function, variable, constant,
// type alias, embedded type, and generic type parameter from the
// loaded packages surfaces as the corresponding node kind with
// correct back-pointers, doc comments preserved verbatim, and
// `+gen:` / `-gen:` directives parsed against the pipeline's
// directive registry.
//
// # Pipeline integration
//
// The frontend implements [plugin.Frontend]; register it on a
// [pipeline.Builder] via [pipeline.Builder.WithFrontend]:
//
//	pipeline.New().
//	    WithFrontend(golang.New()).
//	    WithBackend(backend.New()).
//	    Build()
//
// Load is invoked once per [plugin.FrontendContext.Pattern]. Each
// pattern is interpreted by [golang.org/x/tools/go/packages.Load]
// using the conventional Go-toolchain rules — module paths, "./..."
// recursive scopes, and explicit file lists are all accepted.
//
// # Language-agnostic shapes; Go-specific facts in metadata
//
// The frontend never leaks Go-specific shapes into [node] —
// channels become Named refs with `go.isChannel` / `go.chanDir`
// metadata; iter.Seq / iter.Seq2 patterns surface as `go.isIterSeq`
// / `go.iterValueType` keys; type-set generic constraints attach
// `go.constraintTerms` metadata to the [node.TypeParam]'s
// constraint rather than inflating [node.Constraint] with
// Go-specific fields. The full set of `go.*` metadata keys is
// documented on the constants in meta.go.
//
// # Cache integration
//
// Each Load call hashes its input file bytes plus [Version] and
// composes a [cache.NewKey]-shaped cache key. On hit the cached
// JSON-serialized [node.Package] is deserialized and re-wired via
// [node.RewireOwners]; on miss the AST conversion runs and the
// result is written back to the cache for the next run.
//
// # Concurrency
//
// Load is safe to call concurrently across patterns — each
// invocation builds its own package set and writes through the
// per-package locks the [store] enforces. Parallel-frontend
// dispatch is opt-in at the pipeline level via
// [pipeline.Builder.WithParallel].
package golang
