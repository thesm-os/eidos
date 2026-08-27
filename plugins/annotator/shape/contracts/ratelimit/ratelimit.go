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

// Params enumerates the directive's opaque KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamRate, Kind: shape.KindOpaque},
	{Key: ParamBurst, Kind: shape.KindOpaque},
}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles, Params: Params}
}
