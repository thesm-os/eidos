// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cas

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "cas"

// ParamMismatch is the KV key naming the error a losing writer reports when the version did not match.
//
// A sentinel is a package-level var, so the resolver rewrites it
// through the var scope rather than the callable one — see
// [shape.KindVar]. Absence is not an error: the bare
// form still classifies, and a suite that cannot state the law
// without a sentinel declines to state it.
const ParamMismatch = "mismatch"

// ParamVersion is the KV key naming the field a compare-and-swap
// compares before it writes.
//
// Opaque: the value names a field on the caller's own type, which
// this package never resolves.
const ParamVersion = "version"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{"writer"}

// Params enumerates the directive's opaque KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamVersion, Kind: shape.KindOpaque},
	{Key: ParamMismatch, Kind: shape.KindVar},
}

// Contract returns the [shape.Contract] this package contributes.
// The `version` KV is an opaque field-name reference — the
// resolver never tries to look it up as a sibling callable.
func Contract() shape.Contract {
	return shape.Contract{
		Name:   Name,
		Roles:  Roles,
		Params: Params,
	}
}
