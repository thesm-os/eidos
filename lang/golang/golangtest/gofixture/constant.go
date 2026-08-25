// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// ConstantBuilder configures a [node.Constant] within a [Builder]'s
// accumulating package. Constants that participate in an idiomatic
// enum group are emitted via [Builder.Enum] instead.
type ConstantBuilder struct {
	c *node.Constant
}

// Node returns the underlying [node.Constant].
func (b *ConstantBuilder) Node() *node.Constant { return b.c }

// Pos overrides the constant's source position. Layout derives the
// basename of any file generated from this node from Pos.File, so
// the value decides the output filename; the fixture's synthetic
// `<pkg>/<lowercased-name>.go` keeps that basename non-empty. See
// [StructBuilder.Pos] for what an empty one costs.
func (b *ConstantBuilder) Pos(p position.Pos) *ConstantBuilder {
	b.c.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *ConstantBuilder) Docs(lines ...string) *ConstantBuilder {
	b.c.DocLines = append(b.c.DocLines, lines...)
	return b
}

// Directive attaches d to the constant's directive list.
func (b *ConstantBuilder) Directive(d *directive.Directive) *ConstantBuilder {
	b.c.DirectiveList = append(b.c.DirectiveList, d)
	return b
}

// Type records the declared type.
func (b *ConstantBuilder) Type(t *node.TypeRef) *ConstantBuilder {
	b.c.Type = t
	return b
}

// Value records the verbatim source form of the constant's value.
func (b *ConstantBuilder) Value(v string) *ConstantBuilder {
	b.c.Value = v
	return b
}
