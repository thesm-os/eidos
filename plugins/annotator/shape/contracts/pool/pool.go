// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pool

import (
	"fmt"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
)

// Name is the canonical contract name this package stamps.
const Name = "pool"

// Roles enumerates the contract's role vocabulary.
//
// `stats` names the accounting observation beside the cycle it
// accounts for, and is optional: a pool without one is still a pool,
// and a law reading the numbers simply does not bind. It stays out of
// [Contract.Required] for that reason, while the validator's
// one-per-role rule still applies — two accounting methods would leave
// a reader of the numbers with no way to choose.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{"get", "put", "stats"}

// Contract returns the [shape.Contract] this package contributes.
// The contract requires both `get` and `put` partners on the
// host (Get side) declaration, and ships a [shape.ContractValidator]
// that flags pool instances where either side is missing after
// resolver back-stamping. `stats` is not required — see [Roles].
func Contract() shape.Contract {
	return shape.Contract{
		Name:     Name,
		Roles:    Roles,
		Required: map[string][]string{"get": {"put"}},
		Validate: validate,
	}
}

// validate enforces the pool's structural invariant: exactly one
// callable per role. Two Gets would mean two distinct pools
// folded into one contract membership, which the downstream
// codegen cannot reconcile.
func validate(members map[string][]shape.ContractMember) []shape.ContractViolation {
	var out []shape.ContractViolation
	for _, role := range Roles {
		got := len(members[role])
		if got <= 1 {
			continue
		}
		for _, m := range members[role][1:] {
			out = append(out, shape.ContractViolation{
				Host: m.Host,
				Message: fmt.Sprintf(
					"pool requires exactly one %s; got %d callables", role, got,
				),
			})
		}
	}
	return out
}
