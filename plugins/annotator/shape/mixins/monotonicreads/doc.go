// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package monotonicreads recognises the monotonicreads mixin — the assertion that
// successive reads by one client never observe an older value than one it has already seen.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin monotonicreads
package monotonicreads
