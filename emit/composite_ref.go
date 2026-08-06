// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import (
	"go.thesmos.sh/eidos/core/kind"
)

// CompositeShape discriminates the variant a [CompositeRef] takes.
// The set covers composite type shapes common to general-purpose
// languages — Go, Rust, TypeScript, and similar. Language-specific
// composites (Go-style channels, Rust enum variants with payloads,
// etc.) ride on plugin-defined emit kinds rather than first-class
// variants here.
type CompositeShape int

// CompositeShape variants in declaration order.
//
// The order is part of the serialised contract: [CompositeRef.Shape]
// encodes as a bare integer, so a variant inserted anywhere but the
// end silently reinterprets every emit graph already on disk — a
// persisted `4` would stop meaning func. New variants append.
const (
	// ShapePointer is a pointer to another ref.
	ShapePointer CompositeShape = iota
	// ShapeSlice is a variable-length sequence.
	ShapeSlice
	// ShapeArray is a fixed-length sequence. Length is stored in
	// [CompositeRef.ArrayLen].
	ShapeArray
	// ShapeMap is an associative container. MapKey / MapValue hold
	// the key and value refs.
	ShapeMap
	// ShapeFunc is a function type. FuncParams and FuncReturns hold
	// the parameter and return refs.
	ShapeFunc
	// ShapeUnion is a union of terms, used for constraint interfaces
	// of the form `A | B | ~C`. UnionTerms holds one [UnionTerm] per
	// member of the union; the per-term Approx flag records whether
	// the source carried the `~` prefix.
	ShapeUnion
	// ShapeAnonStruct is an inline, unnamed struct type. StructFields
	// holds its named fields in source order and StructEmbeds its
	// embedded types. Both empty is `struct{}` — how Go spells a set
	// as the value type of `map[K]struct{}`.
	//
	// An anonymous struct names no package, so it must not reach the
	// named-reference path: that builds a reference with an empty
	// import path, which the writer rejects outright and which aborts
	// the whole render rather than degrading its output. The types
	// *inside* the struct register their imports as usual, through
	// their own conversion.
	ShapeAnonStruct
)

// String returns the lower-case textual form of s for diagnostics.
func (s CompositeShape) String() string {
	switch s {
	case ShapePointer:
		return "pointer"
	case ShapeSlice:
		return "slice"
	case ShapeArray:
		return "array"
	case ShapeMap:
		return "map"
	case ShapeFunc:
		return "func"
	case ShapeUnion:
		return "union"
	case ShapeAnonStruct:
		return "anon_struct"
	default:
		return "composite_shape(?)"
	}
}

// AnonField is one named field of a [ShapeAnonStruct] composite.
//
// Deliberately not [Field]. A Field carries an Owner back-pointer and
// a slot map, both of which presuppose a declaration site: a
// cross-cutting generator contributing to a Field's "tags" slot
// expects a backend to route that contribution back to a declared
// struct. An anonymous struct inside a type expression has no
// declaration and no [Target], so those contributions would have
// nowhere to land. This carries what an inline field actually is —
// the three things rendering needs and the three
// [node.TypeRef.Equal] compares for structural identity.
type AnonField struct {
	// Name is the field identifier. Embedded types are carried in
	// [CompositeRef.StructEmbeds] instead.
	Name string `json:"name"`

	// Type is the field's declared type.
	Type Ref `json:"-"`

	// Tag is the raw struct-tag string without its enclosing
	// backticks. Tags are part of a Go struct's type identity, so
	// they survive the round trip rather than being dropped as
	// decoration.
	Tag string `json:"tag,omitempty"`
}

// UnionTerm is one member of a [ShapeUnion] composite. Approx records
// whether the source term carried the Go `~` prefix (e.g., `~int`)
// — meaningful when the union appears as a constraint interface in
// a type parameter list.
type UnionTerm struct {
	Type   Ref  `json:"-"`
	Approx bool `json:"approx,omitempty"`
}

// CompositeRef wraps an inner [Ref] with a composite shape. The
// [CompositeShape] discriminator selects which fields are
// meaningful — Pointer / Slice use Elem; Array uses Elem +
// ArrayLen; Map uses MapKey + MapValue; Func uses FuncParams +
// FuncReturns; Union uses UnionTerms.
type CompositeRef struct {
	BaseEmit

	// Shape discriminates the composite variant.
	Shape CompositeShape `json:"shape"`

	// Elem is the element ref for Pointer / Slice / Array shapes.
	Elem Ref `json:"-"`

	// ArrayLen is the length of a fixed-size array. Meaningful only
	// when Shape == ShapeArray.
	ArrayLen int `json:"array_len,omitempty"`

	// MapKey and MapValue describe the Map shape's key and value
	// types.
	MapKey   Ref `json:"-"`
	MapValue Ref `json:"-"`

	// FuncParams and FuncReturns describe the Func shape's
	// parameter and return types in source order.
	FuncParams  []Ref `json:"-"`
	FuncReturns []Ref `json:"-"`

	// UnionTerms holds the members of a [ShapeUnion] composite in
	// source order. Each term carries its element ref plus an
	// Approx flag (the Go `~T` prefix).
	UnionTerms []UnionTerm `json:"union_terms,omitempty"`

	// StructFields holds the named fields of a [ShapeAnonStruct]
	// composite in source order, and StructEmbeds its embedded
	// types. Both empty is the `struct{}` shape.
	StructFields []AnonField `json:"struct_fields,omitempty"`
	StructEmbeds []Ref       `json:"-"`
}

// Kind returns [KindCompositeRef].
func (*CompositeRef) Kind() kind.Kind { return KindCompositeRef }

// isRef marks CompositeRef as a [Ref] implementation.
func (*CompositeRef) isRef() {}

// Ptr wraps elem in a pointer composite.
//
//	emit.Ptr(emit.External("database/sql", "DB")) // *sql.DB
func Ptr(elem Ref) *CompositeRef {
	return &CompositeRef{Shape: ShapePointer, Elem: elem}
}

// SliceOf wraps elem in a slice composite.
//
//	emit.SliceOf(emit.Builtin("string")) // []string
func SliceOf(elem Ref) *CompositeRef {
	return &CompositeRef{Shape: ShapeSlice, Elem: elem}
}

// ArrayOf wraps elem in a fixed-length array composite of the given
// length.
//
//	emit.ArrayOf(emit.Builtin("byte"), 16) // [16]byte
func ArrayOf(elem Ref, length int) *CompositeRef {
	return &CompositeRef{Shape: ShapeArray, Elem: elem, ArrayLen: length}
}

// MapOf builds a map composite with the given key and value refs.
//
//	emit.MapOf(emit.Builtin("string"), emit.Internal(userStruct))
func MapOf(key, value Ref) *CompositeRef {
	return &CompositeRef{Shape: ShapeMap, MapKey: key, MapValue: value}
}

// FuncOf builds a function composite with the given parameter and
// return refs in source order. Nil slices are normalised to empty
// slices.
//
//	emit.FuncOf(
//	    []emit.Ref{emit.External("context", "Context")},
//	    []emit.Ref{emit.Builtin("error")},
//	) // func(context.Context) error
func FuncOf(params, returns []Ref) *CompositeRef {
	if params == nil {
		params = []Ref{}
	}
	if returns == nil {
		returns = []Ref{}
	}
	return &CompositeRef{Shape: ShapeFunc, FuncParams: params, FuncReturns: returns}
}

// Union builds a union composite from the supplied terms in source
// order. Variadic zero-term invocation produces a non-nil empty
// UnionTerms slice — useful as a builder seed that callers append
// to before downstream consumption.
//
//	emit.Union(
//	    emit.UnionTerm{Type: emit.Builtin("int")},
//	    emit.UnionTerm{Type: emit.Builtin("string"), Approx: true},
//	) // int | ~string
func Union(terms ...UnionTerm) *CompositeRef {
	if terms == nil {
		terms = []UnionTerm{}
	}
	return &CompositeRef{Shape: ShapeUnion, UnionTerms: terms}
}

// AnonStructOf builds an inline anonymous-struct composite from the
// supplied fields and embedded types, both in source order. Nil is
// the natural spelling of "none" for either, and both nil is the
// empty `struct{}`:
//
//	emit.MapOf(emit.Builtin("string"), emit.AnonStructOf(nil, nil))
//	// map[string]struct{}
//
//	emit.AnonStructOf([]emit.AnonField{
//	    {Name: "A", Type: emit.Builtin("int")},
//	    {Name: "B", Type: emit.Builtin("string"), Tag: `json:"b"`},
//	}, nil) // struct{ A int; B string `json:"b"` }
//
// Unlike [FuncOf], nil slices are not normalised to empty ones: a
// composite carrying no fields is the meaningful, common case here
// rather than an unset builder seed, and allocating two empty slices
// for every `struct{}` in a `map[K]struct{}`-heavy graph is pure
// waste.
func AnonStructOf(fields []AnonField, embeds []Ref) *CompositeRef {
	return &CompositeRef{
		Shape:        ShapeAnonStruct,
		StructFields: fields,
		StructEmbeds: embeds,
	}
}
