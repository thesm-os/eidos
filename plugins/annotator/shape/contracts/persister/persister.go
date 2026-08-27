// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package persister

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
)

// Name is the canonical contract name this package stamps.
// Consumers iterating [shape.Contracts] compare against this
// constant rather than the literal string so renames surface as
// compile errors.
const Name = "persister"

// RoleWriter is the callable storing a value.
const RoleWriter = "writer"

// RoleReader is the callable reading it back — the partner that makes
// the writer's effect observable.
const RoleReader = "reader"

// Roles enumerates the contract's role vocabulary. Exported so
// refinement-bucket resolvers and validators can read the
// canonical role list without importing the [shape.Contract]
// value.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleWriter, RoleReader}

// Contract returns the [shape.Contract] this package contributes
// to the umbrella shape plugin. Register one instance per
// pipeline:
//
//	pipe.WithAnnotator(shape.New().Contracts(persister.Contract()))
func Contract() shape.Contract {
	return shape.Contract{
		Name:  Name,
		Roles: Roles,
	}
}
