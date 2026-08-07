// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder

import (
	"errors"
	"fmt"

	"go.thesmos.sh/eidos/core/contract"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// ErrNilOrigin reports a queue call with no origin declaration.
//
// A queued value is routed by the node it was projected from, so a
// nil origin has no slot to land in and no position for a later
// diagnostic to point at. Returned rather than panicked: it is
// reachable from a generator that filtered its work list wrongly,
// and one bad declaration should fail its own emit rather than the
// run.
var ErrNilOrigin = errors.New("builder: nil origin")

// Base builds the emit base for a value projected from origin.
//
// The three fields travel together — the node the value came from,
// the plugin that made it, and the position a diagnostic about it
// should point at — and a value missing any of them is one whose
// failures name the wrong source line, or no line at all.
//
// Every generator ends the same way: build one value per output,
// stamp it, and queue it against its origin so Layout can route it.
// Written out per plugin, the steps are identical and the copies
// drift — one acquires a different field order, or forgets the
// position, and nothing distinguishes it from the others until a
// diagnostic points somewhere wrong.
func Base(c *Context, origin node.Node) emit.BaseEmit {
	if origin == nil {
		return emit.BaseEmit{SetByName: c.SetBy()}
	}
	return emit.BaseEmit{
		OriginNode: origin,
		SetByName:  c.SetBy(),
		SourcePos:  origin.Pos(),
	}
}

// Tagged returns base routed to the named output tag.
//
// Returned by value rather than mutated in place: a plugin building
// a primary output and a companion holds one base and derives the
// second from it, and a helper that mutated would leave the first
// pointing at the companion's output.
func Tagged(base emit.BaseEmit, tag string) emit.BaseEmit {
	base.OutputTagName = tag
	return base
}

// Appender is the emit-slot writer a queue writes through.
//
// A port rather than a concrete store handle, so this package stays
// a leaf over the emit model: the store already imports emit, and
// taking its view type here would bind every builder to one storage
// implementation for the sake of a single method. The store's emit
// view satisfies it structurally, so neither side carries an
// adapter.
type Appender interface {
	// AppendOriginSlot appends item to the named cross-cutting slot
	// on origin, stamped with prov.
	AppendOriginSlot(origin node.Node, slotName string, item emit.Node, prov emit.Provenance) error
}

// Queue appends every value to origin's named slot, stamping each
// with provenance naming its kind and the declaration it came from.
//
// The provenance id is `<kind>.<origin>`, which is what a later
// plugin targets when it positions its own contribution relative to
// this one, and what a reader chasing "which plugin wrote this line"
// gets back.
//
// Variadic because a plugin's outputs are a set that grows:
// appending them in one call is what keeps a primary and its
// companion from acquiring separate, divergent copies of this logic.
//
// A nil value is skipped rather than queued — a projection that
// declined to build one output should not abort the others — but a
// nil origin is [ErrNilOrigin], because there is nowhere to put any
// of them.
func Queue(a Appender, c *Context, slot string, origin contract.Owner, values ...emit.Node) error {
	if origin == nil {
		return fmt.Errorf("%s: append %q slot: %w", c.SetBy(), slot, ErrNilOrigin)
	}
	return QueueAs(a, c, slot, origin, origin.OwnerName(), values...)
}

// QueueAs is [Queue] for a value whose provenance names something
// other than the declaration it hangs off.
//
// Package-scoped output has no declaration of its own, so it is
// anchored on one the package happens to contain and identified by
// the package it is really about. Deriving the id from the anchor
// there would make the identifier a plugin targets depend on which
// declaration sorted first, so renaming an unrelated type would move
// it.
func QueueAs(
	a Appender,
	c *Context,
	slot string,
	origin node.Node,
	id string,
	values ...emit.Node,
) error {
	if origin == nil {
		return fmt.Errorf("%s: append %q slot for %q: %w", c.SetBy(), slot, id, ErrNilOrigin)
	}
	for _, value := range values {
		if value == nil {
			continue
		}
		prov := c.Provenance(string(value.Kind()) + "." + id)
		if err := a.AppendOriginSlot(origin, slot, value, prov); err != nil {
			return fmt.Errorf("%s: append %s slot for %q: %w", c.SetBy(), value.Kind(), id, err)
		}
	}
	return nil
}
