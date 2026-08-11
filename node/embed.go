// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

import (
	"go.thesmos.sh/eidos/core/kind"
)

// Embed is one embedded type in a [Struct] or [Interface]. Go's
// struct embedding and interface composition both surface as Embed
// nodes; downstream consumers use Owner to discriminate.
type Embed struct {
	BaseNode

	// Type is the embedded type reference.
	Type *TypeRef `json:"type,omitempty"`

	// Owner is the struct or interface doing the embedding.
	// Populated by the constructing frontend.
	//
	// Owner is excluded from JSON encoding to break the host →
	// child cycle. Deserialized graphs re-wire Owner via
	// [RewireOwners].
	Owner Node `json:"-"`

	// Resolved is the embedded interface projected from the
	// type-checker, for an embed whose declaration this run did not
	// load — a standard-library interface, or one from any package
	// outside the run's patterns.
	//
	// Nil for a struct embed and for an interface embed the walk can
	// answer from a loaded declaration, which always wins:
	// [MethodSet] consults its resolver first and reads this only on a
	// miss. A loaded declaration carries directives, doc comments and
	// positions that a signature projection cannot.
	//
	// Not indexed in any store. It exists to complete a method-set
	// walk, not to add a declaration nobody's patterns asked for, so
	// it does not appear in [store.NodeView.Interfaces] and a
	// whole-graph walk does not see it.
	Resolved *Interface `json:"resolved,omitempty"`
}

// Kind returns [KindEmbed].
func (*Embed) Kind() kind.Kind { return KindEmbed }
