// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// structDecl converts a class declaration into a [node.Struct].
//
// A class is instantiable, which is what [node.Struct] models. Its
// interface counterpart is [conv.interfaceDecl]; the two share
// everything below the declaration line, because a TypeScript class
// body and interface body declare the same kinds of member.
func (c *conv) structDecl(n *ts.Node) *node.Struct {
	name := c.declName(n, "class")
	if name == "" {
		return nil
	}

	s := &node.Struct{
		BaseNode:   node.BaseNode{SourcePos: c.p.posAt(n)},
		Name:       name,
		Package:    c.pkg,
		TypeParams: c.typeParams(n.ChildByFieldName("type_parameters")),
	}
	c.attachDocs(&s.BaseNode, n)
	c.stampDecl(&s.BaseNode, n)
	c.stampDecorators(&s.BaseNode, n)

	if n.Kind() == "abstract_class_declaration" {
		typescript.MetaAbstract.SetAt(
			s.EnsureMeta(), true, meta.AuthorityPlugin, FrontendName, c.p.posAt(n),
		)
	}

	for _, e := range c.heritage(n) {
		e.Owner = s
		s.Embeds = append(s.Embeds, e)
	}

	members := c.members(n.ChildByFieldName("body"), s, s.EnsureMeta())
	s.Fields, s.Methods = members.fields, members.methods
	s.Methods = c.foldMethodOverloads(s.Methods)
	ownTypeParams(s.TypeParams, s)

	return s
}

// interfaceDecl converts an interface declaration into a
// [node.Interface].
//
// An interface is a contract rather than a type values are made of,
// which is the distinction [node.Interface] carries. Its properties
// land in Fields — most TypeScript interfaces declare nothing else —
// and its method signatures in Methods.
func (c *conv) interfaceDecl(n *ts.Node) *node.Interface {
	name := c.declName(n, "interface")
	if name == "" {
		return nil
	}

	i := &node.Interface{
		BaseNode:   node.BaseNode{SourcePos: c.p.posAt(n)},
		Name:       name,
		Package:    c.pkg,
		TypeParams: c.typeParams(n.ChildByFieldName("type_parameters")),
	}
	c.attachDocs(&i.BaseNode, n)
	c.stampDecl(&i.BaseNode, n)
	c.stampDecorators(&i.BaseNode, n)

	for _, e := range c.heritage(n) {
		e.Owner = i
		i.Embeds = append(i.Embeds, e)
	}

	members := c.members(n.ChildByFieldName("body"), i, i.EnsureMeta())
	i.Fields, i.Methods = members.fields, members.methods
	i.Methods = c.foldMethodOverloads(i.Methods)
	ownTypeParams(i.TypeParams, i)

	return i
}

// declName returns a declaration's identifier, reporting the
// anonymous case rather than dropping it silently.
func (c *conv) declName(n *ts.Node, what string) string {
	name := c.p.text(n.ChildByFieldName("name"))
	if name == "" {
		// `export default class {}` declares a type nothing can
		// reference by name, so there is nothing for a generator to
		// key on and nothing to emit against.
		c.ps.Warnf(c.p.posAt(n),
			"anonymous %s declaration skipped: nothing can reference it by name", what)
	}
	return name
}

// heritage converts the extends and implements clauses of both forms.
//
// An interface carries `extends_type_clause` directly; a class wraps
// `extends_clause` and `implements_clause` in `class_heritage`. Both
// produce [node.Embed] entries, told apart by
// [typescript.MetaHeritage] — the distinction matters because
// extending inherits members while implementing only asserts them.
func (c *conv) heritage(n *ts.Node) []*node.Embed {
	var out []*node.Embed
	for i := range n.NamedChildCount() {
		child := n.NamedChild(i)
		switch child.Kind() {
		case "extends_type_clause":
			out = append(out, c.heritageClause(child, typescript.HeritageExtends)...)
		case "class_heritage":
			for j := range child.NamedChildCount() {
				clause := child.NamedChild(j)
				kind := typescript.HeritageImplements
				if clause.Kind() == "extends_clause" {
					kind = typescript.HeritageExtends
				}
				out = append(out, c.heritageClause(clause, kind)...)
			}
		}
	}
	return out
}

// heritageClause converts one clause's type list into embeds.
func (c *conv) heritageClause(n *ts.Node, kind string) []*node.Embed {
	var out []*node.Embed
	for i := range n.NamedChildCount() {
		child := n.NamedChild(i)
		if child.Kind() == "type_arguments" {
			// `extends B<T>` puts the arguments beside the name rather
			// than under it; typeRef on the name alone would drop them,
			// and they are already folded in by the generic_type case
			// when the grammar nests them. Skipping here avoids a
			// second, argument-only embed.
			continue
		}
		t := c.typeRef(child, maxTypeDepth)
		if t == nil {
			continue
		}
		e := &node.Embed{
			BaseNode: node.BaseNode{SourcePos: c.p.posAt(child)},
			Type:     t,
		}
		typescript.MetaHeritage.SetAt(
			e.EnsureMeta(), kind, meta.AuthorityPlugin, FrontendName, c.p.posAt(child),
		)
		out = append(out, e)
	}
	return out
}

// memberSet is what a class or interface body yields.
//
// Returned rather than assigned, because the two hosts are different
// types and the body converter should not care which one asked. The
// caller wires the slices onto whichever it is building.
type memberSet struct {
	fields  []*node.Field
	methods []*node.Method
}

// members converts a class or interface body.
//
// A TypeScript class body and interface body declare the same kinds
// of member, differing only in whether an implementation is present,
// so one converter serves both. Owner is wired to the supplied host;
// index and construct signatures have no model equivalent and land on
// bag.
func (c *conv) members(body *ts.Node, owner node.Node, bag *meta.Bag) memberSet {
	var out memberSet
	if body == nil {
		return out
	}

	// A method's decorators are siblings that precede it rather than
	// children of it, so they are collected as the walk passes them
	// and handed to the method they belong to.
	var pending []*ts.Node

	for i := range body.NamedChildCount() {
		m := body.NamedChild(i)
		switch m.Kind() {
		case kindDecorator:
			pending = append(pending, m)
			continue

		case "property_signature", "public_field_definition":
			if f := c.field(m, maxTypeDepth); f != nil {
				c.stampDecoratorNodes(&f.BaseNode, pending)
				f.Owner = owner
				out.fields = append(out.fields, f)
			}

		case "method_signature", "method_definition":
			if mm := c.method(m); mm != nil {
				c.stampDecoratorNodes(&mm.BaseNode, pending)
				mm.Owner = owner
				out.methods = append(out.methods, mm)
				out.fields = append(out.fields, parameterProperties(owner, mm)...)
			}

		case kindIndexSignature:
			typescript.MetaIndexSignature.SetAt(
				bag, c.p.text(m), meta.AuthorityPlugin, FrontendName, c.p.posAt(m),
			)

		case "construct_signature":
			typescript.MetaConstructSignature.SetAt(
				bag, c.p.text(m), meta.AuthorityPlugin, FrontendName, c.p.posAt(m),
			)
		}
		// Anything that is not a decorator consumes the pending set,
		// so a decorator never attaches across an intervening member.
		pending = nil
	}
	return out
}

// parameterProperties returns the fields a constructor's parameter
// properties declare on the owning type.
//
// `constructor(public y: string)` declares a member, not just a
// parameter. The declaration sits in the parameter list, so a
// consumer walking Fields would not see it and a consumer walking
// Params would not know it becomes one. Both views carry it, linked
// by [typescript.MetaParameterProperty].
func parameterProperties(owner node.Node, m *node.Method) []*node.Field {
	if m.Name != "constructor" {
		return nil
	}
	var out []*node.Field
	for _, p := range m.Params {
		if ok, _ := typescript.MetaParameterProperty.Get(p.Meta()); !ok {
			continue
		}
		f := &node.Field{
			BaseNode: node.BaseNode{SourcePos: p.Pos()},
			Name:     p.Name,
			Type:     p.Type,
			Owner:    owner,
		}
		carryMemberMeta(p.Meta(), f.EnsureMeta())
		out = append(out, f)
	}
	return out
}

// carryMemberMeta copies the modifier facts a parameter property
// contributes to the field it declares. Only modifiers travel: the
// name, type and position are set from the parameter directly.
func carryMemberMeta(from, to *meta.Bag) {
	for _, k := range []meta.Key[bool]{
		typescript.MetaReadonly,
		typescript.MetaOptional,
		typescript.MetaParameterProperty,
	} {
		if v, ok := k.Get(from); ok {
			k.Set(to, v, FrontendName)
		}
	}
	if v, ok := typescript.MetaVisibility.Get(from); ok {
		typescript.MetaVisibility.Set(to, v, FrontendName)
	}
}

// ownTypeParams wires each type parameter's Owner back-pointer.
func ownTypeParams(params []*node.TypeParam, owner node.Node) {
	for _, p := range params {
		p.Owner = owner
	}
}
