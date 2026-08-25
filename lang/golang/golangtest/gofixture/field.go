// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// FieldBuilder configures a [node.Field] within an enclosing struct.
// The field's [node.Field.Owner] back-pointer is wired by the parent
// [StructBuilder] before the field is constructed.
type FieldBuilder struct {
	f *node.Field
}

// Node returns the underlying [node.Field].
func (b *FieldBuilder) Node() *node.Field { return b.f }

// Pos overrides the field's source position. The field inherits
// its enclosing declaration's synthetic file, matching how a
// frontend records every member of one declaration at the file it
// was parsed from. Layout derives the output basename from Pos.File
// whenever a plugin routes from a field, so overriding here
// redirects that output. See [StructBuilder.Pos].
func (b *FieldBuilder) Pos(p position.Pos) *FieldBuilder {
	b.f.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *FieldBuilder) Docs(lines ...string) *FieldBuilder {
	b.f.DocLines = append(b.f.DocLines, lines...)
	return b
}

// Tag sets the field's struct-tag string verbatim (e.g.,
// "`json:\"id\"`"). Frontends preserve tags exactly as written.
func (b *FieldBuilder) Tag(tag string) *FieldBuilder {
	b.f.Tag = tag
	return b
}

// Directive attaches d to the field's directive list.
func (b *FieldBuilder) Directive(d *directive.Directive) *FieldBuilder {
	b.f.DirectiveList = append(b.f.DirectiveList, d)
	return b
}
