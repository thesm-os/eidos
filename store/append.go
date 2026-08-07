// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store

import (
	"fmt"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// AppendOrigin queues every value against origin's named slot,
// stamping each with provenance identifying the plugin and the
// value's kind.
//
// Every generator ends the same way: build one value per output,
// stamp it, and queue it against its origin so Layout can route it.
// Written out per plugin the steps are identical, and the copies
// drift — one acquires a different provenance shape, or forgets the
// id, and nothing distinguishes it from the others until a later
// plugin cannot find the anchor it meant to position against.
//
// The provenance id is `<kind>.<origin-name>`, which is what a
// later plugin targets when positioning its own contribution
// relative to this one, and what a reader chasing "which plugin
// wrote this line" gets back from `explain`.
//
// Variadic because a generator's outputs are a set that grows:
// appending them in one call is what keeps a primary and its
// companion from acquiring separate, divergent copies of this
// logic. A nil value is skipped rather than rejected, so a caller
// assembling a slice with conditional entries need not compact it
// first.
func (v *EmitView) AppendOrigin(
	setBy, slotName string,
	origin node.Node,
	values ...emit.Node,
) error {
	return v.appendOrigin(setBy, slotName, originName(origin), origin, values...)
}

// AppendOriginAs is [EmitView.AppendOrigin] for a value whose
// provenance names something other than the declaration it hangs
// off.
//
// Package-scoped output has no declaration of its own, so it is
// anchored on one the package happens to contain and identified by
// the package it is really about. Deriving the id from the anchor
// there would make the identifier a later plugin targets depend on
// which declaration sorted first, and renaming an unrelated type in
// the package would move it.
func (v *EmitView) AppendOriginAs(
	setBy, slotName, id string,
	origin node.Node,
	values ...emit.Node,
) error {
	return v.appendOrigin(setBy, slotName, id, origin, values...)
}

// appendOrigin queues each value under `<kind>.<id>` provenance.
func (v *EmitView) appendOrigin(
	setBy, slotName, id string,
	origin node.Node,
	values ...emit.Node,
) error {
	for _, value := range values {
		if value == nil {
			continue
		}
		prov := emit.Provenance{
			SetBy: setBy,
			ID:    fmt.Sprintf("%s.%s", value.Kind(), id),
		}
		if err := v.AppendOriginSlot(origin, slotName, value, prov); err != nil {
			return fmt.Errorf("store: queue %s on slot %q: %w", value.Kind(), slotName, err)
		}
	}
	return nil
}

// originName returns the identifier a provenance id derives from.
//
// Falls back to the kind when the node carries no name, so the id
// stays non-empty: a bare `<kind>.` anchor is one no later plugin
// can target unambiguously once a second nameless origin appears.
func originName(origin node.Node) string {
	if origin == nil {
		return ""
	}
	type named interface{ OwnerName() string }
	if n, ok := origin.(named); ok {
		if name := n.OwnerName(); name != "" {
			return name
		}
	}
	return string(origin.Kind())
}
