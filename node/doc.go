// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package node is the language-agnostic input model — what a frontend
// produces after parsing source. Every concrete node kind (Struct,
// Interface, Method, Field, Function, …) embeds [BaseNode] for the
// shared position / docs / directives / metadata fields and
// satisfies [Node] via its Kind() method.
//
// Integration with foundation primitives:
//
//   - [position.Pos] on every node identifies where it came from.
//   - [meta.Bag] on every node carries typed extension metadata —
//     frontend stamps <lang>.*, shape detectors stamp shape.*,
//     plugins stamp their own namespaces.
//   - frontend — the one meta key this package declares, on every
//     [Package]: the name of the frontend that produced it, and so of
//     the language its declarations were written in. Declared here
//     because every frontend writes it and every bridge reads it, and
//     a key is interned by name — so each of them spelling the string
//     itself, as all three did, holds only while they keep agreeing.
//     See [MetaFrontend].
//   - [directive.Directive] values are parsed from source comments
//     and attached per node; the directive package's validator
//     enforces schemas against them.
//   - [Docs] preserves source documentation verbatim so backends
//     emitting derived types can carry the comments forward.
//
// Generic accessors (Methods / MethodByName / MethodsWith / Fields /
// FieldByName / FieldsWith / Directives / Directive / HasDirective)
// let plugin code traverse the graph by name or predicate without
// hand-rolling iteration. Composable predicate helpers ([ByName],
// [WithDirective], [WithMeta]) are reusable across kinds.
//
// Owner back-pointers (Method.Owner, Field.Owner, EnumVariant.Owner,
// TypeParam.Owner) allow upward traversal without re-finding the
// declaration. They are populated by the constructing frontend and
// must not be mutated downstream.
//
// [Walk] traverses a node tree via a [Visitor]; it visits each node
// in declaration order and yields control to the visitor at every
// step.
//
// [TypeRef] is the unified type-reference type. Its Kind discriminator
// distinguishes Named, Pointer, Slice, Array, Map, Func, and Chan
// forms — the common shapes across Go, Rust, and similar systems
// languages. Truly language-specific quirks ride on the TypeRef's
// metadata.
package node
