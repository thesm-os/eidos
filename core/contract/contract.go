// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package contract defines the cross-cutting interfaces both the
// source-side (`node.*`) and emit-side (`emit.*`) graphs implement.
// The package exists because some framework surfaces — most
// notably [emit.Method.Owner] — need to refer to a value from
// either graph without importing the concrete packages (which
// would close the cycle node → emit → node).
//
// Two interfaces live here:
//
//   - [Node] — the minimal contract every kind in either graph
//     satisfies. Discriminator + source position + documentation +
//     directives + metadata.
//   - [Owner] — extends Node with the contract for being a type
//     a method belongs to. Implementors expose their identifier
//     ([Owner.OwnerName]) and qualified name ([Owner.OwnerQName])
//     uniformly, so consumers walking method buckets never type-
//     switch the underlying concrete type.
//
// Both [emit.Node] and [node.Node] embed [Node]; the seven owner-
// eligible types (emit.{Struct, Interface, Alias} +
// node.{Struct, Interface, Enum, Alias}) implement [Owner].
package contract

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
)

// Node is the minimal contract every concrete kind in the
// source-side and emit-side graphs satisfies. The shape mirrors
// the seven-method surface both [node.Node] and [emit.Node]
// declared independently before this package existed — the
// framework's two graph packages each adopt this interface and
// extend it with their per-graph methods (e.g. emit.Node's
// Origin / SetBy back-pointers).
//
// Kind-specific accessors (Methods, Fields, Params, …) live on
// the concrete types and are reached via type assertion or the
// per-graph Walk helpers.
type Node interface {
	// Kind returns the [kind.Kind] discriminator for this node.
	Kind() kind.Kind

	// Pos returns the source position this node reflects. Derived
	// emit values usually inherit from their origin; purely
	// generated values use [position.Synthetic].
	Pos() position.Pos

	// Docs returns the documentation comment lines preserved or
	// freshly authored for this node.
	Docs() []string

	// Directives returns every parsed `+/-gen:` directive
	// attached to this node in source order.
	Directives() []*directive.Directive

	// Directive returns the first directive with the given name,
	// or nil when none is attached.
	Directive(name directive.Name) *directive.Directive

	// HasDirective reports whether at least one directive with
	// the given name is attached.
	HasDirective(name directive.Name) bool

	// Meta returns the metadata bag for this node, or nil when
	// none has been created. It must not allocate: reading is
	// not writing, and the read paths are concurrent. Every
	// read method on [meta.Bag] treats the nil bag as the empty
	// bag, so plugins read through typed [meta.Key] values
	// without a nil check.
	Meta() *meta.Bag

	// EnsureMeta returns the metadata bag for this node,
	// creating one on first call. Plugins take it when they
	// intend to write through a typed [meta.Key], and the store
	// takes it to register its index observer.
	//
	// Separate from [Node.Meta] because the allocation is a
	// write. Folding the two together made every reader a
	// writer, which is a data race on any path that traverses a
	// shared graph concurrently — and both the backend's render
	// pool and the pipeline's parallel NodesOnly generators do.
	EnsureMeta() *meta.Bag
}

// Owner is the contract for any node that can be the conceptual
// "owner type" of a method — both emit-side composite types
// (Struct, Interface, Alias) and source-side declared types
// (Struct, Interface, Enum, Alias) satisfy it.
//
// [emit.Method.Owner] is typed as Owner; the broadened type lets
// a method emitted onto a user-declared source type (an enum's
// `String() string`, a sentinel error's accessors, etc.) reach
// the source-side node directly, without the framework
// type-switching on every consumer.
//
// Consumers query [Owner.OwnerName] / [Owner.OwnerQName] to
// retrieve the identifier or fully-qualified name; the
// per-implementor implementation is typically a one-line
// `return o.Name` / `return o.QName()` adaptor.
type Owner interface {
	Node

	// OwnerName returns the bare identifier of this owner — the
	// `Name` field's value for every kind that has one. Used by
	// plugins building diagnostic messages or stable test
	// assertions over the owning type.
	OwnerName() string

	// OwnerQName returns the qualified name of this owner —
	// typically `Package.Name`. Used when source-vs-emit
	// disambiguation matters (cache replay, manifest
	// attribution) and when constructing an [OwnerRef] for the
	// serialised form.
	OwnerQName() string
}

// OwnerRef is the serialisable form of an [Owner] reference —
// the shape that survives the JSON round-trip the framework's
// cache layer performs over plugin output.
//
// At construction time the framework populates both
// [emit.Method.Owner] (the live pointer) and
// [emit.Method.OwnerRef] (the serialisable shape). Marshalling
// drops the pointer (Owner is `json:"-"`); unmarshalling leaves
// Owner nil. A later RewireOwners pass consults OwnerRef to
// repopulate Owner against the live store, preserving the
// "every method has a meaningful owner" invariant the framework
// relies on.
//
// The framework's existing [kind.Kind] values are namespaced
// (`"node.enum"`, `"emit.struct"`, ...) so Kind alone discrim-
// inates both the source/emit graph and the per-kind bucket —
// no separate graph field needed.
type OwnerRef struct {
	// Kind identifies the per-graph, per-kind bucket the QName
	// resolves against — e.g. `"node.enum"` for a source enum,
	// `"emit.struct"` for an emitted struct.
	Kind kind.Kind `json:"kind"`

	// QName is the qualified name the lookup keys on
	// (typically `Package.Name`).
	QName string `json:"qname"`
}

// IsZero reports whether the ref carries no resolvable
// information — used by RewireOwners to short-circuit methods
// that were constructed without an owner anchor (synthetic
// fixtures, hand-rolled tests).
func (r OwnerRef) IsZero() bool {
	return r.Kind == "" && r.QName == ""
}

// RefOf constructs the serialisable shape from a live [Owner].
// Empty ref when o is nil. Kind comes from the owner's own
// [kind.Kind] discriminator (which already encodes the source/
// emit graph), so the caller doesn't supply it separately.
func RefOf(o Owner) OwnerRef {
	if o == nil {
		return OwnerRef{}
	}
	return OwnerRef{
		Kind:  o.Kind(),
		QName: o.OwnerQName(),
	}
}
