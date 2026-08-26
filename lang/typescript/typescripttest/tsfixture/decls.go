// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// The declarations with no members to configure — an alias, a
// function, a binding — kept together rather than one file each. Each
// is a handful of setters over a different node, and splitting them
// would give five files whose whole content is Pos, Docs and
// Directive.

// AliasBuilder configures a [node.Alias] — TypeScript's `type X = Y`.
type AliasBuilder struct {
	a    *node.Alias
	file string
}

// Node returns the underlying [node.Alias].
func (b *AliasBuilder) Node() *node.Alias { return b.a }

// Pos overrides the alias's source position; see [ClassBuilder.Pos].
func (b *AliasBuilder) Pos(p position.Pos) *AliasBuilder {
	b.a.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *AliasBuilder) Docs(lines ...string) *AliasBuilder {
	b.a.DocLines = append(b.a.DocLines, lines...)
	return b
}

// Directive attaches d to the alias's directive list.
func (b *AliasBuilder) Directive(d *directive.Directive) *AliasBuilder {
	b.a.DirectiveList = append(b.a.DirectiveList, d)
	return b
}

// Target sets the type the alias names.
func (b *AliasBuilder) Target(t *node.TypeRef) *AliasBuilder {
	b.a.Target = t
	return b
}

// TypeParam declares a generic parameter on the alias — the `T` in
// `type Box<T> = { value: T }`.
func (b *AliasBuilder) TypeParam(name string, c *node.Constraint) *AliasBuilder {
	b.a.TypeParams = append(b.a.TypeParams, &node.TypeParam{
		Name: name, Constraint: c, Owner: b.a,
	})
	return b
}

// FunctionBuilder configures a [node.Function] — a module-level
// function declaration.
type FunctionBuilder struct {
	f *node.Function
}

// Node returns the underlying [node.Function].
func (b *FunctionBuilder) Node() *node.Function { return b.f }

// Pos overrides the function's source position; see [ClassBuilder.Pos].
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

// Param appends a positional parameter.
func (b *FunctionBuilder) Param(name string, t *node.TypeRef) *FunctionBuilder {
	b.f.Params = append(b.f.Params, &node.Param{Name: name, Type: t, Owner: b.f})
	return b
}

// OptionalParam appends a `?`-marked parameter.
func (b *FunctionBuilder) OptionalParam(name string, t *node.TypeRef) *FunctionBuilder {
	b.Param(name, t)
	p := b.f.Params[len(b.f.Params)-1]
	typescript.MetaOptional.Set(p.EnsureMeta(), true, markerAuthority)
	return b
}

// Rest appends a rest parameter, carrying the element type; see
// [MethodBuilder.Rest].
func (b *FunctionBuilder) Rest(name string, elem *node.TypeRef) *FunctionBuilder {
	b.f.Params = append(b.f.Params, &node.Param{
		Name: name, Type: elem, Variadic: true, Owner: b.f,
	})
	return b
}

// Return sets the function's return type; see [MethodBuilder.Return]
// for what a second call describes.
func (b *FunctionBuilder) Return(t *node.TypeRef) *FunctionBuilder {
	b.f.Returns = append(b.f.Returns, &node.Return{Type: t})
	return b
}

// TypeParam declares a generic parameter.
func (b *FunctionBuilder) TypeParam(name string, c *node.Constraint) *FunctionBuilder {
	b.f.TypeParams = append(b.f.TypeParams, &node.TypeParam{
		Name: name, Constraint: c, Owner: b.f,
	})
	return b
}

// Async marks the function `async`.
func (b *FunctionBuilder) Async() *FunctionBuilder {
	typescript.MetaAsync.Set(b.f.EnsureMeta(), true, markerAuthority)
	return b
}

// Generator marks the function a generator — `function*`.
func (b *FunctionBuilder) Generator() *FunctionBuilder {
	typescript.MetaGenerator.Set(b.f.EnsureMeta(), true, markerAuthority)
	return b
}

// Overload appends an overload signature in verbatim source form; see
// [MethodBuilder.Overload].
func (b *FunctionBuilder) Overload(text string) *FunctionBuilder {
	list, _ := typescript.MetaOverloads.Get(b.f.Meta())
	list = append(list, typescript.Overload{Text: text})
	typescript.MetaOverloads.Set(b.f.EnsureMeta(), list, markerAuthority)
	return b
}

// VariableBuilder configures a [node.Variable] — a `let` or `var`
// binding.
type VariableBuilder struct {
	v *node.Variable
}

// Node returns the underlying [node.Variable].
func (b *VariableBuilder) Node() *node.Variable { return b.v }

// Pos overrides the binding's source position; see [ClassBuilder.Pos].
func (b *VariableBuilder) Pos(p position.Pos) *VariableBuilder {
	b.v.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *VariableBuilder) Docs(lines ...string) *VariableBuilder {
	b.v.DocLines = append(b.v.DocLines, lines...)
	return b
}

// Directive attaches d to the binding's directive list.
func (b *VariableBuilder) Directive(d *directive.Directive) *VariableBuilder {
	b.v.DirectiveList = append(b.v.DirectiveList, d)
	return b
}

// Type sets the binding's declared type. Leave it unset for a binding
// whose type TypeScript infers, which is legal and common.
func (b *VariableBuilder) Type(t *node.TypeRef) *VariableBuilder {
	b.v.Type = t
	return b
}

// Value records the initialiser in verbatim source form.
func (b *VariableBuilder) Value(text string) *VariableBuilder {
	b.v.InitExpr = text
	return b
}

// ConstantBuilder configures a [node.Constant] — a `const` binding.
type ConstantBuilder struct {
	c *node.Constant
}

// Node returns the underlying [node.Constant].
func (b *ConstantBuilder) Node() *node.Constant { return b.c }

// Pos overrides the binding's source position; see [ClassBuilder.Pos].
func (b *ConstantBuilder) Pos(p position.Pos) *ConstantBuilder {
	b.c.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *ConstantBuilder) Docs(lines ...string) *ConstantBuilder {
	b.c.DocLines = append(b.c.DocLines, lines...)
	return b
}

// Directive attaches d to the binding's directive list.
func (b *ConstantBuilder) Directive(d *directive.Directive) *ConstantBuilder {
	b.c.DirectiveList = append(b.c.DirectiveList, d)
	return b
}

// Type sets the binding's declared type.
func (b *ConstantBuilder) Type(t *node.TypeRef) *ConstantBuilder {
	b.c.Type = t
	return b
}

// Value records the initialiser in verbatim source form.
func (b *ConstantBuilder) Value(text string) *ConstantBuilder {
	b.c.Value = text
	return b
}
