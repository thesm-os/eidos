// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// stampDecorators records the decorators that are children of n.
//
// A class, a field and a parameter all hold their decorators as
// children. A method does not, and a decorator written above an
// `export` line belongs to the export statement — see
// [conv.stampDecoratorNodes] for both.
func (c *conv) stampDecorators(base *node.BaseNode, n *ts.Node) {
	c.stampDecoratorNodes(base, decoratorChildren(n))
}

// stampDecoratorNodes records an explicitly-collected set of
// decorators on base, appending to whatever is already there.
//
// Appending rather than replacing, because a declaration's decorators
// can arrive from two places at once: a class written as
//
//	@A
//	export @B class C {}
//
// holds `B` itself and `A` on the export statement wrapping it, and
// both belong to the class.
//
// The grammar is not consistent about where a decorator sits, which
// is why collection is the caller's job:
//
//   - a class, field or parameter holds its own as children;
//   - a method's are siblings, directly under the class body;
//   - one written above an `export` line is a child of the export
//     statement rather than of the declaration.
func (c *conv) stampDecoratorNodes(base *node.BaseNode, decorators []*ts.Node) {
	if len(decorators) == 0 {
		return
	}
	bag := base.EnsureMeta()
	existing, _ := typescript.MetaDecorators.Get(bag)

	out := make([]typescript.Decorator, 0, len(existing)+len(decorators))
	out = append(out, existing...)
	for _, d := range decorators {
		name, args := c.decoratorParts(d)
		if name == "" {
			continue
		}
		out = append(out, typescript.Decorator{Name: name, Args: args})
	}
	if len(out) == 0 {
		return
	}
	typescript.MetaDecorators.SetAt(
		bag, out, meta.AuthorityPlugin, FrontendName, c.p.posAt(decorators[0]),
	)
}

// decoratorParts splits a decorator into the name it applies and its
// argument list.
//
// Two forms reach here. `@deco` is a bare identifier and carries no
// arguments; `@Column({…})` is a call expression whose `function`
// child is the name and whose `arguments` child is the list. A
// qualified name — `@a.b.c()` — arrives as a member expression and is
// taken whole, because `a.b.c` is the decorator's identity and the
// last segment alone would collide with any other `c`.
//
// Arguments are kept verbatim, parentheses included. They are
// arbitrary expressions, and a structured form would mean a second
// expression model in the frontend for the one case that needs it.
func (c *conv) decoratorParts(d *ts.Node) (name, args string) {
	inner := d.NamedChild(0)
	if inner == nil {
		return "", ""
	}
	if inner.Kind() != "call_expression" {
		// `@deco` — the whole inner node is the name.
		return c.p.text(inner), ""
	}
	return c.p.text(inner.ChildByFieldName("function")),
		c.p.text(inner.ChildByFieldName("arguments"))
}
