// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package upserter

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "upserter"

// RoleWriter is the callable that inserts or updates.
const RoleWriter = "writer"

// RoleReader is the callable reading the result back.
const RoleReader = "reader"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleWriter, RoleReader}

// Contract returns the [shape.Contract] this package contributes.
// The writer side requires a reader partner so the test suite can
// confirm the upserted record is observable.
func Contract() shape.Contract {
	return shape.Contract{
		Name:     Name,
		Roles:    Roles,
		Required: map[string][]string{RoleWriter: {RoleReader}},
	}
}
