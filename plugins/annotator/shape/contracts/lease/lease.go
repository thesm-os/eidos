// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package lease

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "lease"

// ParamHeld is the KV key naming the error a second acquire reports while the lease is held.
//
// A sentinel is a package-level var, so the resolver rewrites it
// through the var scope rather than the callable one — see
// [shape.Contract.SiblingVars]. Absence is not an error: the bare
// form still classifies, and a suite that cannot state the law
// without a sentinel declines to state it.
const ParamHeld = "held"

// ParamTimeout is the KV key naming how long the lease is held before it lapses.
//
// A law checking that a cancelled acquire releases has to wait past it. A
// bound nobody declared is a number the generator invented, and a law
// enforcing an invented bound fails implementations that are correct against
// the one their author meant.
//
// Opaque to the resolver: a quantity names neither a callable nor a var.
const ParamTimeout = "timeout"

// Params enumerates the directive's opaque KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []string{ParamHeld, ParamTimeout}

// SiblingVars enumerates the param keys whose values name
// package-level vars the resolver rewrites into qualified names.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var SiblingVars = []string{ParamHeld}

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{"acquire", "release"}

// Contract returns the [shape.Contract] this package contributes.
// The acquire side requires a release partner.
func Contract() shape.Contract {
	return shape.Contract{
		Name:        Name,
		Roles:       Roles,
		Params:      Params,
		SiblingVars: SiblingVars,
		Required:    map[string][]string{"acquire": {"release"}},
	}
}
