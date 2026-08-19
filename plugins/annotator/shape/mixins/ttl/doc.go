// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package ttl recognises the ttl mixin — the assertion that an entry
// stops being readable once its time-to-live has elapsed.
//
// The law advances a controlled clock past the lifetime and requires
// the read to miss, which needs every part of that sentence named: the
// `duration` it advances past, the `put` and `read` it drives, and the
// `notfound` sentinel it compares the miss against. With no duration
// it advances past zero, which either fails a correct implementation
// or passes vacuously depending on which side of the boundary zero
// lands.
//
// `notfound=` is the lapsed read's sentinel specifically. When it is
// absent, a consumer falls back to the miss sentinel the read declares
// for itself via the `notfound` mixin — the two usually coincide, and
// naming one here is only necessary when expiry reports differently
// from plain absence.
//
// Distinct from the timeout mixin, which is about a callable returning
// promptly when its context expires. That is a deadline on an
// operation; this is a lifetime on stored data.
//
// The recognised directive is:
//
//	//+gen:mixin ttl duration=5m put=Set read=Get notfound=ErrMissing
package ttl
