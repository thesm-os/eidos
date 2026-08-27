// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package watcher

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "watcher"

// ParamNext is the KV key naming the method that reads the next event
// from the handle the watch answers.
//
// A member rather than a sibling: it is declared on the subscription
// Watch returns, not on the interface Watch is declared on, so neither
// the callable scope nor the var scope reaches it.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
const ParamNext = "next"

// ParamStop is the KV key naming the method that ends the
// subscription.
const ParamStop = "stop"

// Params enumerates the KV keys the directive accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamNext, Kind: shape.KindMember},
	{Key: ParamStop, Kind: shape.KindMember},
}

// RoleWatch is the callable delivering change notifications.
const RoleWatch = "watch"

// RoleTrigger is the callable whose effect a [RoleWatch] observes.
const RoleTrigger = "trigger"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleWatch, RoleTrigger}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles, Params: Params}
}
