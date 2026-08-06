// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package storefixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// MethodBuilder configures a [node.Method] within an enclosing struct
// or interface. Parameters and returns are appended in declaration
// order; the underlying method's [node.Method.Owner] back-pointer is
// set by the parent sub-builder before the method is constructed.
type MethodBuilder struct {
	m *node.Method
}

// Node returns the underlying [node.Method].
func (b *MethodBuilder) Node() *node.Method { return b.m }

// Pos overrides the method's source position. The method inherits
// its enclosing declaration's synthetic file, matching how a
// frontend records every member of one declaration at the file it
// was parsed from. Layout derives the output basename from Pos.File
// whenever a plugin routes from a method, so overriding here
// redirects that output. See [StructBuilder.Pos].
func (b *MethodBuilder) Pos(p position.Pos) *MethodBuilder {
	b.m.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *MethodBuilder) Docs(lines ...string) *MethodBuilder {
	b.m.DocLines = append(b.m.DocLines, lines...)
	return b
}

// Directive attaches d to the method's directive list.
func (b *MethodBuilder) Directive(d *directive.Directive) *MethodBuilder {
	b.m.DirectiveList = append(b.m.DirectiveList, d)
	return b
}

// Receiver overrides the method's receiver type. [Builder.Struct]
// seeds methods with a pointer-to-named receiver matching the
// enclosing struct; override here when the test cares about value
// receivers or unusual receiver shapes.
//
// Receiver has no effect on methods constructed under an interface —
// interface methods carry a nil receiver by contract, and overriding
// to a non-nil value would violate that contract.
func (b *MethodBuilder) Receiver(t *node.TypeRef) *MethodBuilder {
	b.m.Receiver = t
	return b
}

// ReceiverName records the source-level receiver variable name (the
// `s` in `func (s *Repo) Get()`). Has no effect on interface methods.
func (b *MethodBuilder) ReceiverName(name string) *MethodBuilder {
	b.m.ReceiverName = name
	return b
}

// Param appends a positional parameter to the method's signature.
// The parameter's Owner back-pointer is wired automatically.
func (b *MethodBuilder) Param(name string, t *node.TypeRef) *MethodBuilder {
	b.m.Params = append(b.m.Params, &node.Param{
		Name:  name,
		Type:  t,
		Owner: b.m,
	})
	return b
}

// Variadic appends a variadic last parameter. The Type is the element
// type (`int` for `...int`), matching how a Go frontend records it.
func (b *MethodBuilder) Variadic(name string, elemT *node.TypeRef) *MethodBuilder {
	b.m.Params = append(b.m.Params, &node.Param{
		Name:     name,
		Type:     elemT,
		Variadic: true,
		Owner:    b.m,
	})
	return b
}

// Return appends an unnamed return slot to the method's signature.
func (b *MethodBuilder) Return(t *node.TypeRef) *MethodBuilder {
	b.m.Returns = append(b.m.Returns, &node.Return{Type: t})
	return b
}

// NamedReturn appends a return slot carrying a declared name, as a
// signature written `(item string, err error)` produces. Pass an
// empty name for the anonymous form, or use [MethodBuilder.Return].
func (b *MethodBuilder) NamedReturn(name string, t *node.TypeRef) *MethodBuilder {
	b.m.Returns = append(b.m.Returns, &node.Return{Name: name, Type: t})
	return b
}

// TypeParam declares a generic type parameter on the method. Rare in
// Go; methods normally inherit type parameters from their receiver.
// Pass nil for an implicit "any" bound, or use [Constraint] for an
// explicit named-bound constraint.
func (b *MethodBuilder) TypeParam(name string, constraint *node.Constraint) *MethodBuilder {
	b.m.TypeParams = append(b.m.TypeParams, &node.TypeParam{
		Name:       name,
		Constraint: constraint,
		Owner:      b.m,
	})
	return b
}
