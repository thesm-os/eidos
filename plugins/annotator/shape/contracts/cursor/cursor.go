// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cursor

import (
	"fmt"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
)

// Name is the canonical contract name this package stamps.
const Name = "cursor"

// ParamSentinel is the KV key naming the error Next reports once Close has run.
//
// A sentinel is a package-level var, so the resolver rewrites it
// through the var scope rather than the callable one — see
// [shape.KindVar]. Absence is not an error: the bare
// form still classifies, and a suite that cannot state the law
// without a sentinel declines to state it.
const ParamSentinel = "sentinel"

// ParamNext is the KV key naming the reader on the handle an `open`
// callable answers. Scoped to that role: under `next` and `close` the
// same key is a partner reference to a sibling callable.
const ParamNext = "next"

// ParamClose is the KV key naming the release method on the handle an
// `open` callable answers. Role-scoped for the same reason as
// [ParamNext].
const ParamClose = "close"

// RoleOpen is the producer role — a callable answering a fresh cursor
// rather than being one of its methods.
const RoleOpen = "open"

// Params enumerates the directive's KV keys.
//
// `next` and `close` carry a [shape.Param.Role], so they are member
// references on the producer arm and partner references everywhere
// else. Without the scope the two arms cannot coexist: resolving them
// as partners reports a correct `open` directive as naming callables
// absent from the host's scope, because they live on the handle it
// returns rather than beside it.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamSentinel, Kind: shape.KindVar},
	{Key: ParamNext, Kind: shape.KindMember, Role: RoleOpen},
	{Key: ParamClose, Kind: shape.KindMember, Role: RoleOpen},
}

// Roles enumerates the contract's role vocabulary.
//
// `open` joins the two method roles rather than forming its own
// contract: a produced cursor and a self-declaring one are the same
// protocol reached from different sides, and splitting them would
// make a law selecting "cursor" miss half its corpus.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{"next", "close", RoleOpen}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{
		Name:     Name,
		Roles:    Roles,
		Params:   Params,
		Validate: validate,
	}
}

// validate requires an `open` host to name the handle's reader.
//
// The producer arm needs it where the method arms do not: on `role=next`
// the reader is the host itself, so the contract cannot be stamped
// without one. On `role=open` the host is the factory, and `next=` is
// the only thing identifying what to read from the cursor it answers —
// omit it and the callable still classifies as a cursor producer while
// every law selecting the contract has nothing to call.
//
// `close` stays optional in both directions. It is optional on the
// method arms already, and a caller releasing the handle by other
// means — a deferred close at the call site, a cancelled context —
// has a cursor whose contract is honestly stated without one.
func validate(members map[string][]shape.ContractMember) []shape.ContractViolation {
	var out []shape.ContractViolation
	for _, m := range members[RoleOpen] {
		if m.Params[ParamNext] != "" {
			continue
		}
		out = append(out, shape.ContractViolation{
			Host: m.Host,
			Message: fmt.Sprintf(
				"cursor role %q requires %s=, naming the reader on the handle it answers",
				RoleOpen, ParamNext,
			),
		})
	}
	return out
}
