// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// field converts a `property_signature` (interface) or a
// `public_field_definition` (class) into a [node.Field].
//
// The two node kinds differ in what they may carry — only a class
// field has an initialiser or a `static` modifier — but both are one
// named member with a type, so they converge here rather than in two
// near-identical converters.
func (c *conv) field(n *ts.Node, depth int) *node.Field {
	nameNode := n.ChildByFieldName("name")
	name := c.propertyName(nameNode)
	if name == "" {
		return nil
	}

	f := &node.Field{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		Name:     name,
		Type:     c.typeRef(annotatedType(n), depth),
	}
	c.attachDocs(&f.BaseNode, n)
	c.stampDecorators(&f.BaseNode, n)
	c.memberModifiers(f.EnsureMeta(), n, nameNode)

	// `name!: string` asserts the field is initialised somewhere the
	// compiler cannot see. It is neither optional nor initialised, so
	// without this the two absences read as a plain required field.
	if hasToken(n, "!") {
		typescript.MetaDefiniteAssignment.SetAt(
			f.EnsureMeta(), true, meta.AuthorityPlugin, FrontendName, c.p.posAt(n),
		)
	}

	if v := n.ChildByFieldName("value"); v != nil {
		typescript.MetaInitialiser.SetAt(
			f.EnsureMeta(), c.p.text(v), meta.AuthorityPlugin, FrontendName, c.p.posAt(v),
		)
	}
	return f
}

// method converts a `method_signature` (interface) or a
// `method_definition` (class) into a [node.Method].
//
// Receiver stays nil for both. TypeScript has no receiver in a
// signature — `this` is implicit, and a `this` parameter is a type
// annotation rather than a binding — so the model's receiver fields
// describe nothing here.
func (c *conv) method(n *ts.Node) *node.Method {
	nameNode := n.ChildByFieldName("name")
	name := c.propertyName(nameNode)
	if name == "" {
		return nil
	}

	m := &node.Method{
		BaseNode:   node.BaseNode{SourcePos: c.p.posAt(n)},
		Name:       name,
		Params:     c.params(n.ChildByFieldName("parameters")),
		TypeParams: c.typeParams(n.ChildByFieldName("type_parameters")),
	}
	c.attachDocs(&m.BaseNode, n)
	c.stampDecorators(&m.BaseNode, n)
	ownTypeParams(m.TypeParams, m)
	for _, p := range m.Params {
		p.Owner = m
	}

	// returnType, not annotatedType: a callable's return annotation is
	// under `return_type` while a property's or parameter's type is
	// under `type`. Reading the wrong field drops every method's
	// return type silently — the method converts, its signature is
	// just wrong.
	if ret := c.typeRef(returnType(n), maxTypeDepth); ret != nil {
		m.Returns = []*node.Return{{
			BaseNode: node.BaseNode{SourcePos: ret.Pos()},
			Type:     ret,
			Owner:    m,
		}}
	}

	c.memberModifiers(m.EnsureMeta(), n, nameNode)
	c.callableModifiers(m.EnsureMeta(), n)
	c.methodSignatures[m] = signature{
		text:    c.signatureText(n),
		hasBody: n.ChildByFieldName("body") != nil,
	}
	return m
}

// params converts a `formal_parameters` node.
func (c *conv) params(n *ts.Node) []*node.Param {
	if n == nil {
		return nil
	}
	out := make([]*node.Param, 0, n.NamedChildCount())
	for i := range n.NamedChildCount() {
		if p := c.param(n.NamedChild(i)); p != nil {
			out = append(out, p)
		}
	}
	return out
}

// param converts one parameter.
//
// A destructuring parameter — `function f({ a, b }: Opts)` — binds no
// single name. It is kept with an empty Name rather than dropped,
// because dropping it would shift every later parameter's index and
// silently change the signature a generator sees.
func (c *conv) param(n *ts.Node) *node.Param {
	switch n.Kind() {
	case kindRequiredParam, kindOptionalParam:
	default:
		return nil
	}

	pattern := n.ChildByFieldName("pattern")
	variadic := strings.HasPrefix(strings.TrimSpace(c.p.text(n)), "...")
	p := &node.Param{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		Name:     c.bindingName(pattern),
		Type:     variadicElem(c.typeRef(annotatedType(n), maxTypeDepth), variadic),
		Variadic: variadic,
	}

	c.stampDecorators(&p.BaseNode, n)

	bag := p.EnsureMeta()
	pos := c.p.posAt(n)
	if n.Kind() == kindOptionalParam {
		typescript.MetaOptional.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	c.accessModifiers(bag, n, pos)

	// An access modifier in a parameter list is what makes a
	// parameter also declare a field. Nothing else does, so the
	// presence of one is the whole test.
	if typescript.MetaVisibility.Has(bag) || typescript.MetaReadonly.Has(bag) {
		typescript.MetaParameterProperty.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	return p
}

// variadicElem unwraps a rest parameter's declared type to the type
// of one argument.
//
// [node.Param] documents Variadic as carrying the *element* type:
// Go's `...int` records `int`, not `[]int`. TypeScript annotates the
// rest parameter with the array — `...rest: string[]` — so the
// annotation sits one level out from what the model asks for, and
// leaving it there makes a consumer that follows the documented
// contract emit `...rest: string[][]`.
//
// A rest parameter annotated with something that is not an array — a
// tuple, or a generic that resolves to one — is left as written.
// There is no element to take, and inventing one would be worse than
// reporting the type the author declared.
func variadicElem(t *node.TypeRef, variadic bool) *node.TypeRef {
	if !variadic || t == nil || !t.IsSlice() || t.Elem == nil {
		return t
	}
	return t.Elem
}

// bindingName returns the identifier a parameter pattern binds, or
// empty for a destructuring pattern.
func (c *conv) bindingName(pattern *ts.Node) string {
	if pattern == nil {
		return ""
	}
	switch pattern.Kind() {
	case kindIdentifier, "shorthand_property_identifier_pattern":
		return c.p.text(pattern)
	case "rest_pattern":
		return c.bindingName(pattern.NamedChild(0))
	default:
		return ""
	}
}

// propertyName returns a member's name.
//
// Four spellings reach here: a plain identifier, a `#private` name, a
// quoted string key, and a computed `[expr]` key. The first three
// name a member statically; a computed key does not, and is reported
// as unnamed rather than given the expression as its name.
func (c *conv) propertyName(n *ts.Node) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "property_identifier", kindIdentifier, "type_identifier",
		"private_property_identifier":
		return c.p.text(n)
	case "string":
		return strings.Trim(c.p.text(n), `'"`)
	case "number":
		return c.p.text(n)
	default:
		return ""
	}
}

// memberModifiers stamps the modifiers a class or interface member
// may carry.
func (c *conv) memberModifiers(bag *meta.Bag, n, nameNode *ts.Node) {
	pos := c.p.posAt(n)

	if hasToken(n, "?") {
		typescript.MetaOptional.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if hasToken(n, "static") {
		typescript.MetaStatic.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if hasToken(n, "abstract") {
		typescript.MetaAbstract.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	c.accessModifiers(bag, n, pos)

	// A `#name` field is private at runtime, which `private` is not.
	// Checked on the name node because the grammar spells it as the
	// identifier's kind rather than as a modifier token.
	if nameNode != nil && nameNode.Kind() == "private_property_identifier" {
		typescript.MetaVisibility.SetAt(
			bag, typescript.VisibilityHard, meta.AuthorityPlugin, FrontendName, pos,
		)
	}
}

// accessModifiers stamps `readonly` and the three access keywords.
//
// Shared by members and by parameters, because a constructor
// parameter takes the same modifiers and means a member by taking
// them.
func (c *conv) accessModifiers(bag *meta.Bag, n *ts.Node, pos position.Pos) {
	if hasToken(n, "readonly") {
		typescript.MetaReadonly.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	// `public` / `private` / `protected` are one named
	// `accessibility_modifier` node carrying the keyword as its text,
	// not three anonymous tokens — so the keyword is read out of the
	// child rather than matched against the node's kind.
	if v := c.accessibilityModifier(n); v != "" {
		typescript.MetaVisibility.SetAt(bag, v, meta.AuthorityPlugin, FrontendName, pos)
	}
}

// accessibilityModifier returns the keyword of n's
// `accessibility_modifier` child, or empty when it has none.
func (c *conv) accessibilityModifier(n *ts.Node) string {
	for i := range n.NamedChildCount() {
		if child := n.NamedChild(i); child.Kind() == "accessibility_modifier" {
			return c.p.text(child)
		}
	}
	return ""
}

// callableModifiers stamps the modifiers only a function or method
// carries — async, generator, and accessor kind.
func (c *conv) callableModifiers(bag *meta.Bag, n *ts.Node) {
	pos := c.p.posAt(n)

	if hasToken(n, "async") {
		typescript.MetaAsync.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if hasToken(n, "*") {
		typescript.MetaGenerator.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	for _, a := range []string{typescript.AccessorGet, typescript.AccessorSet} {
		if hasToken(n, a) {
			typescript.MetaAccessor.SetAt(bag, a, meta.AuthorityPlugin, FrontendName, pos)
			return
		}
	}
}
