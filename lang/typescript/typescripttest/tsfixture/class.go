// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// ClassBuilder configures a [node.Struct] as part of a [Builder]'s
// accumulating package — a TypeScript class.
//
// The sub-builder is created by [Builder.Class] and handed to that
// method's callback; the underlying node is appended to the package
// after the callback returns. Property, method and heritage
// declarations within the callback see their owner back-pointers
// wired automatically.
type ClassBuilder struct {
	s *node.Struct

	// file is the synthetic source file the parent stamped; see
	// [InterfaceBuilder] for why members read it from here rather
	// than from the node's own position.
	file string
}

// Node returns the underlying [node.Struct]. Use this accessor to set
// typed metadata the builder does not expose, or to assert against
// the node directly.
func (b *ClassBuilder) Node() *node.Struct { return b.s }

// Pos overrides the class's source position.
//
// The position is load-bearing, not decoration. The Layout phase
// composes a generated file's name as
// `<origin-basename><plugin-suffix>`, and the basename comes from the
// origin's Pos.File — so this value decides where everything
// generated from the class lands. A zero position yields an empty
// basename and the filename collapses to the bare suffix, which is a
// dotfile: written to disk, valid TypeScript, loaded by nothing, and
// undiagnosed at any severity.
//
// The fixture therefore seeds `<pkg>/<lowercased-name>.ts` rather
// than leaving the node positionless, and members declared inside the
// callback inherit that file. Call Pos when a test pins a specific
// generated filename or asserts on a diagnostic's reported location;
// otherwise the default already routes correctly.
func (b *ClassBuilder) Pos(p position.Pos) *ClassBuilder {
	b.s.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *ClassBuilder) Docs(lines ...string) *ClassBuilder {
	b.s.DocLines = append(b.s.DocLines, lines...)
	return b
}

// Directive attaches d to the class's directive list.
func (b *ClassBuilder) Directive(d *directive.Directive) *ClassBuilder {
	b.s.DirectiveList = append(b.s.DirectiveList, d)
	return b
}

// TypeParam declares a generic parameter. Pass nil for an unbounded
// parameter, or use [Constraint] for `<T extends Shape>`.
func (b *ClassBuilder) TypeParam(name string, c *node.Constraint) *ClassBuilder {
	b.s.TypeParams = append(b.s.TypeParams, &node.TypeParam{
		Name: name, Constraint: c, Owner: b.s,
	})
	return b
}

// Extends records the base class.
//
// A class extends exactly one, which the fixture does not enforce:
// the graph a bridge or a broken frontend produces is exactly what a
// test asserting on the diagnostic needs to be able to build.
func (b *ClassBuilder) Extends(t *node.TypeRef) *ClassBuilder {
	return b.heritage(t, typescript.HeritageExtends)
}

// Implements records an interface the class implements.
//
// Distinct from [ClassBuilder.Extends] because the two do different
// things: extending inherits members, implementing only asserts that
// they are present. A consumer resolving a member set that treated
// them alike would report members the class never declares.
func (b *ClassBuilder) Implements(t *node.TypeRef) *ClassBuilder {
	return b.heritage(t, typescript.HeritageImplements)
}

// Field declares a property on the class and runs fn (when non-nil)
// against a [FieldBuilder] to configure it.
func (b *ClassBuilder) Field(name string, t *node.TypeRef, fn func(*FieldBuilder)) *ClassBuilder {
	f := &node.Field{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: b.file}},
		Name:     name,
		Type:     t,
		Owner:    b.s,
	}
	if fn != nil {
		fn(&FieldBuilder{f: f})
	}
	b.s.Fields = append(b.s.Fields, f)
	return b
}

// Method declares a method on the class and runs fn (when non-nil)
// against a [MethodBuilder] to configure it.
//
// The method carries no receiver: TypeScript binds `this` implicitly
// and the frontend leaves [node.Method.Receiver] nil, so a fixture
// setting one would build a graph no TypeScript run produces.
func (b *ClassBuilder) Method(name string, fn func(*MethodBuilder)) *ClassBuilder {
	m := &node.Method{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: b.file}},
		Name:     name,
		Owner:    b.s,
	}
	if fn != nil {
		fn(&MethodBuilder{m: m})
	}
	b.s.Methods = append(b.s.Methods, m)
	return b
}

// Abstract marks the class `abstract`.
func (b *ClassBuilder) Abstract() *ClassBuilder {
	typescript.MetaAbstract.Set(b.s.EnsureMeta(), true, markerAuthority)
	return b
}

// Decorator appends a decorator to the class, in source order.
func (b *ClassBuilder) Decorator(name string, args ...string) *ClassBuilder {
	appendDecorator(b.s, name, args)
	return b
}

// heritage records one clause under the given kind.
func (b *ClassBuilder) heritage(t *node.TypeRef, kind string) *ClassBuilder {
	e := &node.Embed{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: b.file}},
		Type:     t,
		Owner:    b.s,
	}
	typescript.MetaHeritage.Set(e.EnsureMeta(), kind, markerAuthority)
	b.s.Embeds = append(b.s.Embeds, e)
	return b
}
