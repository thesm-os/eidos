// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package circuitbreaker

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "circuit-breaker"

// RoleFn is the callable the breaker guards.
const RoleFn = "fn"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleFn}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles}
}
