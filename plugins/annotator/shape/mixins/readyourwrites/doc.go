// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package readyourwrites recognises the readyourwrites mixin — the assertion that
// a read by one client never returns a value older than that same client's most recent write.
//
// The fourth of the four session guarantees, beside [monotonicreads],
// [monotonicwrites] and [writesfollowreads]. Distinct from the
// readafterwrite mixin, which names a write partner and makes a
// per-method claim: this one is checked across a client's trace.
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
//	//+gen:mixin readyourwrites version=Version
package readyourwrites
