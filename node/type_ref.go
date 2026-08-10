// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

import (
	"go.thesmos.sh/eidos/core/kind"
)

// TypeRefKind discriminates the variant forms a [TypeRef] can take.
// The set covers the type shapes shared across general-purpose
// languages — Go, Rust, TypeScript, and similar — without elevating
// truly language-specific quirks to first-class fields. Language-
// specific facts ride on the [TypeRef]'s metadata via the `<lang>.*`
// namespace convention (`go.isChannel`, `go.chanDir`, etc.).
type TypeRefKind int

// TypeRef variants. New variants append to the end so existing
// ordinals stay stable for callers that switch on the discriminator.
const (
	// TypeRefNamed is a reference by qualified name (Package.Name),
	// optionally with generic type arguments. Covers user-defined
	// types, language built-ins ("int", "string", "any"), and
	// channels-as-named-refs (frontends stamp `<lang>.isChannel` etc.
	// on the ref to recover the channel facts without leaking
	// channel-as-primitive into the model).
	TypeRefNamed TypeRefKind = iota

	// TypeRefPointer is a pointer to a referenced type (Go's *T,
	// Rust's &T / Box<T>).
	TypeRefPointer

	// TypeRefSlice is an ordered, variable-length sequence (Go's
	// []T, Rust's Vec<T>, TypeScript's Array<T>).
	TypeRefSlice

	// TypeRefArray is a fixed-length sequence (Go's [N]T, Rust's
	// [T; N]).
	TypeRefArray

	// TypeRefMap is an associative container (Go's map[K]V, Rust's
	// HashMap<K, V>).
	TypeRefMap

	// TypeRefFunc is a function type with parameter and return
	// types.
	TypeRefFunc

	// TypeRefTypeParam is a use-site reference to a generic type
	// parameter declared on the enclosing decl (e.g. the `T` in
	// `func (l *List[T]) Get() T`). [TypeRef.Name] carries the
	// parameter's identifier. Distinguishing a type-param use from
	// a real Named ref preserves the semantic difference for
	// generators that need to know which `T`s are type parameters
	// vs which are types named "T".
	TypeRefTypeParam

	// TypeRefAnonStruct is an anonymous struct type expression,
	// e.g. `struct{ Name string }` appearing as a field type or a
	// function parameter type. [TypeRef.Fields] / [TypeRef.Embeds]
	// carry the inline structure.
	TypeRefAnonStruct

	// TypeRefAnonInterface is an anonymous interface type
	// expression, e.g. `interface{ Read([]byte) (int, error) }`
	// appearing as a field or parameter type. [TypeRef.Methods] and
	// [TypeRef.Embeds] carry the inline structure.
	TypeRefAnonInterface
)

// String returns the lower-case textual form of k for diagnostics.
func (k TypeRefKind) String() string {
	switch k {
	case TypeRefNamed:
		return "named"
	case TypeRefPointer:
		return "pointer"
	case TypeRefSlice:
		return "slice"
	case TypeRefArray:
		return "array"
	case TypeRefMap:
		return "map"
	case TypeRefFunc:
		return "func"
	case TypeRefTypeParam:
		return "type_param"
	case TypeRefAnonStruct:
		return "anon_struct"
	case TypeRefAnonInterface:
		return "anon_interface"
	default:
		return "type_ref_kind(?)"
	}
}

// TypeRef is a unified type reference used by [Field], [Method],
// [Param], [Function], [Alias], and others. Its [TypeRefKind]
// discriminator determines which fields are meaningful.
//
// Most fields are nil / zero unless the kind uses them. The frontend
// populates only the relevant ones per variant.
type TypeRef struct {
	BaseNode

	// TypeKind discriminates the variant. Set by the constructing
	// frontend; downstream reads via TypeRef.Kind().
	TypeKind TypeRefKind `json:"type_kind"`

	// Package is the source package path for Named refs ("" for
	// builtin or in-package types). Empty for non-Named variants.
	Package string `json:"package,omitempty"`

	// Name is the type identifier for Named refs and the type-param
	// identifier for TypeRefTypeParam refs. Empty for composite and
	// anonymous-type variants.
	Name string `json:"name,omitempty"`

	// TypeArgs are generic type arguments for Named refs that are
	// generic instantiations (Go's `Map[string, int]`). Empty for
	// non-Named variants.
	TypeArgs []*TypeRef `json:"type_args,omitempty"`

	// Elem is the element type for Pointer / Slice / Array.
	Elem *TypeRef `json:"elem,omitempty"`

	// ArrayLen is the length of an Array. Zero for slices and
	// non-array variants.
	ArrayLen int `json:"array_len,omitempty"`

	// MapKey is the key type for Map variants.
	MapKey *TypeRef `json:"map_key,omitempty"`

	// MapValue is the value type for Map variants.
	MapValue *TypeRef `json:"map_value,omitempty"`

	// FuncParams are the parameter types of a Func variant in
	// source order.
	FuncParams []*TypeRef `json:"func_params,omitempty"`

	// FuncReturns are the return types of a Func variant in source
	// order.
	FuncReturns []*TypeRef `json:"func_returns,omitempty"`

	// Fields are the inline fields of an [TypeRefAnonStruct]. Each
	// field's [Field.Owner] is the enclosing TypeRef so consumers
	// walking up from a field locate the anonymous host.
	Fields []*Field `json:"fields,omitempty"`

	// Methods are the inline methods of an [TypeRefAnonInterface].
	// Each method's [Method.Owner] is the enclosing TypeRef.
	Methods []*Method `json:"methods,omitempty"`

	// Embeds are the embedded types of an [TypeRefAnonStruct] or
	// [TypeRefAnonInterface] in source order. Each embed's
	// [Embed.Owner] is the enclosing TypeRef.
	Embeds []*Embed `json:"embeds,omitempty"`
}

// Kind returns [KindTypeRef] regardless of TypeRefKind — TypeRefKind
// discriminates *within* the type-reference family, not across the
// node hierarchy.
func (*TypeRef) Kind() kind.Kind { return KindTypeRef }

// IsBuiltin reports whether the ref is a Named ref with no Package
// (covering "int", "string", "bool", "any", and similar basic types).
func (r *TypeRef) IsBuiltin() bool {
	return r.TypeKind == TypeRefNamed && r.Package == ""
}

// IsGeneric reports whether the ref is a Named ref carrying generic
// type arguments.
func (r *TypeRef) IsGeneric() bool {
	return r.TypeKind == TypeRefNamed && len(r.TypeArgs) > 0
}

// IsPointer reports whether the ref is a pointer to another type.
func (r *TypeRef) IsPointer() bool { return r.TypeKind == TypeRefPointer }

// IsSlice reports whether the ref is a variable-length sequence.
func (r *TypeRef) IsSlice() bool { return r.TypeKind == TypeRefSlice }

// IsArray reports whether the ref is a fixed-length sequence.
func (r *TypeRef) IsArray() bool { return r.TypeKind == TypeRefArray }

// IsMap reports whether the ref is an associative container.
func (r *TypeRef) IsMap() bool { return r.TypeKind == TypeRefMap }

// IsFunc reports whether the ref is a function type.
func (r *TypeRef) IsFunc() bool { return r.TypeKind == TypeRefFunc }

// IsTypeParam reports whether the ref is a use-site reference to a
// generic type parameter (e.g. the `T` in `func Get() T`).
func (r *TypeRef) IsTypeParam() bool { return r.TypeKind == TypeRefTypeParam }

// IsAnonStruct reports whether the ref is an anonymous struct
// type expression.
func (r *TypeRef) IsAnonStruct() bool { return r.TypeKind == TypeRefAnonStruct }

// IsAnonInterface reports whether the ref is an anonymous
// interface type expression.
func (r *TypeRef) IsAnonInterface() bool { return r.TypeKind == TypeRefAnonInterface }

// MayDenoteInterface reports whether the ref could name an interface.
//
// Two variants can: a Named ref, which resolves to whatever the
// declaration is, and an anonymous interface expression. Every other
// variant — pointer, slice, array, map, func, anonymous struct, type
// parameter — is a composite the model itself rules out, in any
// language it describes.
//
// # Why a caller wants this
//
// An interface's Embeds list holds two unrelated things: types whose
// methods it takes on, and terms constraining its type set. Nothing
// in the model separates them, so a walker has to. The composite half
// is decidable here, without a resolver and without knowing the
// language, which is what stops a walker asking a resolver a question
// the shape already answered — and then reporting the resolver's miss
// as a failed embed.
//
// It is deliberately *not* [TypeRef.IsBuiltin]: `error` and `any` are
// builtin and are interfaces, so `interface{ error }` is a genuine
// embed. A Named ref stays ambiguous here on purpose; telling `int`
// from `error` from an unloaded `MyReader` needs type information the
// model does not carry, and belongs to whichever frontend has it.
func (r *TypeRef) MayDenoteInterface() bool {
	if r == nil {
		return false
	}
	return r.TypeKind == TypeRefNamed || r.TypeKind == TypeRefAnonInterface
}

// Equal reports whether r and other refer to the same type
// structurally — same kind, same package + name + type args (for
// Named refs), same recursive shape (for composite refs), same
// inline structure (for anonymous-type variants).
//
// Equal compares structural type identity only; it does not consider
// position, doc comments, directives, or metadata. Two refs that
// describe the same logical type but originated from different
// positions in source compare equal — appropriate for "do these two
// methods have the same signature?" checks.
//
// nil compares equal to nil; a nil receiver or nil argument
// compares unequal to any non-nil ref.
func (r *TypeRef) Equal(other *TypeRef) bool {
	if r == nil || other == nil {
		return r == other
	}
	if r.TypeKind != other.TypeKind {
		return false
	}
	switch r.TypeKind {
	case TypeRefNamed:
		return r.equalNamed(other)
	case TypeRefPointer, TypeRefSlice:
		return r.Elem.Equal(other.Elem)
	case TypeRefArray:
		return r.ArrayLen == other.ArrayLen && r.Elem.Equal(other.Elem)
	case TypeRefMap:
		return r.MapKey.Equal(other.MapKey) && r.MapValue.Equal(other.MapValue)
	case TypeRefFunc:
		return equalRefSlice(r.FuncParams, other.FuncParams) &&
			equalRefSlice(r.FuncReturns, other.FuncReturns)
	case TypeRefTypeParam:
		return r.Name == other.Name
	case TypeRefAnonStruct:
		return r.equalAnonStruct(other)
	case TypeRefAnonInterface:
		return r.equalAnonInterface(other)
	default:
		return false
	}
}

// equalNamed handles the [TypeRefNamed] equality path: package, name,
// and recursive comparison of type arguments.
func (r *TypeRef) equalNamed(other *TypeRef) bool {
	if r.Package != other.Package || r.Name != other.Name {
		return false
	}
	return equalRefSlice(r.TypeArgs, other.TypeArgs)
}

// equalAnonStruct compares anonymous-struct typerefs by their inline
// field + embed shape. Field tags are part of identity (Go's
// reflection considers tags significant for struct identity).
func (r *TypeRef) equalAnonStruct(other *TypeRef) bool {
	if len(r.Fields) != len(other.Fields) || len(r.Embeds) != len(other.Embeds) {
		return false
	}
	for i, f := range r.Fields {
		o := other.Fields[i]
		if f.Name != o.Name || f.Tag != o.Tag || !f.Type.Equal(o.Type) {
			return false
		}
	}
	for i, e := range r.Embeds {
		if !e.Type.Equal(other.Embeds[i].Type) {
			return false
		}
	}
	return true
}

// equalAnonInterface compares anonymous-interface typerefs by their
// inline method + embed shape. Method-set order is significant for
// source faithfulness even though Go's type identity is
// order-insensitive — callers that want order-insensitive
// comparison sort the methods first.
//
// Language-specific shapes (Go's `~int | ~string` type-set inside an
// anonymous interface used as a constraint) ride on metadata stamped
// by the originating frontend rather than first-class fields.
func (r *TypeRef) equalAnonInterface(other *TypeRef) bool {
	if len(r.Methods) != len(other.Methods) || len(r.Embeds) != len(other.Embeds) {
		return false
	}
	for i, m := range r.Methods {
		o := other.Methods[i]
		if m.Name != o.Name {
			return false
		}
		if !paramsEqual(m.Params, o.Params) || !equalRefSlice(ReturnTypes(m.Returns), ReturnTypes(o.Returns)) {
			return false
		}
	}
	for i, e := range r.Embeds {
		if !e.Type.Equal(other.Embeds[i].Type) {
			return false
		}
	}
	return true
}

// equalRefSlice compares two TypeRef slices element-wise for
// structural equality.
func equalRefSlice(a, b []*TypeRef) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}

// paramsEqual compares two Param slices by type (parameter names
// are not part of a method's type identity in Go).
func paramsEqual(a, b []*Param) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Variadic != b[i].Variadic || !a[i].Type.Equal(b[i].Type) {
			return false
		}
	}
	return true
}
