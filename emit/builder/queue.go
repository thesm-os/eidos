// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder

import (
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

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
