// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package commutative recognises the commutative mixin — the assertion that
// combining results in any order yields the same value, so concurrent contributions need no arbitration.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin commutative
package commutative
