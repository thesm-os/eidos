// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package publisher

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "publisher"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{"publish", "subscribe", RoleRedeliver}

// ParamMode declares the delivery guarantee the publisher makes.
//
// Opaque to the resolver: the value names neither a callable nor a
// parameter, only which bound a per-subscriber delivery count is
// checked against.
//
// Absent means unstated rather than a default. The three modes imply
// different assertions — duplicates permitted, loss permitted, or
// neither — and picking one for an implementation that did not say
// produces a check that fails on correct code.
const ParamMode = "mode"

// RoleRedeliver is the callable that re-sends an already-published
// message.
//
// The delivery bound is only checkable across one: at-least-once and
// exactly-once differ in what a subscriber sees after a redelivery,
// and a suite that never redelivers cannot tell them apart.
const RoleRedeliver = "redeliver"

// The delivery guarantees [ParamMode] accepts. Each subscriber
// receives a published message one or more times, zero or one times,
// or exactly once across a redelivery.
const (
	ModeAtLeastOnce = "at-least-once"
	ModeAtMostOnce  = "at-most-once"
	ModeExactlyOnce = "exactly-once"
)

// Params enumerates the directive's opaque KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []string{ParamMode}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles, Params: Params}
}
