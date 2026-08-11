// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package transaction

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "transaction"

// ParamNotFound is the KV key naming the error a read reports for work the rollback undid.
//
// A sentinel is a package-level var, so the resolver rewrites it
// through the var scope rather than the callable one — see
// [shape.Contract.SiblingVars]. Absence is not an error: the bare
// form still classifies, and a suite that cannot state the law
// without a sentinel declines to state it.
const ParamNotFound = "notfound"

// Params enumerates the directive's opaque KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []string{ParamNotFound}

// SiblingVars enumerates the param keys whose values name
// package-level vars the resolver rewrites into qualified names.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var SiblingVars = []string{ParamNotFound}

// Roles enumerates the contract's role vocabulary — a single
// "fn" role since the contract is a per-callable marker.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{"fn"}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{
		Name:        Name,
		Roles:       Roles,
		Params:      Params,
		SiblingVars: SiblingVars,
	}
}
