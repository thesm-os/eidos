// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// FieldBuilder configures a [node.Field] within an enclosing
// interface, class or inline object. The field's Owner back-pointer is
// wired by the parent sub-builder before the field is constructed.
type FieldBuilder struct {
	f *node.Field
}

// Node returns the underlying [node.Field].
func (b *FieldBuilder) Node() *node.Field { return b.f }

// Pos overrides the property's source position. A property inherits
// its enclosing declaration's synthetic file, matching how a frontend
// records every member of one declaration at the file it was parsed
// from. Layout derives the output basename from Pos.File whenever a
// plugin routes from a property, so overriding here redirects that
// output. See [ClassBuilder.Pos].
func (b *FieldBuilder) Pos(p position.Pos) *FieldBuilder {
	b.f.SourcePos = p
	return b
}

// Docs appends doc-comment lines preserved verbatim — the textual
// content of a JSDoc block without its `/**` markers, matching what a
// frontend records.
func (b *FieldBuilder) Docs(lines ...string) *FieldBuilder {
	b.f.DocLines = append(b.f.DocLines, lines...)
	return b
}

// Directive attaches d to the property's directive list.
func (b *FieldBuilder) Directive(d *directive.Directive) *FieldBuilder {
	b.f.DirectiveList = append(b.f.DirectiveList, d)
	return b
}

// Optional marks the property `?`.
//
// A different claim from a type union with `undefined`: an optional
// property may be absent from an object entirely, where an
// undefined-valued one must be present holding undefined. The
// distinction is what exactOptionalPropertyTypes enforces, and a
// fixture that could not spell it could not reproduce the bug where a
// generator conflates the two.
func (b *FieldBuilder) Optional() *FieldBuilder {
	return b.mark(typescript.MetaOptional)
}

// Readonly marks the property `readonly`.
func (b *FieldBuilder) Readonly() *FieldBuilder {
	return b.mark(typescript.MetaReadonly)
}

// Static marks the property `static` — a class member rather than an
// instance one.
func (b *FieldBuilder) Static() *FieldBuilder {
	return b.mark(typescript.MetaStatic)
}

// DefiniteAssignment marks the property `!` — `name!: string`, the
// author's assertion that something else assigns it.
func (b *FieldBuilder) DefiniteAssignment() *FieldBuilder {
	return b.mark(typescript.MetaDefiniteAssignment)
}

// Visibility records an explicit access modifier — one of
// [typescript.VisibilityPublic], [typescript.VisibilityProtected],
// [typescript.VisibilityPrivate] or [typescript.VisibilityHard].
//
// Stamped only where the source wrote one. A member with no modifier
// is public and carries no stamp, which is what lets a consumer tell
// "declared public" from "said nothing".
func (b *FieldBuilder) Visibility(level string) *FieldBuilder {
	typescript.MetaVisibility.Set(b.f.EnsureMeta(), level, markerAuthority)
	return b
}

// Initialiser records the property's initialiser in verbatim source
// form — the `'anon'` in `name: string = 'anon'`.
func (b *FieldBuilder) Initialiser(text string) *FieldBuilder {
	typescript.MetaInitialiser.Set(b.f.EnsureMeta(), text, markerAuthority)
	return b
}

// Decorator appends a decorator to the property, in source order.
//
// Order and repetition both carry meaning — a framework applies them
// bottom-up and a name may appear twice — so they accumulate into one
// ordered list rather than a key per name.
func (b *FieldBuilder) Decorator(name string, args ...string) *FieldBuilder {
	appendDecorator(b.f, name, args)
	return b
}

// mark sets a bool-valued key on the property.
func (b *FieldBuilder) mark(key boolKey) *FieldBuilder {
	key.Set(b.f.EnsureMeta(), true, markerAuthority)
	return b
}
