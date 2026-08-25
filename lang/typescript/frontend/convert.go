// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// conv converts one parsed file into node declarations.
//
// One per file rather than one per run: the tree's byte offsets only
// mean anything against the buffer they came from, and the namespace
// stack is per-file state. Sharing a conv across files would pair a
// node with the wrong source.
type conv struct {
	// p is the parsed file being converted.
	p *parsed

	// pkg is the package path recorded on every declaration — the
	// file's module specifier.
	pkg string

	// ps receives per-declaration diagnostics. Never nil.
	ps *diag.PluginSink

	// parser is the run's shared directive parser, so every frontend
	// in a pipeline reads `+gen:` the same way. Never nil.
	parser *directive.Parser

	// namespace is the dotted path of enclosing `namespace` blocks,
	// maintained as the walk descends. Empty at file scope.
	namespace string

	// modifiers carries the flags an enclosing `export_statement` or
	// `ambient_declaration` contributes to the declaration inside it.
	modifiers modifiers

	// signatures and methodSignatures hold what the overload fold
	// needs and the graph does not carry. Populated as each callable
	// converts and read once, by the fold. See [signature].
	signatures       map[*node.Function]signature
	methodSignatures map[*node.Method]signature
}

// newConv returns a converter for one parsed file.
func newConv(p *parsed, pkg string, ps *diag.PluginSink, parser *directive.Parser) *conv {
	return &conv{
		p:                p,
		pkg:              pkg,
		ps:               ps,
		parser:           parser,
		signatures:       map[*node.Function]signature{},
		methodSignatures: map[*node.Method]signature{},
	}
}

// modifiers are the facts a wrapper node contributes to the
// declaration it wraps.
//
// Carried on the converter rather than passed down, because the
// wrappers nest — `export declare class C` is an export_statement
// around an ambient_declaration around the class — and a parameter
// would have to be threaded through every intermediate case that does
// not care.
type modifiers struct {
	exported      bool
	defaultExport bool
	ambient       bool

	// decorators are those a wrapper holds on the declaration's
	// behalf.
	//
	// The grammar puts a decorator in one of three places depending
	// on how the declaration was written. `@D class C {}` and
	// `export @D class C {}` both make it a child of the class;
	// `@D` on the line above `export class C {}` makes it a child of
	// the export statement instead. The third form is the one every
	// framework's documentation uses, so it is the common case rather
	// than an edge one.
	decorators []*ts.Node
}

// declarations walks a parsed file and returns every top-level
// declaration it contains, in source order.
func (c *conv) declarations(root *ts.Node) []node.Node {
	out := make([]node.Node, 0, root.NamedChildCount())
	for i := range root.NamedChildCount() {
		out = append(out, c.declaration(root.NamedChild(i))...)
	}
	// Overloads are several declarations of one callable, and the
	// model holds one declaration per name — so the collapse happens
	// once the whole file is converted, when every signature sharing
	// a name has been seen.
	return c.foldOverloads(out)
}

// declaration converts one statement, which may yield several nodes:
// a namespace flattens to its members, and a `const` declaring more
// than one binding yields one constant apiece.
func (c *conv) declaration(n *ts.Node) []node.Node {
	switch n.Kind() {
	case "export_statement":
		return c.exportStatement(n)

	case "ambient_declaration":
		return c.withModifiers(func(m *modifiers) { m.ambient = true }, n)

	case "internal_module", "module":
		return c.namespaceDecl(n)

	case "statement_block":
		// The body of `declare global { … }`. The block is a scope in
		// JavaScript but declares nothing itself, so its members are
		// the declarations and the wrapper contributes only the
		// ambient marker its parent already set.
		return c.childDeclarations(n)

	case "expression_statement":
		// A `namespace X { … }` at file scope parses as an expression
		// statement wrapping the module, because the grammar admits
		// the same syntax in expression position. Descending is what
		// makes a top-level namespace reachable; a statement wrapping
		// anything else yields nothing from the dispatch below.
		return c.childDeclarations(n)

	case "interface_declaration":
		if i := c.interfaceDecl(n); i != nil {
			return []node.Node{i}
		}

	case "type_alias_declaration":
		if a := c.aliasDecl(n); a != nil {
			return []node.Node{a}
		}

	case "enum_declaration":
		if e := c.enumDecl(n); e != nil {
			return []node.Node{e}
		}

	case "function_declaration", "function_signature", "generator_function_declaration":
		if f := c.functionDecl(n); f != nil {
			return []node.Node{f}
		}

	case "lexical_declaration", "variable_declaration":
		return c.bindingDecl(n)

	case "import_statement":
		if i := c.importDecl(n); i != nil {
			return []node.Node{i}
		}

	case "class_declaration", "abstract_class_declaration", "class":
		// `class` rather than `class_declaration` is what an anonymous
		// `export default class {}` produces. It reaches structDecl so
		// the missing name is reported there, once, rather than being
		// dropped by a dispatch that never matched it.
		if s := c.structDecl(n); s != nil {
			return []node.Node{s}
		}
	}
	return nil
}

// exportStatement unwraps `export …` and `export default …`, and
// converts the re-export forms.
//
// A re-export names bindings that belong to another module, so it
// declares nothing itself — but it does introduce a dependency and it
// does contribute to this module's public surface, which is what
// [conv.reExportDecl] records.
func (c *conv) exportStatement(n *ts.Node) []node.Node {
	if n.ChildByFieldName("source") != nil {
		if imp := c.reExportDecl(n); imp != nil {
			return []node.Node{imp}
		}
		return nil
	}
	carried := decoratorChildren(n)
	return c.withModifiers(func(m *modifiers) {
		m.exported = true
		m.defaultExport = hasToken(n, "default")
		m.decorators = append(m.decorators, carried...)
	}, n)
}

// decoratorChildren returns n's immediate decorator children.
func decoratorChildren(n *ts.Node) []*ts.Node {
	var out []*ts.Node
	for i := range n.NamedChildCount() {
		if d := n.NamedChild(i); d.Kind() == kindDecorator {
			out = append(out, d)
		}
	}
	return out
}

// withModifiers applies mutate to the converter's modifier state,
// converts n's inner declaration under it, and restores the state.
func (c *conv) withModifiers(mutate func(*modifiers), n *ts.Node) []node.Node {
	saved := c.modifiers
	mutate(&c.modifiers)
	defer func() { c.modifiers = saved }()

	out := make([]node.Node, 0, n.NamedChildCount())
	for i := range n.NamedChildCount() {
		out = append(out, c.declaration(n.NamedChild(i))...)
	}
	return out
}

// namespaceDecl flattens a `namespace N { … }` into its members.
//
// The model has no namespace kind and adding one would put a
// TypeScript container in the package every language shares. Members
// are hoisted to package scope with the dotted path recorded on
// [typescript.MetaNamespace], which is what a use site needs to spell
// the qualifier.
func (c *conv) namespaceDecl(n *ts.Node) []node.Node {
	// `declare module 'ext'` names the module with a string literal
	// while `namespace N` names it with an identifier. Both arrive
	// under the same field, so the quotes come off here rather than
	// leaving a namespace path spelled with them.
	name := stringValue(c.p.text(n.ChildByFieldName("name")))
	body := n.ChildByFieldName("body")
	if body == nil {
		return nil
	}

	saved := c.namespace
	c.namespace = joinNamespace(saved, name)
	defer func() { c.namespace = saved }()

	out := make([]node.Node, 0, body.NamedChildCount())
	for i := range body.NamedChildCount() {
		out = append(out, c.declaration(body.NamedChild(i))...)
	}
	return out
}

// childDeclarations converts every named child of n as a declaration.
func (c *conv) childDeclarations(n *ts.Node) []node.Node {
	out := make([]node.Node, 0, n.NamedChildCount())
	for i := range n.NamedChildCount() {
		out = append(out, c.declaration(n.NamedChild(i))...)
	}
	return out
}

// joinNamespace appends one segment to a dotted namespace path.
func joinNamespace(prefix, segment string) string {
	switch {
	case segment == "":
		return prefix
	case prefix == "":
		return segment
	default:
		return prefix + "." + segment
	}
}

// stampDecl applies the converter's current modifier and namespace
// state to a freshly-built declaration.
//
// Every declaration goes through here, so a fact contributed by a
// wrapper is recorded in one place rather than at each kind's own
// construction site.
func (c *conv) stampDecl(base *node.BaseNode, n *ts.Node) {
	pos := c.p.posAt(n)
	bag := base.EnsureMeta()

	if c.modifiers.exported {
		typescript.MetaExported.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if c.modifiers.defaultExport {
		typescript.MetaDefaultExport.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if c.modifiers.ambient {
		typescript.MetaAmbient.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if c.namespace != "" {
		typescript.MetaNamespace.SetAt(bag, c.namespace, meta.AuthorityPlugin, FrontendName, pos)
	}

	// Decorators a wrapper carried down, in addition to any the
	// declaration holds itself.
	c.stampDecoratorNodes(base, c.modifiers.decorators)
}

// hasToken reports whether n has an immediate child that is the given
// anonymous token — `default`, `abstract`, `static`, and the other
// keywords the grammar does not surface as named nodes.
func hasToken(n *ts.Node, token string) bool {
	if n == nil {
		return false
	}
	for i := range n.ChildCount() {
		if n.Child(i).Kind() == token {
			return true
		}
	}
	return false
}
