// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package outbox

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "outbox"

// RoleAppend is the callable enqueuing a message.
const RoleAppend = "append"

// RoleSubscribe is the callable draining what was enqueued.
const RoleSubscribe = "subscribe"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleAppend, RoleSubscribe}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles}
}
