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
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin readyourwrites
package readyourwrites
