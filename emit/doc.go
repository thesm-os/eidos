// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package emit is the language-agnostic output model — what
// generators produce and backends consume. Every concrete emit kind
// (Struct, Interface, Method, Field, …) embeds [BaseEmit] for the
// shared position / docs / directives / metadata fields and the
// origin link back to the source [node.Node]. Each kind satisfies
// [Node] via its Kind() method.
//
// emit mirrors the [node] package's shape so generators can read a
// source [node.Struct] and produce a matching [Struct] in emit with
// minimal translation, but the two packages diverge at three points:
//
//   - Every emit node carries an [Origin] back-link to the source
//     node it was derived from (or nil for purely-generated artifacts).
//   - References to types are explicit about their resolution origin:
//     [TypeRef] for internal (points at another emit entity in this
//     run), [ExternalRef] for third-party / stdlib types
//     (imports needed at render time), [BuiltinRef] for true language
//     built-ins, and [CompositeRef] for compound shapes
//     (Pointer / Slice / Array / Map / Func / Union) over inner refs.
//   - Composable [Slot] regions on each kind allow cross-cutting
//     generators to inject contributions without owning the top-level
//     entity, with [Provenance] recording who appended what at which
//     authority.
//
// [Target] identifies the output file an emit entity belongs to;
// backends group emit by Target to render one file per group. Per the
// spec's mutability contract, emit values are mutable during the
// generator phase and frozen for downstream readers — append-only
// Slot semantics make this contract straightforward to maintain.
package emit
