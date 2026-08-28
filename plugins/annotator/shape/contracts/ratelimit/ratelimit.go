// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ratelimit

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "rate-limit"

// RoleFn is the callable the limiter admits or refuses.
const RoleFn = "fn"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleFn}

// ParamRate is the KV key naming the sustained rate the limiter
// admits, and ParamBurst the allowance above it a caller may spend at
// once.
//
// Both opaque: a rate is a quantity with a unit this package has no
// vocabulary for, and reading one would mean fixing that vocabulary
// for every consumer.
const (
	ParamRate  = "rate"
	ParamBurst = "burst"
)

// ParamLimited is the KV key naming the sentinel a refused call
// reports.
//
// [ParamBurst] is the half a single caller settles and needs no
// clock: with `burst=N`, N calls in a row are admitted and the
// N+1st is not. That is a fixed call sequence — but "is not
// admitted" is only checkable once a refusal can be recognised, and
// without a name any error satisfies it. A subject whose limiter is
// unimplemented and whose eleventh call fails for its own reasons
// passes exactly as a correct one does.
//
// The identity argument every guarded claim in this vocabulary has
// already won: `deleteremoves` has `sentinel=`, `lease` `held=`,
// `if-absent` `conflict=`, `cas` `mismatch=`, and `orderafter`
// `unready=` — each because "the call is refused" says nothing until
// the refusal has a name.
//
// A package-level var, so it resolves through the var scope rather
// than the callable one — see [shape.KindVar]. Not a counterexample:
// it names how a refusal is recognised, not an input that provokes
// one, and the burst check provokes its refusal by counting rather
// than by handing over a declared value. Optional; the bare form
// still classifies, and a consumer that cannot state the law without
// it declines to state it.
const ParamLimited = "limited"

// Params enumerates the directive's KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamBurst, Kind: shape.KindOpaque},
	{Key: ParamLimited, Kind: shape.KindVar},
	{Key: ParamRate, Kind: shape.KindOpaque},
}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles, Params: Params}
}
