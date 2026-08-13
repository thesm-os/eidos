// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package poisonaccessor recognises the poison-accessor shape —
// a callable that takes nothing and reports success or failure
// solely via an error return (`func () error` in Go).
//
// A positive detection stamps the structural shape on the
// callable's meta bag (via the umbrella shape plugin):
//
//	shape = "poisonaccessor"
//
// No key or value type is stamped — poison accessors have
// neither.
//
// A teardown wears the same signature and is claimed first by
// [closer], which gates on the method name. The two carry opposite
// semantics — a latch answers the same every time, a close-once
// teardown deliberately does not — so a law selecting on this shape
// would redden a correct Close. Structure cannot separate them; the
// name is the only signal there is.
//
// This shape says a callable reports a latched failure. It does not
// say anything can cause one: an Err() with nothing that trips it is a
// signature, not a latch. The claim that a subject can be poisoned,
// and by what, is the poisonable mixin”'s induce= — a promise, and so
// declared rather than detected.
package poisonaccessor
