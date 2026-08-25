// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// importDecl converts an `import_statement` into a [node.Import].
//
// Path is the specifier with its quotes removed, which is what a
// consumer compares and what a backend re-emits. The specifier is
// also kept verbatim on [typescript.MetaModuleSpecifier], because
// `./user` and `user` resolve differently and a normalisation step
// that lost the leading `./` would silently turn a relative import
// into a package one.
func (c *conv) importDecl(n *ts.Node) *node.Import {
	src := n.ChildByFieldName("source")
	if src == nil {
		// `import x = require('y')` keeps its source on the require
		// clause rather than on the statement. It is still a module
		// dependency, so it is recorded as one.
		return c.importRequireDecl(n)
	}
	path := stringValue(c.p.text(src))
	if path == "" {
		return nil
	}

	imp := &node.Import{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		Path:     path,
		Alias:    c.importAlias(n),
	}
	c.attachDocs(&imp.BaseNode, n)

	bag := imp.EnsureMeta()
	pos := c.p.posAt(n)
	typescript.MetaModuleSpecifier.SetAt(bag, path, meta.AuthorityPlugin, FrontendName, pos)
	if hasToken(n, "type") {
		typescript.MetaTypeOnly.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	return imp
}

// importRequireDecl converts the CommonJS interop form
// `import x = require('y')`.
//
// The same dependency an ES import records, written the way a
// TypeScript file consuming a CommonJS module has to write it when
// `esModuleInterop` is off. Skipping it would leave such a file
// looking as though it imports nothing.
func (c *conv) importRequireDecl(n *ts.Node) *node.Import {
	clause := firstChildOfKind(n, "import_require_clause")
	if clause == nil {
		return nil
	}
	src := clause.ChildByFieldName("source")
	if src == nil {
		return nil
	}
	path := stringValue(c.p.text(src))
	if path == "" {
		return nil
	}

	imp := &node.Import{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		Path:     path,
		Alias:    c.p.text(firstChildOfKind(clause, kindIdentifier)),
	}
	c.attachDocs(&imp.BaseNode, n)
	typescript.MetaModuleSpecifier.SetAt(
		imp.EnsureMeta(), path, meta.AuthorityPlugin, FrontendName, c.p.posAt(n),
	)
	return imp
}

// reExportDecl converts `export { X } from './y'` and
// `export * from './y'` into a [node.Import].
//
// A re-export is a dependency and a contribution to this module's own
// public surface at once. Modelling it as an Import records the
// first; [typescript.MetaReExport] and [typescript.MetaReExportNames]
// record the second.
//
// A barrel file — an `index.ts` that re-exports a directory — is
// built entirely from these. Treating a re-export as declaring
// nothing, which is what this frontend did before, meant reporting
// that such a file declares nothing at all, when in fact it declares
// the package's entire API.
func (c *conv) reExportDecl(n *ts.Node) *node.Import {
	src := n.ChildByFieldName("source")
	if src == nil {
		// `export { a }` with no `from` re-exports a local binding.
		// It names no module, so there is no dependency to record and
		// the declaration it forwards is already in the graph.
		return nil
	}
	path := stringValue(c.p.text(src))
	if path == "" {
		return nil
	}

	imp := &node.Import{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		Path:     path,
		Alias:    c.starExportAlias(n),
	}
	c.attachDocs(&imp.BaseNode, n)

	bag := imp.EnsureMeta()
	pos := c.p.posAt(n)
	typescript.MetaModuleSpecifier.SetAt(bag, path, meta.AuthorityPlugin, FrontendName, pos)
	typescript.MetaReExport.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	if hasToken(n, "type") {
		typescript.MetaTypeOnly.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if names := c.exportedNames(n); len(names) > 0 {
		typescript.MetaReExportNames.SetAt(bag, names, meta.AuthorityPlugin, FrontendName, pos)
	}
	return imp
}

// exportedNames lists the names a re-export forwards, each as
// written, so `Y as Z` is preserved whole.
//
// Empty for `export * from './y'`, which forwards everything the
// target module exports — a set this frontend cannot enumerate
// without resolving the module, and which would be wrong to record
// as "no names".
func (c *conv) exportedNames(n *ts.Node) []string {
	clause := firstChildOfKind(n, "export_clause")
	if clause == nil {
		return nil
	}
	out := make([]string, 0, clause.NamedChildCount())
	for i := range clause.NamedChildCount() {
		if spec := clause.NamedChild(i); spec.Kind() == "export_specifier" {
			out = append(out, c.p.text(spec))
		}
	}
	return out
}

// starExportAlias returns the local name of a namespaced star
// re-export — the `NS` of `export * as NS from './y'`.
func (c *conv) starExportAlias(n *ts.Node) string {
	if ns := firstChildOfKind(n, "namespace_export"); ns != nil {
		return c.p.text(ns.NamedChild(0))
	}
	return ""
}

// importAlias returns the local name a whole-module import binds —
// the `D` of `import D from './d'` or the `NS` of
// `import * as NS from './ns'`.
//
// Named imports (`import { X, Y as Z }`) bind several names and none
// of them is the module's, so they leave Alias empty. The model has
// one alias per import, and picking one of several would misreport
// the others.
func (c *conv) importAlias(n *ts.Node) string {
	clause := firstChildOfKind(n, "import_clause")
	if clause == nil {
		return ""
	}
	for i := range clause.NamedChildCount() {
		child := clause.NamedChild(i)
		switch child.Kind() {
		case kindIdentifier:
			return c.p.text(child)
		case "namespace_import":
			return c.p.text(child.NamedChild(0))
		}
	}
	return ""
}

// firstChildOfKind returns n's first named child of the given kind.
func firstChildOfKind(n *ts.Node, kind string) *ts.Node {
	for i := range n.NamedChildCount() {
		if child := n.NamedChild(i); child.Kind() == kind {
			return child
		}
	}
	return nil
}

// stringValue strips the surrounding quotes from a string literal's
// source text.
func stringValue(text string) string {
	if len(text) < 2 {
		return ""
	}
	q := text[0]
	if q != '\'' && q != '"' && q != '`' {
		return text
	}
	return text[1 : len(text)-1]
}
