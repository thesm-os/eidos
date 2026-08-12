// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package writesfollowreads recognises the writesfollowreads mixin — the assertion that
// a write issued after a read is ordered after the write that read observed.
//
// The `version` param names the member of the read or written value
// carrying the stamp the guarantee orders by — a logical clock, a row
// version, the global write order. A law replaying a trace reads it
// off each operation, and nothing in a signature says which member it
// is.
//
// The claim itself is declared rather than detected: no signature
// reveals it.
//
// The recognised directive is:
//
//	//+gen:mixin writesfollowreads version=Version
package writesfollowreads
