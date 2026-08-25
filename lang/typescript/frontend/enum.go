// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// enumDecl converts `enum E { … }` into a [node.Enum].
//
// TypeScript's enum is first-class, so unlike Go's the frontend does
// not have to recognise it from a constant group — the declaration
// says what it is.
func (c *conv) enumDecl(n *ts.Node) *node.Enum {
	name := c.p.text(n.ChildByFieldName("name"))
	if name == "" {
		return nil
	}

	e := &node.Enum{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		Name:     name,
		Package:  c.pkg,
	}
	c.attachDocs(&e.BaseNode, n)
	c.stampDecl(&e.BaseNode, n)
	c.stampDecorators(&e.BaseNode, n)

	if hasToken(n, "const") {
		typescript.MetaConstEnum.SetAt(
			e.EnsureMeta(), true, meta.AuthorityPlugin, FrontendName, c.p.posAt(n),
		)
	}

	c.enumVariants(e, n.ChildByFieldName("body"))
	e.Underlying = c.enumUnderlying(e, n)
	return e
}

// enumVariants converts an `enum_body` into variants.
//
// A member is either a bare `property_identifier` — implicitly the
// next number — or an `enum_assignment` carrying an explicit value.
// The implicit value is left empty rather than computed: the rule
// that produces it is "one more than the previous numeric member",
// which a consumer can apply, and inventing a value here would record
// a literal the source never wrote.
func (c *conv) enumVariants(e *node.Enum, body *ts.Node) {
	if body == nil {
		return
	}
	for i := range body.NamedChildCount() {
		m := body.NamedChild(i)

		v := &node.EnumVariant{
			BaseNode: node.BaseNode{SourcePos: c.p.posAt(m)},
			Owner:    e,
		}
		switch m.Kind() {
		case "property_identifier":
			v.Name = c.p.text(m)
		case "enum_assignment":
			v.Name = c.propertyName(m.ChildByFieldName("name"))
			v.Value = c.p.text(m.ChildByFieldName("value"))
		default:
			continue
		}
		if v.Name == "" {
			continue
		}
		c.attachDocs(&v.BaseNode, m)
		e.Variants = append(e.Variants, v)
	}
}

// enumUnderlying reports the enum's underlying type.
//
// TypeScript enums are numeric unless their members are assigned
// strings; there is no declared underlying type to read, so it is
// derived. A member with no value is numeric by definition — the
// implicit value is a number — which is why the presence of one
// settles the question on its own.
func (c *conv) enumUnderlying(e *node.Enum, n *ts.Node) *node.TypeRef {
	stringly := false
	for _, v := range e.Variants {
		switch {
		case v.Value == "":
			return c.namedRef(n, "number", "")
		case isStringLiteral(v.Value):
			stringly = true
		default:
			return c.namedRef(n, "number", "")
		}
	}
	if stringly {
		return c.namedRef(n, "string", "")
	}
	return c.namedRef(n, "number", "")
}

// isStringLiteral reports whether a verbatim value is a quoted string.
func isStringLiteral(v string) bool {
	if len(v) < 2 {
		return false
	}
	q := v[0]
	return (q == '\'' || q == '"' || q == '`') && strings.HasSuffix(v, string(q))
}
