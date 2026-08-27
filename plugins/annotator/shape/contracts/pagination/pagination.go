// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pagination

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "pagination"

// RoleReader is the callable answering one page.
const RoleReader = "reader"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleReader}

// ParamCursor is the KV key naming the field carrying the position a
// page resumes from.
//
// Opaque: the value names a field on the caller's own page type,
// which this package never resolves.
const ParamCursor = "cursor"

// Params enumerates the directive's opaque KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamCursor, Kind: shape.KindOpaque},
}

// Contract returns the [shape.Contract] this package contributes.
// The `cursor` KV is an opaque field-name reference.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles, Params: Params}
}
