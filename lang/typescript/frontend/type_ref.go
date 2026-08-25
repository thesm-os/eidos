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

// maxTypeDepth bounds the type-expression walk.
//
// Type expressions nest — `Promise<Array<Map<string, T>>>` is four
// levels — and a malformed tree could in principle cycle. Sixteen is
// past anything written by hand and shallow enough that a pathological
// input fails one declaration rather than the process.
const maxTypeDepth = 16

// typeRef converts a tree-sitter type node into a [node.TypeRef].
//
// Returns nil for a nil node or an exhausted depth budget, which every
// caller must tolerate: a type is optional throughout the model
// (`Field.Type`, `Param.Type`, `Constant.Type` are all nillable) and a
// TypeScript declaration may genuinely carry none.
func (c *conv) typeRef(n *ts.Node, depth int) *node.TypeRef {
	if n == nil || depth <= 0 {
		return nil
	}

	switch n.Kind() {
	case "type_identifier":
		return c.namedRef(n, c.p.text(n), "")

	case "predefined_type":
		// `string`, `number`, `boolean`, `any`, `void`, `never`,
		// `unknown`, `symbol`, `object`. Package stays empty, which is
		// what [node.TypeRef.IsBuiltin] keys on.
		return c.namedRef(n, c.p.text(n), "")

	case "nested_type_identifier":
		return c.nestedRef(n)

	case "generic_type":
		return c.genericRef(n, depth)

	case "array_type":
		return c.sliceRef(n, depth)

	case "parenthesized_type":
		// Parentheses group; they carry no meaning the model records.
		// Unwrapping without spending depth would let `((((T))))`
		// recurse freely, so the budget is charged.
		return c.typeRef(n.NamedChild(0), depth-1)

	case "readonly_type":
		return c.readonlyRef(n, depth)

	case "union_type":
		return c.markerRef(n, typescript.RefUnion, depth)

	case "intersection_type":
		return c.markerRef(n, typescript.RefIntersection, depth)

	case "tuple_type":
		return c.tupleRef(n, depth)

	case "literal_type":
		return c.literalRef(n)

	case "function_type":
		return c.funcRef(n, depth, false)

	case "constructor_type":
		return c.funcRef(n, depth, true)

	case "object_type":
		return c.objectRef(n, depth)

	default:
		// Conditional, mapped-in-type-position, keyof, typeof, lookup,
		// infer and template-literal types, plus anything the grammar
		// grows later. All carried as text — see
		// [typescript.RefOperator].
		return c.operatorRef(n)
	}
}

// namedRef builds a Named ref, positioned at n.
func (c *conv) namedRef(n *ts.Node, name, pkg string) *node.TypeRef {
	return &node.TypeRef{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		TypeKind: node.TypeRefNamed,
		Name:     name,
		Package:  pkg,
	}
}

// nestedRef converts a dotted type name — `ns.Other`, `A.B.C`.
//
// The trailing identifier is the type; everything before it is the
// qualifier. A qualifier is a namespace or an import alias, not a
// module specifier, so it is recorded verbatim rather than resolved:
// resolving it needs the file's import block, and a type may name a
// namespace that was never imported at all.
func (c *conv) nestedRef(n *ts.Node) *node.TypeRef {
	full := c.p.text(n)
	name, pkg := full, ""
	if i := strings.LastIndex(full, "."); i >= 0 {
		name, pkg = full[i+1:], full[:i]
	}
	return c.namedRef(n, name, pkg)
}

// genericRef converts an instantiation — `Array<T>`, `A.B<C>`.
func (c *conv) genericRef(n *ts.Node, depth int) *node.TypeRef {
	name := n.ChildByFieldName("name")
	ref := c.typeRef(name, depth-1)
	if ref == nil {
		return nil
	}
	// The ref carries the instantiated name's position, but the whole
	// expression is what a diagnostic should point at.
	ref.SourcePos = c.p.posAt(n)
	if args := n.ChildByFieldName("type_arguments"); args != nil {
		ref.TypeArgs = c.typeList(args, depth-1)
	}
	return ref
}

// sliceRef converts `T[]`.
func (c *conv) sliceRef(n *ts.Node, depth int) *node.TypeRef {
	return &node.TypeRef{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		TypeKind: node.TypeRefSlice,
		Elem:     c.typeRef(n.NamedChild(0), depth-1),
	}
}

// readonlyRef converts `readonly T[]`, stamping the modifier on the
// element type it wraps rather than introducing a wrapper ref.
//
// `readonly` constrains the binding, not the type: `readonly T[]` and
// `T[]` are the same array of the same element. Modelling it as a ref
// of its own would make every consumer unwrap one more layer to reach
// a type it already understands.
func (c *conv) readonlyRef(n *ts.Node, depth int) *node.TypeRef {
	inner := c.typeRef(n.NamedChild(0), depth-1)
	if inner == nil {
		return nil
	}
	typescript.MetaReadonly.SetAt(
		inner.EnsureMeta(), true, meta.AuthorityPlugin, FrontendName, c.p.posAt(n),
	)
	return inner
}

// markerRef builds one of the structural markers — union or
// intersection — carrying its members on TypeArgs.
func (c *conv) markerRef(n *ts.Node, marker string, depth int) *node.TypeRef {
	ref := &node.TypeRef{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		TypeKind: node.TypeRefNamed,
		Package:  typescript.RefPackage,
		Name:     marker,
		TypeArgs: c.flatten(n, n.Kind(), depth-1),
	}
	if marker == typescript.RefUnion && unionIsNullable(ref.TypeArgs) {
		typescript.MetaNullable.SetAt(
			ref.EnsureMeta(), true, meta.AuthorityPlugin, FrontendName, c.p.posAt(n),
		)
	}
	return ref
}

// flatten collects the members of a union or intersection, splicing
// in the members of any nested node of the same kind.
//
// The grammar parses `A | B | C` left-associatively, as
// `(A | B) | C` — two nodes, not one with three children. Converting
// that shape directly would give the outer union two members, one of
// which is another union, so a consumer counting members of a
// three-way union would get two. Flattening restores the shape the
// source reads as.
//
// Only same-kind nesting is spliced: `A | (B & C)` keeps the
// intersection as one member, which is what it is.
func (c *conv) flatten(n *ts.Node, kind string, depth int) []*node.TypeRef {
	var out []*node.TypeRef
	for i := range n.NamedChildCount() {
		child := n.NamedChild(i)
		if child.Kind() == kind {
			out = append(out, c.flatten(child, kind, depth)...)
			continue
		}
		if t := c.typeRef(child, depth); t != nil {
			out = append(out, t)
		}
	}
	return out
}

// unionIsNullable reports whether any member is the `null` or
// `undefined` literal type.
func unionIsNullable(members []*node.TypeRef) bool {
	for _, m := range members {
		if m == nil {
			continue
		}
		lit, ok := typescript.MetaLiteralType.Get(m.Meta())
		if ok && (lit == "null" || lit == "undefined") {
			return true
		}
	}
	return false
}

// tupleRef converts `[A, B]` and its labelled form
// `[a: string, b?: number, ...r: T[]]`.
//
// Labelled elements arrive as parameter nodes rather than bare types,
// so the element type is one level further down and the label's `?`
// and `...` are modifiers on the parameter. Unlabelled elements are
// the type directly. Both forms produce the same TypeArgs shape.
func (c *conv) tupleRef(n *ts.Node, depth int) *node.TypeRef {
	ref := &node.TypeRef{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		TypeKind: node.TypeRefNamed,
		Package:  typescript.RefPackage,
		Name:     typescript.RefTuple,
	}
	for i := range n.NamedChildCount() {
		child := n.NamedChild(i)
		elem, optional, rest := c.tupleElem(child, depth-1)
		if elem == nil {
			continue
		}
		bag := elem.EnsureMeta()
		if optional {
			typescript.MetaOptional.SetAt(
				bag, true, meta.AuthorityPlugin, FrontendName, c.p.posAt(child),
			)
		}
		if rest {
			typescript.MetaRest.SetAt(
				bag, true, meta.AuthorityPlugin, FrontendName, c.p.posAt(child),
			)
		}
		ref.TypeArgs = append(ref.TypeArgs, elem)
	}
	return ref
}

// tupleElem resolves one tuple element to its type plus the two
// modifiers a labelled element may carry.
func (c *conv) tupleElem(n *ts.Node, depth int) (elem *node.TypeRef, optional, rest bool) {
	switch n.Kind() {
	case kindRequiredParam, kindOptionalParam:
		optional = n.Kind() == kindOptionalParam
		rest = strings.HasPrefix(strings.TrimSpace(c.p.text(n)), "...")
		return c.typeRef(annotatedType(n), depth), optional, rest
	default:
		return c.typeRef(n, depth), false, false
	}
}

// literalRef converts a literal used in type position — `'a'`, `42`,
// `true`, `null`, `undefined`.
//
// Modelled as a Named ref whose Name is the literal's source text,
// with the same text on [typescript.MetaLiteralType]. The name makes
// it printable by anything that renders a Named ref; the key is what
// lets a consumer tell `type A = 'x'` from a type actually called `x`.
func (c *conv) literalRef(n *ts.Node) *node.TypeRef {
	text := c.p.text(n)
	ref := c.namedRef(n, text, "")
	typescript.MetaLiteralType.SetAt(
		ref.EnsureMeta(), text, meta.AuthorityPlugin, FrontendName, c.p.posAt(n),
	)
	return ref
}

// funcRef converts a function type `(a: T) => U` and a constructor
// type `new (a: T) => U`, which differ only by a marker.
func (c *conv) funcRef(n *ts.Node, depth int, constructor bool) *node.TypeRef {
	ref := &node.TypeRef{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		TypeKind: node.TypeRefFunc,
	}
	if params := n.ChildByFieldName("parameters"); params != nil {
		for i := range params.NamedChildCount() {
			if t := c.typeRef(annotatedType(params.NamedChild(i)), depth-1); t != nil {
				ref.FuncParams = append(ref.FuncParams, t)
			}
		}
	}
	if ret := n.ChildByFieldName("return_type"); ret != nil {
		if t := c.typeRef(ret, depth-1); t != nil {
			ref.FuncReturns = append(ref.FuncReturns, t)
		}
	}
	if constructor {
		typescript.MetaConstructor.SetAt(
			ref.EnsureMeta(), true, meta.AuthorityPlugin, FrontendName, c.p.posAt(n),
		)
	}
	return ref
}

// objectRef converts an inline `{ … }` type into an anonymous struct
// ref, carrying its property signatures as fields.
//
// A mapped type is the same grammar node and is not a struct — it has
// no fixed members at all — so it is marked and carried as text
// instead of being flattened into fields it does not have.
func (c *conv) objectRef(n *ts.Node, depth int) *node.TypeRef {
	if mappedType(n) {
		ref := c.operatorRef(n)
		typescript.MetaMapped.SetAt(
			ref.EnsureMeta(), true, meta.AuthorityPlugin, FrontendName, c.p.posAt(n),
		)
		return ref
	}

	// An object type that is nothing but an index signature is an
	// associative container, which the model has a variant for.
	// Rendering it as a struct with no fields would say the type has
	// no members, when what it says is that any key of one type maps
	// to a value of another.
	//
	// An object carrying fields as well keeps being a struct: it does
	// have those members, and the signature describes what else is
	// admitted alongside them.
	if idx := soleIndexSignature(n); idx != nil {
		return c.mapRef(n, idx, depth)
	}

	ref := &node.TypeRef{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		TypeKind: node.TypeRefAnonStruct,
	}
	for i := range n.NamedChildCount() {
		member := n.NamedChild(i)
		switch member.Kind() {
		case "property_signature":
			if f := c.field(member, depth-1); f != nil {
				f.Owner = ref
				ref.Fields = append(ref.Fields, f)
			}
		case kindIndexSignature:
			typescript.MetaIndexSignature.SetAt(
				ref.EnsureMeta(), c.p.text(member),
				meta.AuthorityPlugin, FrontendName, c.p.posAt(member),
			)
		}
	}
	return ref
}

// soleIndexSignature returns n's index signature when that is the
// only member it has, and nil otherwise.
func soleIndexSignature(n *ts.Node) *ts.Node {
	if n.NamedChildCount() != 1 {
		return nil
	}
	if idx := n.NamedChild(0); idx.Kind() == kindIndexSignature {
		return idx
	}
	return nil
}

// mapRef converts an index-signature object type into a Map ref.
//
// The signature's `index_type` is the key and its annotation is the
// value, which is exactly what [node.TypeRef]'s MapKey and MapValue
// hold.
func (c *conv) mapRef(n, idx *ts.Node, depth int) *node.TypeRef {
	ref := &node.TypeRef{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		TypeKind: node.TypeRefMap,
		MapKey:   c.typeRef(idx.ChildByFieldName("index_type"), depth-1),
		MapValue: c.typeRef(annotatedType(idx), depth-1),
	}
	if hasToken(idx, "readonly") {
		typescript.MetaReadonly.SetAt(
			ref.EnsureMeta(), true, meta.AuthorityPlugin, FrontendName, c.p.posAt(idx),
		)
	}
	return ref
}

// operatorRef builds the text-carrying marker for a type expression
// with no structured form.
func (c *conv) operatorRef(n *ts.Node) *node.TypeRef {
	ref := &node.TypeRef{
		BaseNode: node.BaseNode{SourcePos: c.p.posAt(n)},
		TypeKind: node.TypeRefNamed,
		Package:  typescript.RefPackage,
		Name:     typescript.RefOperator,
	}
	typescript.MetaTypeText.SetAt(
		ref.EnsureMeta(), c.p.text(n), meta.AuthorityPlugin, FrontendName, c.p.posAt(n),
	)
	return ref
}

// typeList converts the members of a `type_arguments` node.
func (c *conv) typeList(n *ts.Node, depth int) []*node.TypeRef {
	return c.namedChildTypes(n, depth)
}

// namedChildTypes converts every named child of n as a type,
// dropping those that convert to nothing.
func (c *conv) namedChildTypes(n *ts.Node, depth int) []*node.TypeRef {
	out := make([]*node.TypeRef, 0, n.NamedChildCount())
	for i := range n.NamedChildCount() {
		if t := c.typeRef(n.NamedChild(i), depth); t != nil {
			out = append(out, t)
		}
	}
	return out
}

// annotatedType returns the type inside a `: T` annotation on n, or
// nil when n carries none.
//
// Parameters, properties and variables all hold their type behind a
// `type_annotation` wrapper rather than as a direct child, and an
// annotation is optional on every one of them.
func annotatedType(n *ts.Node) *ts.Node {
	if n == nil {
		return nil
	}
	ann := n.ChildByFieldName("type")
	if ann == nil {
		return nil
	}
	if ann.Kind() == "type_annotation" {
		return ann.NamedChild(0)
	}
	return ann
}

// mappedType reports whether an `object_type` is a mapped type.
//
// `{ [k: string]: T }` and `{ [K in keyof T]: T[K] }` are both an
// `index_signature` inside an `object_type`, so the outer shape does
// not separate them. The inner one does: a mapped type's brackets
// hold a `mapped_type_clause`, and a plain index signature's hold a
// name and an `index_type`.
func mappedType(n *ts.Node) bool {
	if n.NamedChildCount() != 1 {
		return false
	}
	idx := n.NamedChild(0)
	if idx.Kind() != kindIndexSignature {
		return false
	}
	return firstChildOfKind(idx, "mapped_type_clause") != nil
}
