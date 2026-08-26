// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture

import (
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// The package-level constructors build [node.TypeRef] values without
// struct-literal verbosity. They are pure functions; callers may
// compose freely.
//
// The set is TypeScript's type grammar rather than the model's
// vocabulary, which is why there is no Pointer: TypeScript has no
// pointer, and what a Go fixture spells with one this one spells with
// [Nullable]. Union, intersection and tuple are the marker refs
// `lang/typescript` declares, so a fixture builds the same shape the
// frontend does and a consumer reading either cannot tell them apart.

// Named builds a reference to a type in the current module —
// `string`, `User`.
func Named(name string) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefNamed, Name: name}
}

// ModNamed builds a reference to a type imported from another module
// — the `Person` in `import type { Person } from './models/person'`.
//
// The module is the specifier as written, so `./models/person` and
// `models/person` are different references and neither is normalised.
// The backend renders the specifier verbatim, which is the only
// treatment that keeps a relative import relative.
func ModNamed(module, name string) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefNamed, Package: module, Name: name}
}

// TypeParamRef builds a reference to a generic parameter in scope —
// the `T` in `Box<T>`.
//
// Spelled apart from [Named] because the two are indistinguishable in
// the model and not in meaning: a consumer substituting arguments has
// to know which names are bound.
func TypeParamRef(name string) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefTypeParam, Name: name}
}

// WithArgs applies type arguments to a named reference — `Box<string>`
// from Named("Box") and Named("string").
//
// Returns a copy, so the reference passed in is left alone and a
// fixture may apply different arguments to one base.
func WithArgs(named *node.TypeRef, args ...*node.TypeRef) *node.TypeRef {
	if named == nil {
		return nil
	}
	out := *named
	out.TypeArgs = args
	return &out
}

// Array builds `T[]`.
func Array(elem *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: elem}
}

// Record builds `Record<K, V>`, TypeScript's associative container.
func Record(key, value *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefMap, MapKey: key, MapValue: value}
}

// Func builds a function type — `(arg0: A) => R`.
//
// Several returns spell the tuple TypeScript would need, matching
// what the backend renders: the language has one return value, and a
// signature carrying more than one is the tuple that holds them.
func Func(params, returns []*node.TypeRef) *node.TypeRef {
	return &node.TypeRef{
		TypeKind:    node.TypeRefFunc,
		FuncParams:  params,
		FuncReturns: returns,
	}
}

// Union builds `A | B`, carried as the marker ref
// [typescript.RefUnion] with the members on TypeArgs.
//
// Structural rather than stamped on a metadata key, for the reason
// `lang/typescript` states: the members are [node.TypeRef] values and
// TypeArgs is the field a generic walker already descends, so hiding
// them behind a key would hide a union's members from every traversal
// that does not know it.
func Union(members ...*node.TypeRef) *node.TypeRef {
	return marker(typescript.RefUnion, members...)
}

// Intersection builds `A & B`.
func Intersection(members ...*node.TypeRef) *node.TypeRef {
	return marker(typescript.RefIntersection, members...)
}

// Tuple builds `[A, B]`, elements on TypeArgs in order.
func Tuple(elems ...*node.TypeRef) *node.TypeRef {
	return marker(typescript.RefTuple, elems...)
}

// Operator builds a type expression with no structured form —
// conditional, mapped, `keyof`, indexed access — carrying the source
// text verbatim.
//
// The projection and the backend both spell it by writing the text
// out, which is the only faithful treatment: a type the model cannot
// hold is one nothing downstream can reconstruct, and a
// reconstruction that guessed would be a different type that reads as
// correct.
func Operator(text string) *node.TypeRef {
	t := marker(typescript.RefOperator)
	typescript.MetaTypeText.Set(t.EnsureMeta(), text, markerAuthority)
	return t
}

// Literal builds a literal type — the `'admin'` in `type Role =
// 'admin'`, or `42`, or `true`.
//
// The text is the literal as written, quotes included, matching what
// the frontend records.
func Literal(text string) *node.TypeRef {
	t := Named(text)
	typescript.MetaLiteralType.Set(t.EnsureMeta(), text, markerAuthority)
	return t
}

// Nullable builds `T | null`.
//
// What a fixture for another language spells with a pointer. The
// union is the shape rather than a metadata flag, because that is
// what the source says and what every consumer classifying the type
// reads back.
func Nullable(inner *node.TypeRef) *node.TypeRef {
	return Union(inner, Literal(typescript.TypeNull))
}

// Undefinable builds `T | undefined`.
//
// Distinct from [Nullable] on purpose: TypeScript has two absent
// values and strictNullChecks makes the difference load-bearing, so a
// fixture that could only spell one could not reproduce the bug where
// a generator conflates them.
func Undefinable(inner *node.TypeRef) *node.TypeRef {
	return Union(inner, Literal(typescript.TypeUndefined))
}

// Object builds an inline object type — `{ a: string; b?: number }`.
func Object(fields ...*node.Field) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefAnonStruct, Fields: fields}
}

// AnonObject builds the empty object type — `object`, the type of
// anything that is not a primitive.
//
// Distinct from `Object()` with no members: `{}` accepts a string and
// `object` does not, and a generator that emitted one where it meant
// the other widened or narrowed a contract without changing a name.
func AnonObject() *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefAnonInterface}
}

// Prop builds one member of an inline [Object].
//
// A bare constructor rather than a sub-builder: an inline object's
// members carry a name and a type and nothing else worth configuring,
// and a callback per member would be more punctuation than
// declaration. Mark one optional by setting [typescript.MetaOptional]
// on the returned field.
func Prop(name string, t *node.TypeRef) *node.Field {
	return &node.Field{Name: name, Type: t}
}

// Constraint builds a generic parameter's bound — the `Shape` in
// `<T extends Shape>`.
//
// Several bounds become the intersection the backend renders, since
// TypeScript's `extends` takes one type and a parameter required to
// satisfy two is bounded by the type that is both.
func Constraint(bounds ...*node.TypeRef) *node.Constraint {
	if len(bounds) == 0 {
		return nil
	}
	return &node.Constraint{Embedded: bounds}
}

// Bound builds a generic parameter's bound carrying the source text
// the constraint was written as, alongside the references it names.
//
// For a bound the model has no structure for — `T extends keyof U`,
// a conditional, a mapped type. [Constraint] holds references and a
// consumer rendering one spells them; this one carries what the
// author wrote, which is the only faithful treatment of a bound
// nothing can reconstruct.
func Bound(raw string, bounds ...*node.TypeRef) *node.Constraint {
	return &node.Constraint{Raw: raw, Embedded: bounds}
}

// marker builds one of `lang/typescript`'s structural markers with
// its members on TypeArgs.
func marker(name string, args ...*node.TypeRef) *node.TypeRef {
	return &node.TypeRef{
		TypeKind: node.TypeRefNamed,
		Package:  typescript.RefPackage,
		Name:     name,
		TypeArgs: args,
	}
}
