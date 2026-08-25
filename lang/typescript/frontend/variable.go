// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/node"
)

// bindingDecl converts a `lexical_declaration` (`const` / `let`) or a
// `variable_declaration` (`var`) into one node per binding.
//
// `const a = 1, b = 2` is one statement declaring two things, so this
// returns a slice. `const` yields [node.Constant] and the rest yield
// [node.Variable] — the split the model already draws, and the one a
// consumer means when it asks for constants.
func (c *conv) bindingDecl(n *ts.Node) []node.Node {
	constant := hasToken(n, "const")

	var out []node.Node
	for i := range n.NamedChildCount() {
		d := n.NamedChild(i)
		if d.Kind() != "variable_declarator" {
			continue
		}
		name := c.bindingName(d.ChildByFieldName("name"))
		if name == "" {
			// A destructuring binding — `const { a, b } = o` — declares
			// names without a declaration to hang each on. Skipped
			// rather than recorded under an invented name.
			continue
		}
		if constant {
			out = append(out, c.constantFrom(n, d, name))
			continue
		}
		out = append(out, c.variableFrom(n, d, name))
	}
	return out
}

// constantFrom builds a [node.Constant] from one declarator.
func (c *conv) constantFrom(stmt, d *ts.Node, name string) *node.Constant {
	k := &node.Constant{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(d)},
		Name:     name,
		Package:  c.pkg,
		Type:     c.typeRef(annotatedType(d), maxTypeDepth),
		Value:    c.p.text(d.ChildByFieldName("value")),
	}
	c.attachDocs(&k.BaseNode, stmt)
	c.stampDecl(&k.BaseNode, stmt)
	return k
}

// variableFrom builds a [node.Variable] from one declarator.
func (c *conv) variableFrom(stmt, d *ts.Node, name string) *node.Variable {
	v := &node.Variable{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(d)},
		Name:     name,
		Package:  c.pkg,
		Type:     c.typeRef(annotatedType(d), maxTypeDepth),
		InitExpr: c.p.text(d.ChildByFieldName("value")),
	}
	c.attachDocs(&v.BaseNode, stmt)
	c.stampDecl(&v.BaseNode, stmt)
	return v
}
