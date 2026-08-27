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
// Beside the names sit the catalog's answers about them. The set
// functions ([Detectors], [Contracts], [Mixins], [ContractParams],
// [MixinParams]) enumerate what is registered; the lookups
// ([ContractOf], [MixinOf], [DetectorOf], [ContractParam],
// [MixinParam]) answer for one name — the roles a contract declares,
// a key's [shape.ParamKind], its role scope, whether a directive must
// carry it. All of it is read off the registered specs rather than
// restated, so what this package answers is what the validator
// enforces; the hand-maintained part is the constants alone, and the
// tests pin that every registered name and key has one.
//
// What stays with each shape's own package is the prose: a key's
// docblock there explains what its value must name and how it
// resolves, which no lookup can carry. Reach for the declaring
// package when writing a directive, and for this one when consuming
// the catalog.
package ids
