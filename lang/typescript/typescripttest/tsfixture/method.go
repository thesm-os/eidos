// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// MethodBuilder configures a [node.Method] within an enclosing
// interface or class. Parameters and returns are appended in
// declaration order; the underlying method's Owner back-pointer is set
// by the parent sub-builder before the method is constructed.
//
// There is no Receiver method. TypeScript has no receiver in a
// signature — a method binds `this` implicitly — so the frontend
// leaves [node.Method.Receiver] nil for both classes and interfaces,
// and a fixture that set one would build a graph no TypeScript run
// produces.
type MethodBuilder struct {
	m *node.Method
}

// Node returns the underlying [node.Method].
func (b *MethodBuilder) Node() *node.Method { return b.m }

// Pos overrides the method's source position. A method inherits its
// enclosing declaration's synthetic file; see [ClassBuilder.Pos] for
// why the value is load-bearing.
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

// Param appends a positional parameter. The parameter's Owner
// back-pointer is wired automatically.
func (b *MethodBuilder) Param(name string, t *node.TypeRef) *MethodBuilder {
	b.m.Params = append(b.m.Params, &node.Param{Name: name, Type: t, Owner: b.m})
	return b
}

// OptionalParam appends a `?`-marked parameter.
func (b *MethodBuilder) OptionalParam(name string, t *node.TypeRef) *MethodBuilder {
	b.Param(name, t)
	p := b.m.Params[len(b.m.Params)-1]
	typescript.MetaOptional.Set(p.EnsureMeta(), true, markerAuthority)
	return b
}

// Rest appends a rest parameter — `...items: T[]`.
//
// The Type is the *element* type, matching what
// [node.Param.Variadic] documents and what the frontend records: a
// fixture storing the array would describe `...items: T[][]`.
func (b *MethodBuilder) Rest(name string, elem *node.TypeRef) *MethodBuilder {
	b.m.Params = append(b.m.Params, &node.Param{
		Name: name, Type: elem, Variadic: true, Owner: b.m,
	})
	return b
}

// Return sets the method's return type.
//
// Appends, like its Go counterpart, because the model holds a slice
// — but a TypeScript signature returns one value, and the backend
// spells several slots as the tuple that holds them. Calling Return
// twice therefore describes `[A, B]` rather than a second return.
func (b *MethodBuilder) Return(t *node.TypeRef) *MethodBuilder {
	b.m.Returns = append(b.m.Returns, &node.Return{Type: t})
	return b
}

// TypeParam declares a generic parameter on the method. Pass nil for
// an unbounded parameter, or use [Constraint] for `<T extends Shape>`.
func (b *MethodBuilder) TypeParam(name string, constraint *node.Constraint) *MethodBuilder {
	b.m.TypeParams = append(b.m.TypeParams, &node.TypeParam{
		Name:       name,
		Constraint: constraint,
		Owner:      b.m,
	})
	return b
}

// Optional marks the method `?` — a member an implementation need not
// declare.
func (b *MethodBuilder) Optional() *MethodBuilder {
	return b.mark(typescript.MetaOptional)
}

// Async marks the method `async`.
func (b *MethodBuilder) Async() *MethodBuilder {
	return b.mark(typescript.MetaAsync)
}

// Generator marks the method a generator — `*values()`.
func (b *MethodBuilder) Generator() *MethodBuilder {
	return b.mark(typescript.MetaGenerator)
}

// Static marks the method `static`.
func (b *MethodBuilder) Static() *MethodBuilder {
	return b.mark(typescript.MetaStatic)
}

// Abstract marks the method `abstract` — declared without a body on a
// class that cannot be instantiated.
func (b *MethodBuilder) Abstract() *MethodBuilder {
	return b.mark(typescript.MetaAbstract)
}

// Accessor records that the method is a property accessor — pass
// [typescript.AccessorGet] or [typescript.AccessorSet].
//
// A getter is a method in the model whose use site is a property
// read, which is the whole reason it is recorded rather than
// inferred: a consumer generating a call for one would emit `u.name()`
// where the source says `u.name`.
func (b *MethodBuilder) Accessor(kind string) *MethodBuilder {
	typescript.MetaAccessor.Set(b.m.EnsureMeta(), kind, markerAuthority)
	return b
}

// Visibility records an explicit access modifier; see
// [FieldBuilder.Visibility].
func (b *MethodBuilder) Visibility(level string) *MethodBuilder {
	typescript.MetaVisibility.Set(b.m.EnsureMeta(), level, markerAuthority)
	return b
}

// Overload appends an overload signature in verbatim source form,
// without its trailing semicolon — `find(id: string): User`.
//
// In source order, which is resolution order: TypeScript matches a
// call against the signatures top-down and takes the first that fits,
// so a fixture reordering them describes a different callable.
func (b *MethodBuilder) Overload(text string) *MethodBuilder {
	list, _ := typescript.MetaOverloads.Get(b.m.Meta())
	list = append(list, typescript.Overload{Text: text})
	typescript.MetaOverloads.Set(b.m.EnsureMeta(), list, markerAuthority)
	return b
}

// Decorator appends a decorator to the method, in source order.
func (b *MethodBuilder) Decorator(name string, args ...string) *MethodBuilder {
	appendDecorator(b.m, name, args)
	return b
}

// mark sets a bool-valued key on the method.
func (b *MethodBuilder) mark(key boolKey) *MethodBuilder {
	key.Set(b.m.EnsureMeta(), true, markerAuthority)
	return b
}
