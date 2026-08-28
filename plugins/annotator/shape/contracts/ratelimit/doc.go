// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package ratelimit recognises the rate-limit contract — a
// callable bounded by a per-time-unit rate and an instantaneous
// burst capacity. Both are opaque literals: they are quantities, and
// neither names an error.
//
// `limited=` names the sentinel a refused call reports, which is what
// makes the burst statable — with `burst=N` a check counts to N+1 and
// requires a refusal, and until the refusal has a name any error
// satisfies it, including one from a limiter that was never
// implemented. Optional; a consumer that cannot state the law without
// it declines to state it.
//
// The rate itself is a deeper sentence: it needs time to pass, where
// the burst is a fixed call sequence a single caller settles.
//
// The recognised directives are:
//
//	//+gen:contract rate-limit role=fn rate=100 burst=10
//	//+gen:contract rate-limit role=fn rate=100 burst=10 limited=ErrRateLimited
package ratelimit
