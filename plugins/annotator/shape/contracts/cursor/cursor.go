// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cursor

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "cursor"

// ParamSentinel is the KV key naming the error Next reports once Close has run.
//
// A sentinel is a package-level var, so the resolver rewrites it
// through the var scope rather than the callable one — see
// [shape.Contract.SiblingVars]. Absence is not an error: the bare
// form still classifies, and a suite that cannot state the law
// without a sentinel declines to state it.
const ParamSentinel = "sentinel"

// Params enumerates the directive's opaque KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []string{ParamSentinel}

// SiblingVars enumerates the param keys whose values name
// package-level vars the resolver rewrites into qualified names.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var SiblingVars = []string{ParamSentinel}

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{"next", "close"}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{
		Name:        Name,
		Roles:       Roles,
		Params:      Params,
		SiblingVars: SiblingVars,
	}
}
