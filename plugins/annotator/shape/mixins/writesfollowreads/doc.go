// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package writesfollowreads recognises the writesfollowreads mixin — the assertion that
// a write issued after a read is ordered after the write that read observed.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin writesfollowreads
package writesfollowreads
