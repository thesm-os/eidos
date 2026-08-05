// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

import "reflect"

// Visitor is the callback used by [Walk]. Visit is invoked once per
// node encountered during traversal. Returning a nil Visitor from
// Visit prunes the subtree under the visited node; returning the
// receiver (or another non-nil Visitor) continues descent.
//
// This mirrors the [go/ast.Visitor] convention so anyone familiar
// with the standard library's tree-walking pattern picks it up
// immediately.
type Visitor interface {
	Visit(n Node) Visitor
}

// VisitorFunc adapts an ordinary function into a [Visitor]. The
// function returns the next visitor for descending into the visited
// node's children, mirroring [Visitor.Visit].
type VisitorFunc func(n Node) Visitor

// Visit forwards to the underlying function.
func (f VisitorFunc) Visit(n Node) Visitor { return f(n) }

// Walk traverses the node graph rooted at n in declaration order.
// For every visited node, Walk calls v.Visit; the returned visitor
// (when non-nil) drives the descent into that node's children.
//
// The traversal order per kind:
//
//   - [Package]: visits Structs, Interfaces, Functions, Variables,
//     Constants, Enums, Aliases in source order.
//   - [Struct]: visits TypeParams, Fields, Embeds, Methods.
//   - [Interface]: visits TypeParams, Methods, Embeds.
//   - [Method] / [Function]: visits TypeParams, Params, Returns.
//   - [Field]: visits the Field's Type.
//   - [Param]: visits the Param's Type.
//   - [TypeParam]: visits every entry in the Constraint's Embedded
//     ref list when a Constraint is set. [Constraint] itself is not
//     a [Node].
//   - [Enum]: visits Underlying, then Variants.
//   - [Alias]: visits TypeParams, then Target.
//   - [Variable] / [Constant]: visits Type when present.
//   - [Embed]: visits the embedded Type.
//   - [TypeRef]: visits TypeArgs / Elem / MapKey / MapValue /
//     FuncParams / FuncReturns / inline Fields / Methods / Embeds
//     according to TypeKind.
//   - [EnumVariant]: leaf (no children).
//
// Walk is iterative-recursive in style; for very deep node graphs the
// caller's stack grows linearly with depth. For typical source
// programs that depth is bounded by source structure.
func Walk(n Node, v Visitor) {
	if v == nil || isNilNode(n) {
		return
	}
	w := v.Visit(n)
	if w == nil {
		return
	}
	walkChildren(n, w)
}

// isNilNode reports whether n is nil OR a non-nil interface wrapping a
// nil pointer (Go's typed-nil case). Walk uses this to handle the
// common pattern where callers pass an optional sub-node like
// [Variable.Type] without a manual guard.
func isNilNode(n Node) bool {
	if n == nil {
		return true
	}
	rv := reflect.ValueOf(n)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}

// walkChildren dispatches to the appropriate per-kind child walker.
// Unhandled or leaf nodes simply return without descent.
func walkChildren(n Node, v Visitor) {
	switch x := n.(type) {
	case *Package:
		walkPackage(x, v)
	case *File:
		walkFile(x, v)
	case *Struct:
		walkStruct(x, v)
	case *Interface:
		walkInterface(x, v)
	case *Method:
		walkMethod(x, v)
	case *Function:
		walkFunction(x, v)
	case *Field:
		Walk(x.Type, v)
	case *Param:
		Walk(x.Type, v)
	case *Return:
		Walk(x.Type, v)
	case *TypeParam:
		walkTypeParam(x, v)
	case *Enum:
		Walk(x.Underlying, v)
		for _, vt := range x.Variants {
			Walk(vt, v)
		}
	case *Alias:
		for _, tp := range x.TypeParams {
			Walk(tp, v)
		}
		Walk(x.Target, v)
	case *Variable:
		Walk(x.Type, v)
	case *Constant:
		Walk(x.Type, v)
	case *Embed:
		Walk(x.Type, v)
	case *TypeRef:
		walkTypeRef(x, v)
	}
}

func walkPackage(p *Package, v Visitor) {
	for _, f := range p.Files {
		Walk(f, v)
	}
	for _, imp := range p.Imports {
		Walk(imp, v)
	}
	for _, s := range p.Structs {
		Walk(s, v)
	}
	for _, i := range p.Interfaces {
		Walk(i, v)
	}
	for _, f := range p.Functions {
		Walk(f, v)
	}
	for _, vd := range p.Variables {
		Walk(vd, v)
	}
	for _, c := range p.Constants {
		Walk(c, v)
	}
	for _, e := range p.Enums {
		Walk(e, v)
	}
	for _, a := range p.Aliases {
		Walk(a, v)
	}
}

func walkFile(f *File, v Visitor) {
	for _, imp := range f.Imports {
		Walk(imp, v)
	}
}

func walkStruct(s *Struct, v Visitor) {
	for _, tp := range s.TypeParams {
		Walk(tp, v)
	}
	for _, f := range s.Fields {
		Walk(f, v)
	}
	for _, e := range s.Embeds {
		Walk(e, v)
	}
	for _, m := range s.Methods {
		Walk(m, v)
	}
}

func walkInterface(i *Interface, v Visitor) {
	for _, tp := range i.TypeParams {
		Walk(tp, v)
	}
	for _, m := range i.Methods {
		Walk(m, v)
	}
	for _, e := range i.Embeds {
		Walk(e, v)
	}
}

func walkMethod(m *Method, v Visitor) {
	for _, tp := range m.TypeParams {
		Walk(tp, v)
	}
	for _, p := range m.Params {
		Walk(p, v)
	}
	for _, r := range m.Returns {
		Walk(r, v)
	}
}

func walkFunction(f *Function, v Visitor) {
	for _, tp := range f.TypeParams {
		Walk(tp, v)
	}
	for _, p := range f.Params {
		Walk(p, v)
	}
	for _, r := range f.Returns {
		Walk(r, v)
	}
}

func walkTypeParam(tp *TypeParam, v Visitor) {
	if tp.Constraint == nil {
		return
	}
	for _, e := range tp.Constraint.Embedded {
		Walk(e, v)
	}
}

func walkTypeRef(r *TypeRef, v Visitor) {
	switch r.TypeKind {
	case TypeRefNamed:
		for _, t := range r.TypeArgs {
			Walk(t, v)
		}
	case TypeRefPointer, TypeRefSlice, TypeRefArray:
		Walk(r.Elem, v)
	case TypeRefMap:
		Walk(r.MapKey, v)
		Walk(r.MapValue, v)
	case TypeRefFunc:
		for _, p := range r.FuncParams {
			Walk(p, v)
		}
		for _, ret := range r.FuncReturns {
			Walk(ret, v)
		}
	case TypeRefAnonStruct:
		for _, f := range r.Fields {
			Walk(f, v)
		}
		for _, e := range r.Embeds {
			Walk(e, v)
		}
	case TypeRefAnonInterface:
		for _, m := range r.Methods {
			Walk(m, v)
		}
		for _, e := range r.Embeds {
			Walk(e, v)
		}
	case TypeRefTypeParam:
		// A use-site type-param reference is a leaf — there are no
		// further refs to descend into. The reference's Name
		// identifies which type parameter it points at; the
		// declaration itself is reachable via the enclosing decl.
	}
}
