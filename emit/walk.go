// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import (
	"reflect"
	"slices"
)

// Visitor is the callback used by [Walk]. Visit is invoked once per
// node encountered during traversal. Returning a nil Visitor from
// Visit prunes the subtree under the visited node; returning the
// receiver (or another non-nil Visitor) continues descent.
//
// This mirrors the [go/ast.Visitor] convention so anyone familiar
// with the standard library's tree-walking pattern picks it up
// immediately. It also mirrors the [node.Visitor] convention so a
// caller can reuse the same VisitorFunc shape for both trees.
type Visitor interface {
	Visit(n Node) Visitor
}

// VisitorFunc adapts an ordinary function into a [Visitor]. The
// function returns the next visitor for descending into the visited
// node's children, mirroring [Visitor.Visit].
type VisitorFunc func(n Node) Visitor

// Visit forwards to the underlying function.
func (f VisitorFunc) Visit(n Node) Visitor { return f(n) }

// Walk traverses the emit graph rooted at n in declaration order.
// For every visited node, Walk calls v.Visit; the returned visitor
// (when non-nil) drives the descent into that node's children.
//
// Cross-tree links — [Slot.Owner], [Param.Owner], [Field.Owner],
// [Method.Owner], [TypeParam.Owner], [Import.Owner], [Enum]
// variants' Owner, and [TypeRef.Target] — are NOT followed; only
// downward composition relationships are traversed. This avoids
// re-walking the tree from arbitrary nodes and prevents cycles.
//
// Per-kind child order:
//
//   - [Package]: Files, Imports, Structs, Interfaces, Functions,
//     Variables, Constants, Enums, Aliases, package-level slots.
//   - [File]: Imports, file-level slots.
//   - [Struct]: TypeParams, Fields, Embeds, Methods, slots.
//   - [Interface]: TypeParams, Methods, Embeds, slots.
//   - [Method] / [Function]: TypeParams, Receiver (Method only),
//     Params, Returns, Body, slots.
//   - [Field]: Type, slots.
//   - [Param]: Type.
//   - [TypeParam]: Constraint.
//   - [Enum]: Underlying, Variants, slots.
//   - [EnumVariant]: Value.
//   - [Alias]: TypeParams, Target.
//   - [Variable] / [Constant]: Type, Init/Value.
//   - [Embed]: Type.
//   - [Import]: leaf.
//   - [TypeRef]: TypeArgs (Target is a cross-tree link and not
//     followed).
//   - [ExternalRef]: TypeArgs.
//   - [BuiltinRef]: leaf.
//   - [CompositeRef]: Elem / MapKey / MapValue / FuncParams /
//     FuncReturns / UnionTerms according to Shape.
//   - [Slot]: Items.
//   - [Stmt]: per-variant; see [walkStmt].
//   - [Expr]: per-variant; see [walkExpr].
//
// Walk is iterative-recursive in style; for very deep emit graphs
// the caller's stack grows linearly with depth.
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
// [Variable.Init] or [TypeParam.Constraint] without a manual guard.
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
		walkField(x, v)
	case *Param:
		Walk(x.Type, v)
	case *TypeParam:
		walkTypeParam(x, v)
	case *Enum:
		walkEnum(x, v)
	case *EnumVariant:
		Walk(x.Value, v)
	case *Alias:
		walkAlias(x, v)
	case *Variable:
		Walk(x.Type, v)
		Walk(x.Init, v)
	case *Constant:
		Walk(x.Type, v)
		Walk(x.Value, v)
	case *Embed:
		Walk(x.Type, v)
	case *TypeRef:
		for _, t := range x.TypeArgs {
			Walk(t, v)
		}
	case *ExternalRef:
		for _, t := range x.TypeArgs {
			Walk(t, v)
		}
	case *CompositeRef:
		walkCompositeRef(x, v)
	case *Slot:
		for _, it := range x.Items {
			Walk(it, v)
		}
	case *Stmt:
		walkStmt(x, v)
	case *Expr:
		walkExpr(x, v)
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
	walkSlots(p.SlotsByName(), v)
}

func walkFile(f *File, v Visitor) {
	for _, imp := range f.Imports {
		Walk(imp, v)
	}
	walkSlots(f.SlotsByName(), v)
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
	walkSlots(s.SlotsByName(), v)
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
	walkSlots(i.SlotsByName(), v)
}

func walkMethod(m *Method, v Visitor) {
	for _, tp := range m.TypeParams {
		Walk(tp, v)
	}
	Walk(m.Receiver, v)
	for _, p := range m.Params {
		Walk(p, v)
	}
	for _, r := range m.Returns {
		if r != nil {
			Walk(r.Type, v)
		}
	}
	for _, s := range m.Body {
		Walk(s, v)
	}
	walkSlots(m.SlotsByName(), v)
}

func walkFunction(f *Function, v Visitor) {
	for _, tp := range f.TypeParams {
		Walk(tp, v)
	}
	for _, p := range f.Params {
		Walk(p, v)
	}
	for _, r := range f.Returns {
		if r != nil {
			Walk(r.Type, v)
		}
	}
	for _, s := range f.Body {
		Walk(s, v)
	}
	walkSlots(f.SlotsByName(), v)
}

func walkField(f *Field, v Visitor) {
	Walk(f.Type, v)
	walkSlots(f.SlotsByName(), v)
}

func walkEnum(e *Enum, v Visitor) {
	Walk(e.Underlying, v)
	for _, vt := range e.Variants {
		Walk(vt, v)
	}
	walkSlots(e.SlotsByName(), v)
}

func walkAlias(a *Alias, v Visitor) {
	for _, tp := range a.TypeParams {
		Walk(tp, v)
	}
	Walk(a.Target, v)
}

func walkTypeParam(tp *TypeParam, v Visitor) {
	if tp.Constraint == nil {
		return
	}
	for _, e := range tp.Constraint.Embedded {
		Walk(e, v)
	}
}

func walkCompositeRef(r *CompositeRef, v Visitor) {
	switch r.Shape {
	case ShapePointer, ShapeSlice, ShapeArray:
		Walk(r.Elem, v)
	case ShapeMap:
		Walk(r.MapKey, v)
		Walk(r.MapValue, v)
	case ShapeFunc:
		for _, p := range r.FuncParams {
			Walk(p, v)
		}
		for _, ret := range r.FuncReturns {
			Walk(ret, v)
		}
	case ShapeUnion:
		for _, t := range r.UnionTerms {
			Walk(t.Type, v)
		}
	}
}

func walkStmt(s *Stmt, v Visitor) {
	Walk(s.Init, v)
	Walk(s.Cond, v)
	Walk(s.Post, v)
	Walk(s.RangeOver, v)
	for _, t := range s.Targets {
		Walk(t, v)
	}
	for _, val := range s.Values {
		Walk(val, v)
	}
	for _, r := range s.Returns {
		Walk(r, v)
	}
	Walk(s.Call, v)
	Walk(s.DeclType, v)
	Walk(s.Inner, v)
	Walk(s.Node, v)
	for _, sub := range s.Block {
		Walk(sub, v)
	}
	for _, sub := range s.Else {
		Walk(sub, v)
	}
	for _, c := range s.Cases {
		Walk(c, v)
	}
}

func walkExpr(e *Expr, v Visitor) {
	Walk(e.Receiver, v)
	Walk(e.Callee, v)
	Walk(e.Left, v)
	Walk(e.Right, v)
	Walk(e.IndexExpr, v)
	Walk(e.Low, v)
	Walk(e.High, v)
	Walk(e.Max, v)
	Walk(e.AsType, v)
	for _, a := range e.Args {
		Walk(a, v)
	}
	for _, t := range e.TypeArgs {
		Walk(t, v)
	}
	for _, p := range e.FuncParams {
		Walk(p, v)
	}
	for _, r := range e.FuncReturns {
		Walk(r, v)
	}
	for _, s := range e.FuncBody {
		Walk(s, v)
	}
}

// walkSlots walks every slot in the host's slot map in alphabetical
// key order. Deterministic order matters for testability and for
// templates that depend on slot iteration being stable.
func walkSlots(slots map[string]*Slot, v Visitor) {
	if len(slots) == 0 {
		return
	}
	keys := make([]string, 0, len(slots))
	for k := range slots {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		Walk(slots[k], v)
	}
}
