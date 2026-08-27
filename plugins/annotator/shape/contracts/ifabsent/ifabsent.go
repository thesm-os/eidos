// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ifabsent

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "if-absent"

// ParamConflict is the KV key naming the error a refused write
// reports when the key is already held.
//
// The whole claim is the refusal, and without the sentinel a check can
// assert only that some error came back — which a store refusing every
// write passes, because nothing separates "refused for presence" from
// "refused". Naming it turns the check into an identity assertion.
//
// A sentinel is a package-level var, so it resolves through the var
// scope rather than the callable one — see [shape.KindVar].
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
const ParamConflict = "conflict"

// Params enumerates the directive's opaque KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamConflict, Kind: shape.KindVar},
}

// RoleWriter is the insert-if-absent callable.
const RoleWriter = "writer"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleWriter}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{
		Name:   Name,
		Roles:  Roles,
		Params: Params,
	}
}
