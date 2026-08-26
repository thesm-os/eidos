// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// InterfaceBuilder configures a [node.Interface] as part of a
// [Builder]'s accumulating package. The sub-builder is created by
// [Builder.Interface] and handed to that method's callback; the
// underlying interface is appended to the package after the callback
// returns.
//
// Property, method and heritage declarations within the callback see
// their owner back-pointers wired automatically, matching the shape a
// real frontend produces.
type InterfaceBuilder struct {
	i *node.Interface

	// file is the synthetic source file [Builder.Interface] stamped on
	// the declaration. Members inherit it verbatim: a real frontend
	// records every member at the file the declaration was parsed
	// from, and Layout routes from whichever of them a plugin picks as
	// its origin.
	//
	// Read at member-declaration time from the value the parent
	// computed rather than from the node's own position — a Pos call
	// inside the callback would otherwise make a member's file depend
	// on whether it was declared before or after it.
	file string
}

// Node returns the underlying [node.Interface].
func (b *InterfaceBuilder) Node() *node.Interface { return b.i }

// Pos overrides the interface's source position; see
// [ClassBuilder.Pos] for why the value is load-bearing.
func (b *InterfaceBuilder) Pos(p position.Pos) *InterfaceBuilder {
	b.i.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *InterfaceBuilder) Docs(lines ...string) *InterfaceBuilder {
	b.i.DocLines = append(b.i.DocLines, lines...)
	return b
}

// Directive attaches d to the interface's directive list.
func (b *InterfaceBuilder) Directive(d *directive.Directive) *InterfaceBuilder {
	b.i.DirectiveList = append(b.i.DirectiveList, d)
	return b
}

// TypeParam declares a generic parameter. Pass nil for an unbounded
// parameter, or use [Constraint] for `<T extends Shape>`.
func (b *InterfaceBuilder) TypeParam(name string, c *node.Constraint) *InterfaceBuilder {
	b.i.TypeParams = append(b.i.TypeParams, &node.TypeParam{
		Name: name, Constraint: c, Owner: b.i,
	})
	return b
}

// Extends records an interface this one extends.
//
// Stamped [typescript.HeritageExtends], which is also what an unmarked
// embed reads as — the reading that holds for an interface, since
// `implements` is a class clause and an interface has no other kind.
// Stamped anyway so a consumer never has to know that.
func (b *InterfaceBuilder) Extends(t *node.TypeRef) *InterfaceBuilder {
	e := &node.Embed{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: b.file}},
		Type:     t,
		Owner:    b.i,
	}
	typescript.MetaHeritage.Set(e.EnsureMeta(), typescript.HeritageExtends, markerAuthority)
	b.i.Embeds = append(b.i.Embeds, e)
	return b
}

// Field declares a property on the interface and runs fn (when
// non-nil) against a [FieldBuilder] to configure it.
//
// A TypeScript interface declares properties alongside methods — see
// ADR-0008 for why [node.Interface] carries a field list at all — so
// this is the common case rather than the unusual one.
func (b *InterfaceBuilder) Field(
	name string, t *node.TypeRef, fn func(*FieldBuilder),
) *InterfaceBuilder {
	f := &node.Field{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: b.file}},
		Name:     name,
		Type:     t,
		Owner:    b.i,
	}
	if fn != nil {
		fn(&FieldBuilder{f: f})
	}
	b.i.Fields = append(b.i.Fields, f)
	return b
}

// Method declares a method signature on the interface and runs fn
// (when non-nil) against a [MethodBuilder] to configure it.
func (b *InterfaceBuilder) Method(name string, fn func(*MethodBuilder)) *InterfaceBuilder {
	m := &node.Method{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: b.file}},
		Name:     name,
		Owner:    b.i,
	}
	if fn != nil {
		fn(&MethodBuilder{m: m})
	}
	b.i.Methods = append(b.i.Methods, m)
	return b
}

// IndexSignature records an index signature in verbatim source form —
// `[key: string]: T`.
//
// Verbatim because the model has no variant for it: an index
// signature declares the shape of *any* key rather than a named
// member, so there is no field to hold it. The backend spells the text
// back out.
func (b *InterfaceBuilder) IndexSignature(text string) *InterfaceBuilder {
	typescript.MetaIndexSignature.Set(b.i.EnsureMeta(), text, markerAuthority)
	return b
}

// ConstructSignature records a `new (…): T` signature in verbatim
// source form, for the same reason [InterfaceBuilder.IndexSignature]
// is verbatim.
func (b *InterfaceBuilder) ConstructSignature(text string) *InterfaceBuilder {
	typescript.MetaConstructSignature.Set(b.i.EnsureMeta(), text, markerAuthority)
	return b
}
