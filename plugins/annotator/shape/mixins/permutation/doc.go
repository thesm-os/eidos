// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package permutation recognises the permutation mixin — the assertion that
// iteration yields every element exactly once in an unspecified order, so a suite must compare as a set.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin permutation
package permutation
