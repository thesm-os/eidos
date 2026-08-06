// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
)

// BaseNode supplies the fields and methods shared by every concrete
// node kind. Each concrete type embeds BaseNode by value and overrides
// [Kind] to return its [kind.Kind] constant.
//
// All struct fields are exported so frontends and tests can populate
// them via struct literals. Outside the frontend/annotator phases the
// node graph is treated as frozen — see the spec mutability contract.
//
// The MetaBag field is allocated lazily on first call to [BaseNode.Meta]
// so struct-literal construction in tests works without an explicit
// constructor invocation.
type BaseNode struct {
	// SourcePos is where the node was declared. Synthetic nodes use
	// [position.Synthetic] tags.
	SourcePos position.Pos `json:"pos,omitzero"`
	// DocLines holds doc comment text preserved verbatim from source
	// (comment markers stripped). Frontend populates; downstream
	// reads via [BaseNode.Docs].
	DocLines []string `json:"docs,omitempty"`
	// DirectiveList holds every +/-gen: directive attached to this
	// node, in source order.
	DirectiveList []*directive.Directive `json:"directives,omitempty"`
	// MetaBag is the lazily-allocated metadata storage. Access via
	// [BaseNode.Meta] rather than touching the field directly.
	MetaBag *meta.Bag `json:"meta,omitempty"`
}

// Pos returns [BaseNode.SourcePos].
func (b *BaseNode) Pos() position.Pos { return b.SourcePos }

// Docs returns [BaseNode.DocLines]. The returned slice aliases
// internal storage; callers must not mutate it.
func (b *BaseNode) Docs() []string { return b.DocLines }

// Directives returns [BaseNode.DirectiveList]. The returned slice
// aliases internal storage; callers must not mutate it.
func (b *BaseNode) Directives() []*directive.Directive { return b.DirectiveList }

// Directive returns the first directive whose [directive.Name] matches
// name, or nil when no such directive is attached.
func (b *BaseNode) Directive(name directive.Name) *directive.Directive {
	for _, d := range b.DirectiveList {
		if d.Name == name {
			return d
		}
	}
	return nil
}

// HasDirective reports whether at least one directive with the given
// name is attached.
func (b *BaseNode) HasDirective(name directive.Name) bool {
	return b.Directive(name) != nil
}

// HasPositiveDirective reports whether any directive named name is
// attached with [directive.Directive.Negated] false — the
// `+gen:NAME` form. Useful for opt-in gating.
func (b *BaseNode) HasPositiveDirective(name directive.Name) bool {
	return directive.HasPositive(b.DirectiveList, name)
}

// HasNegatedDirective reports whether any directive named name is
// attached with [directive.Directive.Negated] true — the
// `-gen:NAME` form. Useful for opt-out gating.
func (b *BaseNode) HasNegatedDirective(name directive.Name) bool {
	return directive.HasNegated(b.DirectiveList, name)
}

// Meta returns the metadata bag for this node, or nil when none has
// been created. It does not allocate, and every read method on
// [meta.Bag] treats the nil bag as the empty bag — so
// `n.Meta().Has(k)` is correct on a node nothing has stamped.
//
// The read path here is concurrent by contract, not by accident:
// the pipeline runs a bucket of [plugin.NodesOnly] generators in
// parallel over one shared node graph, and reading the metadata an
// annotator stamped is precisely what those generators do. An
// accessor that allocated on first access made every such read a
// write, racing on the creation of the very lock meant to make the
// bag safe.
//
// Writers call [BaseNode.EnsureMeta]. Mirrors
// [go.thesmos.sh/eidos/emit.BaseEmit.Meta] deliberately: the two
// accessors are the same contract on either side of the pipeline.
func (b *BaseNode) Meta() *meta.Bag {
	return b.MetaBag
}

// EnsureMeta returns the metadata bag for this node, creating one on
// first call. The allocation is one-shot per node.
//
// This is the write-side accessor: annotators and frontend stamping
// take it, and both run in sequential phases. It is not safe to call
// concurrently on the same node.
func (b *BaseNode) EnsureMeta() *meta.Bag {
	if b.MetaBag == nil {
		b.MetaBag = meta.NewBag()
	}
	return b.MetaBag
}
