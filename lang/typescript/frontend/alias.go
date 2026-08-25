// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/node"
)

// aliasDecl converts `type A<T> = B<T>` into a [node.Alias].
//
// IsAlias is always true. Go distinguishes `type X = Y` from
// `type X Y` — the first names an existing type, the second creates a
// new one — and TypeScript has only the first: `type` never creates a
// nominal type, and the compiler treats the alias and its target as
// the same type everywhere.
func (c *conv) aliasDecl(n *ts.Node) *node.Alias {
	name := c.p.text(n.ChildByFieldName("name"))
	if name == "" {
		return nil
	}

	a := &node.Alias{
		BaseNode:   node.BaseNode{SourcePos: c.p.posAt(n)},
		Name:       name,
		Package:    c.pkg,
		IsAlias:    true,
		Target:     c.typeRef(n.ChildByFieldName("value"), maxTypeDepth),
		TypeParams: c.typeParams(n.ChildByFieldName("type_parameters")),
	}
	c.attachDocs(&a.BaseNode, n)
	c.stampDecl(&a.BaseNode, n)
	c.stampDecorators(&a.BaseNode, n)
	ownTypeParams(a.TypeParams, a)
	return a
}
