// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package updater

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "updater"

// RoleWriter is the callable updating an existing value.
const RoleWriter = "writer"

// RoleReader is the callable reading it back.
const RoleReader = "reader"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleWriter, RoleReader}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles}
}
