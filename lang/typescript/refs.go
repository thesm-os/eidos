// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript

import "go.thesmos.sh/eidos/node"

// RefPackage is the synthetic qualifier carried by a [node.TypeRef]
// that models a TypeScript type shape the model has no variant for.
//
// The convention is the Go frontend's: a channel arrives as a Named
// ref qualified `go` and named `chan`, recovering the channel facts
// from metadata rather than adding a channel variant to the model.
// TypeScript has more such shapes than Go does — unions,
// intersections and tuples are all ordinary in it — so the same trick
// carries more weight here.
//
// `ts` is not a real module specifier and cannot collide with one: a
// TypeScript import path is either relative (`./x`) or a package name,
// and neither is the bare string `ts`.
const RefPackage = "ts"

// The structural type markers. Each is the Name of a Named ref
// qualified by [RefPackage].
const (
	// RefUnion marks a union type. The members ride on the ref's
	// TypeArgs in source order.
	//
	// Members are TypeArgs rather than a metadata slice because they
	// are [node.TypeRef] values, and TypeArgs is the field a generic
	// walker already descends. Putting them on metadata would hide a
	// union's members from every traversal that does not know the key.
	RefUnion = "union"

	// RefIntersection marks an intersection type, members on TypeArgs
	// as for [RefUnion].
	RefIntersection = "intersection"

	// RefTuple marks a tuple type, elements on TypeArgs in order.
	// Per-element optionality and rest-ness ride on each element's
	// own [MetaOptional] and [MetaRest].
	RefTuple = "tuple"

	// RefOperator marks a type expression with no structured form —
	// conditional, mapped, `keyof`, `typeof`, `infer`, and
	// template-literal types. The source text rides on [MetaTypeText].
	//
	// One marker for all of them rather than one each: a consumer
	// either understands the text or it does not, and splitting the
	// marker would imply a structural difference the model does not
	// carry.
	RefOperator = "operator"
)

// IsMarker reports whether t is one of this package's structural
// markers — a Named ref qualified [RefPackage].
func IsMarker(t *node.TypeRef) bool {
	return t != nil && t.TypeKind == node.TypeRefNamed && t.Package == RefPackage
}

// IsUnion reports whether t models a union type.
func IsUnion(t *node.TypeRef) bool { return IsMarker(t) && t.Name == RefUnion }

// IsIntersection reports whether t models an intersection type.
func IsIntersection(t *node.TypeRef) bool { return IsMarker(t) && t.Name == RefIntersection }

// IsTuple reports whether t models a tuple type.
func IsTuple(t *node.TypeRef) bool { return IsMarker(t) && t.Name == RefTuple }

// IsOperator reports whether t models a type expression carried as
// text — see [RefOperator].
func IsOperator(t *node.TypeRef) bool { return IsMarker(t) && t.Name == RefOperator }

// Members returns the members of a union or intersection, or the
// elements of a tuple, in source order. Empty for any other ref.
//
// A named accessor rather than reaching for TypeArgs directly,
// because TypeArgs means "generic arguments" everywhere else and a
// call site reading `u.TypeArgs` to get union members reads as a
// mistake.
func Members(t *node.TypeRef) []*node.TypeRef {
	if !IsUnion(t) && !IsIntersection(t) && !IsTuple(t) {
		return nil
	}
	return t.TypeArgs
}
