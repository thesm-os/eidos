// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// FunctionBuilder configures a [node.Function] within a [Builder]'s
// accumulating package. Functions differ from methods in that they
// have no receiver and are addressed by package-qualified name.
type FunctionBuilder struct {
	f *node.Function
}

// Node returns the underlying [node.Function].
func (b *FunctionBuilder) Node() *node.Function { return b.f }

// Pos overrides the function's source position. Layout derives the
// basename of any file generated from this node from Pos.File, so
// the value decides the output filename; the fixture's synthetic
// `<pkg>/<lowercased-name>.go` keeps that basename non-empty. See
// [StructBuilder.Pos] for what an empty one costs.
func (b *FunctionBuilder) Pos(p position.Pos) *FunctionBuilder {
	b.f.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *FunctionBuilder) Docs(lines ...string) *FunctionBuilder {
	b.f.DocLines = append(b.f.DocLines, lines...)
	return b
}

// Directive attaches d to the function's directive list.
func (b *FunctionBuilder) Directive(d *directive.Directive) *FunctionBuilder {
	b.f.DirectiveList = append(b.f.DirectiveList, d)
	return b
}

// Param appends a positional parameter to the function's signature.
func (b *FunctionBuilder) Param(name string, t *node.TypeRef) *FunctionBuilder {
	b.f.Params = append(b.f.Params, &node.Param{
		Name:  name,
		Type:  t,
		Owner: b.f,
	})
	return b
}

// Variadic appends a variadic last parameter.
func (b *FunctionBuilder) Variadic(name string, elemT *node.TypeRef) *FunctionBuilder {
	b.f.Params = append(b.f.Params, &node.Param{
		Name:     name,
		Type:     elemT,
		Variadic: true,
		Owner:    b.f,
	})
	return b
}

// Return appends an unnamed return slot.
func (b *FunctionBuilder) Return(t *node.TypeRef) *FunctionBuilder {
	b.f.Returns = append(b.f.Returns, &node.Return{Type: t})
	return b
}

// NamedReturn appends a return slot carrying a declared name, as a
// signature written `(item string, err error)` produces. Pass an
// empty name for the anonymous form, or use [Return].
func (b *FunctionBuilder) NamedReturn(name string, t *node.TypeRef) *FunctionBuilder {
	b.f.Returns = append(b.f.Returns, &node.Return{Name: name, Type: t})
	return b
}

// TypeParam declares a generic type parameter on the function. Pass
// nil for an implicit "any" bound, or use [Constraint] for an
// explicit named-bound constraint.
func (b *FunctionBuilder) TypeParam(name string, constraint *node.Constraint) *FunctionBuilder {
	b.f.TypeParams = append(b.f.TypeParams, &node.TypeParam{
		Name:       name,
		Constraint: constraint,
		Owner:      b.f,
	})
	return b
}
