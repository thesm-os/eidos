// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// BaseEmit supplies the fields and methods every concrete emit type
// shares. Each concrete type embeds BaseEmit by value and overrides
// [Kind] to return its [kind.Kind] constant.
//
// All struct fields are exported so generators and tests can populate
// them via struct literals. The MetaBag field is allocated lazily on
// first call to [BaseEmit.Meta] so struct-literal construction works
// without an explicit constructor invocation.
//
// During the generator phase the emit tree is mutable; once the
// constructing generator returns, downstream consumers (later
// generators reading prior emit and the backend) treat the tree as
// frozen — see the spec mutability contract.
type BaseEmit struct {
	// SourcePos is the source position this emit value reflects.
	// Frontends synthesising emit purely from plugin logic should
	// use [position.Synthetic] tags.
	SourcePos position.Pos `json:"pos,omitzero"`

	// DocLines holds doc comment text — either preserved from the
	// originating source node or freshly authored by the generator.
	DocLines []string `json:"docs,omitempty"`

	// DirectiveList holds the directives attached to this emit
	// value (typically copied verbatim from the originating source
	// node so backend rendering can re-read them).
	DirectiveList []*directive.Directive `json:"directives,omitempty"`

	// MetaBag is the lazily-allocated metadata storage. Access via
	// [BaseEmit.Meta] rather than touching the field directly.
	MetaBag *meta.Bag `json:"meta,omitempty"`

	// OriginNode is the source [node.Node] this emit value was
	// derived from. nil for purely-generated artifacts.
	OriginNode node.Node `json:"-"`

	// SetByName is the plugin identifier that produced this emit
	// value. Stamped at construction time by the builder constructors
	// (e.g. [builder.PackageBuilder.Struct]) from the originating
	// [builder.Context]'s SetBy value. The backend reads it to
	// compose the per-file `Plugins:` header from only the plugins
	// that actually contributed entities to the target; the
	// pipeline's manifest sink reads it for the same per-target
	// plugin attribution.
	//
	// Empty for entities constructed without a builder context
	// (hand-rolled test fixtures, synthetic entities the framework
	// stitches together internally) — callers treat the empty
	// string as "unattributed" and drop it from any plugin set.
	// Access via [BaseEmit.SetBy].
	SetByName string `json:"set_by,omitempty"`

	// OutputTagName identifies the [go.thesmos.sh/eidos/plugin.Output]
	// this decl belongs to within its owning plugin's namespace.
	// Empty means the plugin's primary output; non-empty values
	// must match one of the values
	// [go.thesmos.sh/eidos/plugin.FilenameProvider.Outputs] returns
	// for the active backend language. The Layout phase reads this
	// field to resolve a per-decl filename suffix against the
	// owning plugin's declared output set.
	//
	// Direct field assignment is not the supported authoring API.
	// Plugins stamp the tag indirectly through the sub-context
	// `PackageBuilder.File(tag)` returns; reading the value is
	// always safe for downstream consumers (later generators,
	// weavers iterating decls, backends rendering output) — access
	// via [BaseEmit.OutputTag].
	OutputTagName string `json:"output_tag,omitempty"`
}

// Pos returns [BaseEmit.SourcePos].
func (b *BaseEmit) Pos() position.Pos { return b.SourcePos }

// Docs returns [BaseEmit.DocLines]. The returned slice aliases
// internal storage; callers must not mutate it.
func (b *BaseEmit) Docs() []string { return b.DocLines }

// Directives returns [BaseEmit.DirectiveList]. The returned slice
// aliases internal storage; callers must not mutate it.
func (b *BaseEmit) Directives() []*directive.Directive { return b.DirectiveList }

// Directive returns the first directive whose [directive.Name]
// matches name, or nil when no such directive is attached.
func (b *BaseEmit) Directive(name directive.Name) *directive.Directive {
	for _, d := range b.DirectiveList {
		if d.Name == name {
			return d
		}
	}
	return nil
}

// HasDirective reports whether at least one directive with the given
// name is attached.
func (b *BaseEmit) HasDirective(name directive.Name) bool {
	return b.Directive(name) != nil
}

// HasPositiveDirective reports whether any directive named name is
// attached with [directive.Directive.Negated] false — the
// `+gen:NAME` form. Useful for opt-in gating.
func (b *BaseEmit) HasPositiveDirective(name directive.Name) bool {
	return directive.HasPositive(b.DirectiveList, name)
}

// HasNegatedDirective reports whether any directive named name is
// attached with [directive.Directive.Negated] true — the
// `-gen:NAME` form. Useful for opt-out gating.
func (b *BaseEmit) HasNegatedDirective(name directive.Name) bool {
	return directive.HasNegated(b.DirectiveList, name)
}

// Meta returns the metadata bag for this emit value, or nil when
// none has been created. It does not allocate, and every read method
// on [meta.Bag] treats the nil bag as the empty bag — so
// `n.Meta().Has(k)` is correct on a value nothing has written to.
//
// Reading must not write. The accessor used to allocate a bag on
// first access, which made three consequences follow from a call
// whose published purpose was to answer a question: two allocations
// retained for the life of the emit tree per node anything ever
// asked about; a change to the value's serialised form, because
// `json:"meta,omitempty"` does not omit a non-nil pointer to an
// empty bag; and a data race. The bag's own lock cannot defend
// against the last one — two goroutines reaching a nil MetaBag each
// build a bag and each proceed through a mutex the other's bag does
// not share, so the lock is the object being raced on. Both
// [Backend.Render]'s worker pool and the pipeline's parallel
// NodesOnly generators traverse a shared graph.
//
// Writers call [BaseEmit.EnsureMeta].
func (b *BaseEmit) Meta() *meta.Bag {
	return b.MetaBag
}

// EnsureMeta returns the metadata bag for this emit value, creating
// one on first call. The allocation is one-shot per value.
//
// This is the write-side accessor: take it when you intend to Set,
// Tombstone, or AddObserver. It is not safe to call concurrently on
// the same value — the creation it performs is the write [Meta]
// exists to avoid — which is not a constraint in practice, because
// every writer runs in a sequential phase (frontend stamping, store
// indexing) while the concurrent phases only read.
func (b *BaseEmit) EnsureMeta() *meta.Bag {
	if b.MetaBag == nil {
		b.MetaBag = meta.NewBag()
	}
	return b.MetaBag
}

// Origin returns [BaseEmit.OriginNode] — the source node this emit
// value was derived from, or nil for purely-generated artifacts.
func (b *BaseEmit) Origin() node.Node { return b.OriginNode }

// SetBy returns [BaseEmit.SetByName] — the plugin identifier that
// produced this emit value, or the empty string for entities
// constructed without a builder context.
func (b *BaseEmit) SetBy() string { return b.SetByName }

// OutputTag returns [BaseEmit.OutputTagName] — the
// [plugin.Output.Tag] this emit value belongs to within its
// owning plugin's namespace, or the empty string for the
// plugin's primary output. Downstream consumers pair this with
// [SetBy] to render `<plugin>:<tag>` for human-readable surfaces.
func (b *BaseEmit) OutputTag() string { return b.OutputTagName }
