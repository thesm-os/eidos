// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package sdk is the plugin-author façade. It re-exports what a
// plugin names — the contracts it declares itself against, the
// source model it reads, the emit model it writes, and the
// metadata, diagnostic, position and store vocabulary that joins
// them — so a plugin's import block names the façade rather than
// the framework's layering.
//
// A Go-generating plugin imports three things:
//
//	import (
//	    "go.thesmos.sh/eidos/sdk"        // contracts + the models
//	    "go.thesmos.sh/eidos/sdk/golang" // the Go plugin base
//	    "go.thesmos.sh/eidos/lang/golang" // Go conventions + refs
//	)
//
// Everything below is about which names cross into this package
// and how they are spelled. The rules are stated because every
// alias here is public API on an independently versioned module:
// a name added is a name that cannot be withdrawn, and a name
// misspelled is one every plugin in the ecosystem learns wrong.
//
// # What belongs in the façade
//
// A symbol belongs here when a plugin has to name it. Three
// families qualify:
//
//   - Contracts. Anything the framework asserts against a plugin,
//     or that a plugin must name to declare itself: roles,
//     capabilities, per-phase contexts, annotator hooks, priority
//     buckets, directive schemas, the kind discriminator, the
//     emit-side interfaces the framework calls back on a plugin's
//     own values.
//   - The models. The source graph a plugin reads and the emit
//     graph it writes, along with everything that builds the
//     latter — the leaf constructors and the fluent builders a
//     generator assembles a whole package through. A generator's
//     whole body is these two.
//   - The joining vocabulary. Metadata, diagnostics, positions,
//     and the store handles the phase contexts carry — the types
//     a plugin names when it factors its own code into helpers.
//
// The test for the first family is whether a plugin can be
// *declared* without the symbol. The test for the other two is
// whether a plugin can be *written* without it. [OutputPackageSetter]
// passes the first; [NewLiteralString] passes the second.
//
// # What does not
//
// Anything a plugin has no business touching, however often the
// framework itself uses it. Concretely:
//
//   - Construction of things the pipeline owns. There is no
//     re-export of the store constructor, the reader constructor,
//     or the diagnostic-sink constructor: a plugin is handed all
//     three on its phase context, and one that built its own would
//     query a graph nothing else can see and record reads no cache
//     key hashes. Test scaffolding that legitimately needs them
//     builds through eidostest rather than through here.
//   - Pipeline internals and registries — plugin registration,
//     capability resolution, the read-set and index plumbing, the
//     diagnostic formatters the CLI renders through.
//   - Renderer-side discriminators. The expression, statement, and
//     composite-shape enums exist for a backend switching over
//     what a generator built; a generator uses the constructors
//     and never reads them back.
//   - A second spelling of something already here. The builder
//     package's expression and statement shorthands (`Str`,
//     `Ident`, `Return`) construct exactly what [NewLiteralString],
//     [NewIdent] and [NewReturn] do. Carrying both would make this
//     the place a plugin author learns two vocabularies for one
//     model, and would force collisions — [Return], [Tag],
//     [Constraint] and [Default] are all taken — that the rules
//     below would then have to resolve for no gain.
//   - The predicate helpers ([node.And], [store.WithDirective] and
//     their siblings). A store query takes a plain closure, so
//     nothing is unwritable without them, and the three packages
//     spell the same idea with three incompatible signatures — one
//     of which is generic over an unexported constraint and so
//     cannot be re-exported at all.
//
// A façade that re-exported everything would be the framework
// with one more name on it, and would freeze the layering it was
// meant to hide.
//
// # How a name is spelled
//
// Carried over verbatim, unless two packages would collide. Then:
//
//  1. The source model takes the bare name; the emit model takes
//     the Emit prefix. [Struct] is [node.Struct] and [EmitStruct]
//     is [emit.Struct]; likewise the kind constants, [NodeKindStruct]
//     against [EmitKindStruct]. Every declaration kind exists on
//     both sides carrying a different value, and confusing them
//     fails silently in both directions — a slot constrained on a
//     source kind accepts nothing an emit builder produces, and a
//     directive scoped to an emit kind matches no source node, so
//     the plugin never fires and nothing reports it. The source
//     side gets the bare name because it is the common case: every
//     annotator only reads it, and a generator reads it before it
//     writes anything.
//  2. Where this package already owns the bare name, both sides
//     are qualified. [Walk] is the annotator hook driver, so the
//     tree traversals are [NodeWalk] and [EmitWalk]. [Provenance]
//     is the contribution builder, so the record it stamps is
//     [EmitProvenance] and the metadata one is [MetaProvenance].
//  3. A name that would read as something else in a Go package is
//     qualified even without a collision — [StoreReader] rather
//     than a bare Reader, [SeverityError] rather than a bare Error.
//     Where such a name belongs to a family, the family is
//     requalified together: the slot insert positions are
//     [InsertPrepend], [InsertAt], [InsertBefore] and [InsertAfter]
//     because [At] already names the position constructor, and
//     renaming only the one that collides would leave three
//     siblings reading as a different idea from the fourth.
//
// The rule is applied for the plugin author's benefit, not the
// internal package's. Where a name is unambiguous on its own, it
// keeps its original spelling however deep it sits: [Ref], [Expr],
// [Stmt], [Builtin], [Ptr] and the New… constructor families come
// across unchanged, because a plugin author who knows the emit
// package should not have to learn a second spelling of it.
//
// # Aliases, never wrappers
//
// Every type here is a Go type alias. A wrapper would break
// identity with the underlying package, and a plugin holding both
// spellings of the same value — its own through the façade, the
// pipeline's through the layer — would stop compiling. The
// identity tests in this package's _test files assert exactly
// that, one assignment per alias, so a wrapper introduced by
// accident fails at build rather than at a plugin boundary.
//
// Generic functions are the one exception the language forces:
// Go cannot bind an uninstantiated generic function to a variable,
// so [NewKey], [EnsureKey] and [BindOptions] are thin generic
// wrappers. They return the aliased type, so identity survives.
//
// # File map
//
//   - plugin.go: roles, capabilities, contexts, hooks
//   - priority.go: Priority type + canonical buckets
//   - directive.go: schemas, parsing, validation, registry, errors
//   - kind.go: Kind type + the NodeKind source kinds
//   - options.go: typed options binding
//   - node.go: the source model
//   - emit_model.go: the emit model + the EmitKind emit kinds
//   - emit_build.go: refs, expressions, statements
//   - emit_builder.go: the fluent builders, insert positions,
//     type-argument lifting
//   - emit.go: emit contracts — base value, refs, provenance,
//     slots, output-package dispatch
//   - meta.go: bags, typed keys, parsers, authorities
//   - diag.go: sinks, diagnostics, severities
//   - position.go: source positions and ranges
//   - store.go: the store handles, views, and failure modes
package sdk
