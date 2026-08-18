// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package pool recognises the pool contract — an acquire / release
// pair (Get / Put) with the invariant that every Get balances
// exactly one Put. Carries a [Validate] hook that flags pool
// declarations missing either side.
//
// The optional `stats` role names the accounting observation — the
// method reporting outstanding, idle or waited counts — beside the
// cycle it accounts for. A pool without one is still a pool, so it is
// not required; what it buys is a balance or leak law reading the
// numbers from a method the resolver has qualified and back-stamped,
// rather than from a closure hand-wired at run time against a name
// nothing validated.
//
// The recognised directive (on the get side) is:
//
//	//+gen:contract pool role=get put=Put
//	//+gen:contract pool role=get put=Put stats=Stats
package pool
