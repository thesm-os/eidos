// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package windowed recognises the windowed mixin — the assertion that
// the result covers a bounded window of recent input rather than the whole history.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin windowed
package windowed
