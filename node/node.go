// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

import (
	"go.thesmos.sh/eidos/core/contract"
)

// Node is the common interface every concrete source-side node
// kind satisfies. The shape — a kind discriminator, source
// position, documentation, directives, and a metadata bag — is
// identical to [contract.Node]; Node is exposed as a type alias
// so existing callers continue to spell `node.Node` while
// cross-graph framework components (the routing layer, the
// owner-resolve pass, plugin author cookbooks) can speak
// `contract.Node` and accept either source-side or emit-side
// values.
//
// Kind-specific accessors (Methods, Fields, Params, …) live on
// the concrete types and are reached via type assertion or
// [Walk].
type Node = contract.Node

// Resolver answers what a named type is, over the declarations a run
// loaded.
//
// A [TypeRef] records a package and an identifier and nothing about
// what they resolve to, so a classifier cannot tell from `Weekday`
// alone that it is an integer. A caller holding the graph can, and
// this is the narrow interface that lets one — satisfied by a
// store-backed index without this package depending on the store.
//
// Declared here rather than in a language package because more than
// one names it: a language's own helpers take it, and the plugin
// façade puts it in a contract those helpers satisfy. Two named
// interfaces with identical methods are still two types, and a method
// declared against one does not implement a contract written against
// the other — so both alias this.
type Resolver interface {
	// Resolve returns the declaration a type reference names, and
	// whether the run loaded one. A type from a package the run never
	// read reports false, which is a smaller answer rather than a
	// wrong one.
	Resolve(t *TypeRef) (Node, bool)
}
