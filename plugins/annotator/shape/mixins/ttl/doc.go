// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package ttl recognises the ttl mixin — the assertion that an entry
// stops being readable once its time-to-live has elapsed.
//
// The law advances a controlled clock past the lifetime and requires
// the read to miss, which needs every part of that sentence named: the
// lifetime it advances past, the `put` and `read` it drives, and the
// `notfound` sentinel it compares the miss against. With no lifetime
// at all it advances past zero, which either fails a correct
// implementation or passes vacuously depending on which side of the
// boundary zero lands — so a consumer with neither key states the
// classification and derives no check from it.
//
// The lifetime is named one of two ways, and never both. `duration=`
// fixes one for every entry, which is the API-wide expiry a rate
// limiter or a fixed-TTL cache has. `lifetime=` names the member of
// the stored value that carries its own, which is the commoner shape
// in caches and session stores:
//
//	type Entry struct {
//	    Body     string
//	    Lifetime time.Duration
//	}
//
// Before `lifetime=` existed such a store could say nothing. Its
// declaration was complete and the law was the same sentence, but the
// only key on offer asserted a fixed lifetime it does not have — so
// the honest choice was to misdescribe the store or drop the
// classification.
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
// The recognised directives are:
//
//	//+gen:mixin ttl duration=5m put=Set read=Get notfound=ErrMissing
//	//+gen:mixin ttl lifetime=Lifetime put=Put read=Read notfound=ErrExpired
package ttl
