// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// typeParams converts a `type_parameters` node — `<T extends U = V>`.
func (c *conv) typeParams(n *ts.Node) []*node.TypeParam {
	if n == nil {
		return nil
	}
	out := make([]*node.TypeParam, 0, n.NamedChildCount())
	for i := range n.NamedChildCount() {
		if p := c.typeParam(n.NamedChild(i)); p != nil {
			out = append(out, p)
		}
	}
	return out
}

// typeParam converts one type parameter.
//
// TypeScript's `extends` bound maps onto [node.Constraint]: Raw holds
// the printed form and Embedded the bound itself, matching what a Go
// interface bound produces. A parameter's default (`= V`) has no
// model field — Go has no equivalent — so it rides on
// [typescript.MetaTypeParamDefault].
func (c *conv) typeParam(n *ts.Node) *node.TypeParam {
	if n.Kind() != "type_parameter" {
		return nil
	}
	name := c.p.text(n.ChildByFieldName("name"))
	if name == "" {
		return nil
	}

	p := &node.TypeParam{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		Name:     name,
	}

	if bound := n.ChildByFieldName("constraint"); bound != nil {
		inner := boundType(bound)
		if t := c.typeRef(inner, maxTypeDepth); t != nil {
			p.Constraint = &node.Constraint{
				Raw:      c.p.text(inner),
				Embedded: []*node.TypeRef{t},
			}
		}
	}

	// The `value` field is a `default_type` node whose text includes
	// the `=`, so the type itself is one level down. Stamping the
	// node's own text would record `= {}` as the default's spelling.
	if def := n.ChildByFieldName("value"); def != nil {
		if inner := def.NamedChild(0); inner != nil {
			typescript.MetaTypeParamDefault.SetAt(
				p.EnsureMeta(), c.p.text(inner), meta.AuthorityPlugin, FrontendName, c.p.posAt(inner),
			)
		}
	}
	return p
}

// boundType unwraps a `constraint` node to the type it names.
func boundType(n *ts.Node) *ts.Node {
	if n.Kind() == "constraint" {
		return n.NamedChild(0)
	}
	return n
}
