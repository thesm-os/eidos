// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package associative recognises the associative mixin — the assertion that
// combining results in any grouping yields the same value, so a suite may fold a batch in any shape.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin associative
package associative
