// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package poisonable recognises the poisonable mixin — the assertion
// that the annotated accessor reports a sticky failure state, and that
// the named callable is what induces it.
//
// Stamped on the accessor rather than the inducer, because the
// accessor is what a law reads: a suite induces the state once and
// then asserts every subsequent read agrees. The `induce` param names
// the operation that puts the subject there, without which a law can
// observe the healthy state and nothing else.
//
// The structural half is already detected. A `func() error` is the
// poison-accessor shape, and a detector can say that much; it cannot
// say which operation breaks it, because that is a fact about the type
// rather than about the signature. This mixin carries the half a
// signature cannot.
//
// The recognised directive is:
//
//	//+gen:mixin poisonable induce=Poison
package poisonable
