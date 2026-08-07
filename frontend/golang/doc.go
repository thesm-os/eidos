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
// channels become Named refs carrying channel metadata; iter.Seq /
// iter.Seq2 patterns surface as metadata on an ordinary
// [node.Function]; type-set generic constraints attach metadata to
// the [node.TypeParam] rather than inflating [node.Constraint] with
// Go-specific fields.
//
// # Meta-key catalog
//
// The catalog below is the complete set of keys the converter
// stamps. Each is an exported `Meta*` registry singleton declared
// in meta.go, read typed through [meta.Key.Get] and string-keyed
// from templates through the `metaBool` / `metaStr` funcmap
// helpers. `docs/frontend/golang.md` lays out the same catalog
// grouped by host node kind, with worked read examples.
//
// Every stamp records full provenance — author `"golang"` at
// [meta.AuthorityPlugin], positioned at the declaration that
// motivated it — so `eidos explain` traces each fact back to its
// source expression. Boolean keys are written only when the fact
// holds: absence is the negative, a `false` value is never stamped.
//
//   - frontend — cross-frontend provenance marker on every produced
//     [node.Package]; string value `"golang"`. The one key outside
//     the `go.*` namespace and the only one declared through
//     [meta.EnsureKey] rather than [meta.NewKey], because the
//     protobuf frontend declares the same name independently.
//     Bridge annotators and the cross-namespace audit step filter
//     their walks by reading it.
//   - The `go.*` keys themselves are declared and documented in
//     [go.thesmos.sh/eidos/lang/golang], which every Go-speaking
//     consumer can import — this frontend stamps them, the Go
//     backend reads them, and plugins query them. Listing them
//     here as well would be a second copy to drift.
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
