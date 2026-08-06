// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package storefixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// EnumBuilder configures a [node.Enum] within a [Builder]'s
// accumulating package. Variants are appended in declaration order;
// each variant's [node.EnumVariant.Owner] back-pointer is wired
// automatically.
type EnumBuilder struct {
	e *node.Enum
}

// Node returns the underlying [node.Enum].
func (b *EnumBuilder) Node() *node.Enum { return b.e }

// Pos overrides the enum's source position. Layout derives the
// basename of any file generated from this node from Pos.File, so
// the value decides the output filename; the fixture's synthetic
// `<pkg>/<lowercased-name>.go` keeps that basename non-empty. See
// [StructBuilder.Pos] for what an empty one costs.
func (b *EnumBuilder) Pos(p position.Pos) *EnumBuilder {
	b.e.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *EnumBuilder) Docs(lines ...string) *EnumBuilder {
	b.e.DocLines = append(b.e.DocLines, lines...)
	return b
}

// Directive attaches d to the enum's directive list.
func (b *EnumBuilder) Directive(d *directive.Directive) *EnumBuilder {
	b.e.DirectiveList = append(b.e.DirectiveList, d)
	return b
}

// Underlying records the enum's underlying type. Leave unset for
// typeless enums; downstream consumers can detect the absence and
// fall back to a default type.
func (b *EnumBuilder) Underlying(t *node.TypeRef) *EnumBuilder {
	b.e.Underlying = t
	return b
}

// Variant appends a variant. The variant's Owner back-pointer is
// wired automatically. Pass an empty value when the variant has no
// declared value (e.g., languages where variants are unit-only).
func (b *EnumBuilder) Variant(name, value string) *EnumBuilder {
	b.e.Variants = append(b.e.Variants, &node.EnumVariant{
		Name:  name,
		Value: value,
		Owner: b.e,
	})
	return b
}
