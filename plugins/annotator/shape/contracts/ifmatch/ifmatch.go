// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ifmatch

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "if-match"

// RoleWriter is the conditional write the contract is declared on.
const RoleWriter = "writer"

// RoleMatch is the callable deciding whether the write proceeds.
//
// Distinct from [ParamPred], which spells the same decision as an
// expression the resolver must not touch. The two cannot share a key:
// a value that is qualified and one that is left verbatim are
// different treatments, and a single key would leave the resolver
// guessing which it received.
const RoleMatch = "match"

// ParamPred is the KV key carrying the predicate as an expression.
//
// Opaque by design — `pred=Version==Expected` names no callable, so
// there is nothing for the resolver to look up and every attempt would
// report a correct directive as unresolved.
const ParamPred = "pred"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleWriter, RoleMatch}

// Params enumerates the directive's opaque KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamPred, Kind: shape.KindOpaque},
}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles, Params: Params}
}
