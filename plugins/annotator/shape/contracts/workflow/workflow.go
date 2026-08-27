// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package workflow

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "workflow"

// RoleFn is the callable the workflow advances.
const RoleFn = "fn"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleFn}

// ParamTransitions is the KV key naming the state moves the workflow
// permits.
//
// Opaque: the value is a workflow's own state vocabulary, and a
// parser here would fix a notation for every consumer.
const ParamTransitions = "transitions"

// Params enumerates the directive's opaque KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamTransitions, Kind: shape.KindOpaque},
}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles, Params: Params}
}
