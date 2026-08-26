// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// EnumBuilder configures a [node.Enum] as part of a [Builder]'s
// accumulating package.
type EnumBuilder struct {
	e *node.Enum

	// file is the synthetic source file the parent stamped; see
	// [InterfaceBuilder] for why members read it from here.
	file string
}

// Node returns the underlying [node.Enum].
func (b *EnumBuilder) Node() *node.Enum { return b.e }

// Pos overrides the enum's source position; see [ClassBuilder.Pos].
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

// Underlying sets the type the members carry.
//
// Derived rather than declared in TypeScript — an enum is numeric
// unless its members are assigned strings — so the frontend computes
// it and a fixture states it. [EnumBuilder.Strings] and
// [EnumBuilder.Numbers] set the two answers a real declaration
// produces.
func (b *EnumBuilder) Underlying(t *node.TypeRef) *EnumBuilder {
	b.e.Underlying = t
	return b
}

// Strings declares the enum's members to be strings, which is what
// the frontend derives for a declaration whose every member is
// assigned a quoted value.
func (b *EnumBuilder) Strings() *EnumBuilder {
	return b.Underlying(Named(typescript.ScalarString))
}

// Numbers declares the enum's members to be numbers — the frontend's
// answer for any declaration with a member that took the implicit
// counter.
func (b *EnumBuilder) Numbers() *EnumBuilder {
	return b.Underlying(Named(typescript.ScalarNumber))
}

// Const marks the declaration a `const enum`, which is inlined at
// every use site and emits no runtime object.
func (b *EnumBuilder) Const() *EnumBuilder {
	typescript.MetaConstEnum.Set(b.e.EnsureMeta(), true, markerAuthority)
	return b
}

// Variant declares a member carrying an explicit value, in verbatim
// source form — quotes included for a string member, so `'admin'`
// rather than `admin`.
//
// Pass an empty value for a member that took the implicit counter.
// The counter is deliberately not applied here: the rule is "one more
// than the previous numeric member", and recording a value the source
// never wrote would make a fixture disagree with what the frontend
// produces from the same declaration.
func (b *EnumBuilder) Variant(name, value string) *EnumBuilder {
	return b.VariantWith(name, value, nil)
}

// VariantWith is [EnumBuilder.Variant] with a callback for a member
// that carries docs or a directive.
func (b *EnumBuilder) VariantWith(name, value string, fn func(*VariantBuilder)) *EnumBuilder {
	v := &node.EnumVariant{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: b.file}},
		Name:     name,
		Value:    value,
		Owner:    b.e,
	}
	if fn != nil {
		fn(&VariantBuilder{v: v})
	}
	b.e.Variants = append(b.e.Variants, v)
	return b
}

// Method declares a method on the enum.
//
// TypeScript has no methods on an enum, so this builds a shape no
// `.ts` file produces. It exists because [node.Enum] carries the
// slice and a test asserting that a consumer ignores it needs a way
// to populate one.
func (b *EnumBuilder) Method(name string, fn func(*MethodBuilder)) *EnumBuilder {
	m := &node.Method{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: b.file}},
		Name:     name,
		Owner:    b.e,
	}
	if fn != nil {
		fn(&MethodBuilder{m: m})
	}
	b.e.Methods = append(b.e.Methods, m)
	return b
}

// VariantBuilder configures one [node.EnumVariant].
type VariantBuilder struct {
	v *node.EnumVariant
}

// Node returns the underlying [node.EnumVariant].
func (b *VariantBuilder) Node() *node.EnumVariant { return b.v }

// Pos overrides the member's source position; see [ClassBuilder.Pos].
func (b *VariantBuilder) Pos(p position.Pos) *VariantBuilder {
	b.v.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *VariantBuilder) Docs(lines ...string) *VariantBuilder {
	b.v.DocLines = append(b.v.DocLines, lines...)
	return b
}

// Directive attaches d to the member's directive list.
func (b *VariantBuilder) Directive(d *directive.Directive) *VariantBuilder {
	b.v.DirectiveList = append(b.v.DirectiveList, d)
	return b
}
