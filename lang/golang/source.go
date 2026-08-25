// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import "go.thesmos.sh/eidos/node"

// Source answers how a declaration written in Go is read.
//
// The read-side half of what a plugin declares for Go, satisfying the
// SDK's source-rules contract. Every method forwards to the function
// beside it in this package, which is the point: the two notations a
// directive value may be written in, the literal well-formedness
// rule, the file-scoped qualifier lookup and the tag vocabulary are
// Go's own rules and already live here. A plugin holding private
// copies is the same rules written twice, disagreeing the first time
// either is corrected.
//
// The zero value is usable and carries no state, so a declaration is
// `Source{}` and costs nothing to hand out.
//
// # Why this package does not import the SDK
//
// The methods take [node] types, and the façade's source-model names
// are aliases of exactly those — so this satisfies the SDK interface
// structurally, without this package depending on it. That keeps the
// boundary the package documentation describes: `lang/golang` sits
// over [node], [emit] and `core`, below every consumer. The
// compile-time confirmation lives in `lang/golang/sdk`, which imports
// both and is where a plugin picks the declaration up.
type Source struct{}

// ResolveValue splits a value written in a directive or a tag into the
// package it names and the symbol within it.
//
// Both notations an author writes are accepted — a qualifier resolved
// against the file's own import block, and a full import path for a
// package imported only for the directive, which is otherwise an
// unused import and does not compile. See [ResolveValue].
func (Source) ResolveValue(f *node.File, value string) (pkg, symbol string, err error) {
	return ResolveValue(f, value)
}

// Tag returns the named struct-tag entry on f.
//
// Answered as the union of the `go.tag.*` stamp and the field's own
// tag string — see [Tag] — so a graph no Go frontend produced still
// reports the tags its declarations carry.
func (Source) Tag(f *node.Field, key string) (string, bool) {
	return Tag(f, key)
}

// FileOf returns the file within pkg that declared n.
func (Source) FileOf(pkg *node.Package, n node.Node) *node.File {
	return FileOf(pkg, n)
}
