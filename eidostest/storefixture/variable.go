// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package storefixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// VariableBuilder configures a [node.Variable] within a [Builder]'s
// accumulating package.
type VariableBuilder struct {
	v *node.Variable
}

// Node returns the underlying [node.Variable].
func (b *VariableBuilder) Node() *node.Variable { return b.v }

// Pos overrides the variable's source position. Layout derives the
// basename of any file generated from this node from Pos.File, so
// the value decides the output filename; the fixture's synthetic
// `<pkg>/<lowercased-name>.go` keeps that basename non-empty. See
// [StructBuilder.Pos] for what an empty one costs.
func (b *VariableBuilder) Pos(p position.Pos) *VariableBuilder {
	b.v.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *VariableBuilder) Docs(lines ...string) *VariableBuilder {
	b.v.DocLines = append(b.v.DocLines, lines...)
	return b
}

// Directive attaches d to the variable's directive list.
func (b *VariableBuilder) Directive(d *directive.Directive) *VariableBuilder {
	b.v.DirectiveList = append(b.v.DirectiveList, d)
	return b
}

// Type records the declared type. Leave unset (or pass nil) for
// variables whose type is inferred from the initialiser expression.
func (b *VariableBuilder) Type(t *node.TypeRef) *VariableBuilder {
	b.v.Type = t
	return b
}

// InitExpr records the verbatim initialiser expression.
func (b *VariableBuilder) InitExpr(expr string) *VariableBuilder {
	b.v.InitExpr = expr
	return b
}
