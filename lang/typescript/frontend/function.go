// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/node"
)

// functionDecl converts a `function_declaration` or a
// `function_signature` into a [node.Function].
//
// A signature is an overload declaration: several may share one name,
// each describing a different way to call the implementation that
// follows. They are kept as separate Functions marked with
// [typescript.MetaOverload] rather than folded into the
// implementation, because the model has one signature per Function
// and folding would need a shape it does not have. A consumer wanting
// the overload set groups by name.
func (c *conv) functionDecl(n *ts.Node) *node.Function {
	name := c.p.text(n.ChildByFieldName("name"))
	if name == "" {
		return nil
	}

	f := &node.Function{
		BaseNode:   node.BaseNode{SourcePos: c.p.posAt(n)},
		Name:       name,
		Package:    c.pkg,
		Params:     c.params(n.ChildByFieldName("parameters")),
		TypeParams: c.typeParams(n.ChildByFieldName("type_parameters")),
	}
	c.attachDocs(&f.BaseNode, n)
	c.stampDecl(&f.BaseNode, n)
	c.stampDecorators(&f.BaseNode, n)
	ownTypeParams(f.TypeParams, f)
	for _, p := range f.Params {
		p.Owner = f
	}

	if ret := c.typeRef(returnType(n), maxTypeDepth); ret != nil {
		f.Returns = []*node.Return{{
			BaseNode: node.BaseNode{SourcePos: ret.Pos()},
			Type:     ret,
			Owner:    f,
		}}
	}

	c.callableModifiers(f.EnsureMeta(), n)
	c.signatures[f] = signature{
		text:    c.signatureText(n),
		hasBody: n.ChildByFieldName("body") != nil,
	}
	return f
}

// signatureText renders a callable's declaration without its body,
// which is the form an overload alternative is written in.
//
// Taken as source text between the declaration's start and the start
// of its body rather than reassembled from the parts: an alternative
// exists to be rendered back, and the author's own spelling is what
// a reader expects to see.
func (c *conv) signatureText(n *ts.Node) string {
	full := c.p.text(n)
	body := n.ChildByFieldName("body")
	if body != nil {
		full = string(c.p.src[n.StartByte():body.StartByte()])
	}
	return strings.TrimRight(strings.TrimSpace(full), ";")
}

// returnType returns the type inside a function's `return_type`
// annotation.
//
// Distinct from [annotatedType], which reads the `type` field: a
// function's return annotation is under `return_type`, while a
// property's or parameter's type annotation is under `type`. Both
// wrap the type in a `type_annotation`.
func returnType(n *ts.Node) *ts.Node {
	ann := n.ChildByFieldName("return_type")
	if ann == nil {
		return nil
	}
	if ann.Kind() == "type_annotation" {
		return ann.NamedChild(0)
	}
	return ann
}
