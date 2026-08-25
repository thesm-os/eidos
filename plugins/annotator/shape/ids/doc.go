// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package ids re-exports every name the shipped catalog registers —
// one import for the words a consumer would otherwise spell as string
// literals.
//
//	shape.MixinParamKey(ids.MixinNotFound, notfound.ParamSentinel)
//	slices.Contains(shape.Mixins(bag), ids.MixinIdempotent)
//
// Each constant is its own package's Name rather than a copy of it,
// so a rename in the catalog is a compile error here instead of a
// literal that quietly stopped matching. A mistyped name was already
// reported at stamp time — an unregistered contract or mixin is a
// diagnostic, not a silence — but that is a run-time answer to a
// question the compiler can settle.
//
// The three families are prefixed because they share a namespace only
// by accident: `pure` is both a detector and a mixin, and the
// prefixes are what let both keep the name they are declared under.
//
// Names are not identifiers. `batch-writer`, `if-absent` and
// `rate-limit` hyphenate, and the write-through-cache contract
// registers as plain `cache` — so [ContractCache] names the
// `writethroughcache` package, and reading the constant is how a
// consumer learns that without grepping the catalog.
//
// This package deliberately carries names only. A parameter key
// belongs to one shape and is exported beside it (`ttl.ParamNotFound`,
// `cursor.ParamSentinel`), where its docblock explains what the value
// must name; lifting those here would separate each key from the one
// paragraph that says how it resolves.
package ids
