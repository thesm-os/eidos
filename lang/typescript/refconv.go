// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript

import (
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// maxConvDepth bounds the source-to-emit walk, on the same reasoning
// the frontend's own budget carries: a type expression nests, and a
// malformed graph should fail one declaration rather than the
// process.
const maxConvDepth = 16

// FromNode projects a source type reference onto the emit side.
//
// The one direction that matters: a generator reads a [node.TypeRef]
// off a declaration and has to hand the backend an [emit.Ref] to
// render. Doing it here rather than per generator is what keeps two
// generators from disagreeing about what a TypeScript union projects
// to.
func FromNode(t *node.TypeRef) emit.Ref {
	return fromNodeAt(t, maxConvDepth)
}

// fromNodeAt is the recursive worker.
func fromNodeAt(t *node.TypeRef, depth int) emit.Ref {
	if t == nil || depth <= 0 {
		return nil
	}

	switch {
	case IsUnion(t):
		return unionFrom(t, depth)
	case IsIntersection(t):
		// The emit side has no intersection shape. A type that is both
		// A and B is carried as its source text rather than as a union,
		// which would be the opposite claim.
		return textRef(t)
	case IsTuple(t), IsOperator(t):
		return textRef(t)
	case t.IsSlice(), t.IsArray():
		return emit.SliceOf(fromNodeAt(t.Elem, depth-1))
	case t.IsMap():
		return emit.MapOf(fromNodeAt(t.MapKey, depth-1), fromNodeAt(t.MapValue, depth-1))
	case t.IsPointer():
		return emit.Ptr(fromNodeAt(t.Elem, depth-1))
	case t.IsFunc():
		return funcFrom(t, depth)
	case t.IsBuiltin():
		return emit.Builtin(t.Name)
	default:
		return namedFrom(t, depth)
	}
}

// unionFrom projects a union onto the emit union shape, which is the
// one structural shape both sides model.
func unionFrom(t *node.TypeRef, depth int) emit.Ref {
	members := Members(t)
	terms := make([]emit.UnionTerm, 0, len(members))
	for _, m := range members {
		if ref := fromNodeAt(m, depth-1); ref != nil {
			terms = append(terms, emit.UnionTerm{Type: ref})
		}
	}
	return emit.Union(terms...)
}

// funcFrom projects a function type.
func funcFrom(t *node.TypeRef, depth int) emit.Ref {
	params := make([]emit.Ref, 0, len(t.FuncParams))
	for _, p := range t.FuncParams {
		params = append(params, fromNodeAt(p, depth-1))
	}
	returns := make([]emit.Ref, 0, len(t.FuncReturns))
	for _, r := range t.FuncReturns {
		returns = append(returns, fromNodeAt(r, depth-1))
	}
	return emit.FuncOf(params, returns)
}

// namedFrom projects a named reference.
//
// A ref carrying a module qualifier becomes an external reference, so
// the backend registers the import; one without becomes a builtin,
// which renders bare. There is no third case: TypeScript has no
// package-local qualifier the way Go does.
func namedFrom(t *node.TypeRef, depth int) emit.Ref {
	if t.Package == "" {
		return emit.Builtin(t.Name)
	}
	args := make([]emit.Ref, 0, len(t.TypeArgs))
	for _, a := range t.TypeArgs {
		args = append(args, fromNodeAt(a, depth-1))
	}
	ref := emit.External(t.Package, t.Name)
	ref.TypeArgs = args
	return ref
}

// textRef carries a shape the emit side cannot model as its source
// text.
//
// A builtin whose name is the whole type expression: the backend
// spells a builtin verbatim, which is exactly what a type carried as
// text needs. Modelling these structurally would mean reproducing
// most of TypeScript's type system on the emit side, for shapes only
// TypeScript has.
func textRef(t *node.TypeRef) emit.Ref {
	if text, ok := MetaTypeText.Get(t.Meta()); ok && text != "" {
		return emit.Builtin(text)
	}
	return emit.Builtin(TypeString(t))
}
