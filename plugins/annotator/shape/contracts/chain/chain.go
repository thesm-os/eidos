// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package chain

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "chain"

// RoleAppend is the callable adding an entry to the log.
const RoleAppend = "append"

// RoleReplay is the callable reading the history back.
const RoleReplay = "replay"

// RoleVerify is the callable checking the log for tampering.
//
// Optional, unlike [RoleReplay]: an implementation reporting
// corruption through a poison accessor's error surface is checkable
// without one, and requiring it would rule out the commoner spelling.
const RoleVerify = "verify"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleAppend, RoleReplay, RoleVerify}

// Required pins the replay to the append.
//
// Append-only is a claim about history, so it cannot be checked
// without a way to read history: an append naming no replay leaves
// every one of this contract's properties unobservable.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Required = map[string][]string{RoleAppend: {RoleReplay}}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles, Required: Required}
}
