// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tx

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "tx"

// ParamClosed is the KV key naming the error a commit or rollback reports once the transaction is finished.
//
// A sentinel is a package-level var, so the resolver rewrites it
// through the var scope rather than the callable one — see
// [shape.KindVar]. Absence is not an error: the bare
// form still classifies, and a suite that cannot state the law
// without a sentinel declines to state it.
const ParamClosed = "closed"

// Params enumerates the directive's opaque KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamClosed, Kind: shape.KindVar},
}

// RoleBegin is the callable opening the transaction.
const RoleBegin = "begin"

// RoleCommit is the callable making its writes durable.
const RoleCommit = "commit"

// RoleRollback is the callable discarding them.
const RoleRollback = "rollback"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleBegin, RoleCommit, RoleRollback}

// Contract returns the [shape.Contract] this package contributes.
// Begin requires both Commit and Rollback partners — the
// validator flags any Begin declaration missing either side.
func Contract() shape.Contract {
	return shape.Contract{
		Name:     Name,
		Roles:    Roles,
		Params:   Params,
		Required: map[string][]string{RoleBegin: {RoleCommit, RoleRollback}},
	}
}
