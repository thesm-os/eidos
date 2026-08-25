// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"strings"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// Source answers how a declaration written in Go is read.
//
// The read-side half of what a plugin declares for Go, satisfying the
// SDK's source-rules contract. Every method forwards to the function
// beside it in this package, which is the point: the two notations a
// directive value may be written in, the literal well-formedness
// rule, the file-scoped qualifier lookup, the tag vocabulary, the
// type predicates and the sampler are Go's own rules and already live
// here. A plugin holding private copies is the same rules written
// twice, disagreeing the first time either is corrected.
//
// The zero value is usable and carries no state, so a declaration is
// `Source{}` and costs nothing to hand out.
//
// # Why this package does not import the SDK
//
// The methods take [node] types and return [emit] ones, and the
// façade's source and emit names are aliases of exactly those — so
// this satisfies the SDK interface structurally, without this package
// depending on it. That keeps the boundary the package documentation
// describes: `lang/golang` sits over [node], [emit] and `core`, below
// every consumer. The compile-time confirmation lives in
// `lang/golang/sdk`, which imports both and is where a plugin picks
// the declaration up.
type Source struct{}

// ResolveValue splits a value written in a directive or a tag into the
// package it names and the symbol within it.
//
// Both notations an author writes are accepted — a qualifier resolved
// against the file's own import block, and a full import path for a
// package imported only for the directive, which is otherwise an
// unused import and does not compile. See [ResolveValue].
func (Source) ResolveValue(f *node.File, value string) (pkg, symbol string, err error) {
	return ResolveValue(f, value)
}

// Tag returns the named struct-tag entry on f.
//
// Answered as the union of the `go.tag.*` stamp and the field's own
// tag string — see [Tag] — so a graph no Go frontend produced still
// reports the tags its declarations carry.
func (Source) Tag(f *node.Field, key string) (string, bool) {
	return Tag(f, key)
}

// FileOf returns the file within pkg that declared n.
func (Source) FileOf(pkg *node.Package, n node.Node) *node.File {
	return FileOf(pkg, n)
}

// TypeOf classifies a Go type into the shape vocabulary.
//
// The order of the arms is the classification. A byte slice is a
// slice and a set is a map, so each narrower reading is asked first —
// and the byte case takes the `Any` spelling, because `[]uint8` is
// the same type as `[]byte` and an author may have written either.
//
// A pointer answers as optional, which is what a pointer field means
// in a declaration a constructor fills: it distinguishes unset from
// zero. Nothing else here is a shape; a channel, a func, an interface
// and a defined scalar all owe one plain assignment.
//
// The resolver is unused, and deliberately. A defined type keeps its
// own shape: `type IDs []string` classifies as scalar, so its setter
// takes an `IDs` rather than a variadic `...string`, which is what the
// declaration was written for. Resolving through to the underlying
// type would hand the caller the representation and discard the name.
// Another language whose type aliases are transparent will need it,
// which is why the parameter is in the contract.
func (Source) TypeOf(t *node.TypeRef, _ Resolver) emit.TypeInfo {
	switch {
	case IsByteSliceAny(t):
		return emit.TypeInfo{Shape: emit.ShapeBytes, Elem: ElemType(t)}
	case IsSlice(t):
		return emit.TypeInfo{Shape: emit.ShapeSequence, Elem: ElemType(t)}
	case IsMap(t) && IsEmptyStruct(MapValue(t)):
		// A set carries its whole meaning in its keys, so it names no
		// element: there is no value type worth carrying when every
		// value is the same one.
		return emit.TypeInfo{Shape: emit.ShapeSet, Key: MapKeyType(t)}
	case IsMap(t):
		return emit.TypeInfo{
			Shape: emit.ShapeMapping,
			Key:   MapKeyType(t),
			Elem:  MapValType(t),
		}
	case t != nil && t.IsPointer():
		return emit.TypeInfo{Shape: emit.ShapeOptional, Elem: ElemType(t)}
	default:
		return emit.TypeInfo{Shape: emit.ShapeScalar}
	}
}

// SamplesOf returns two distinct values of a Go type.
//
// Resolved through r for named types, which is what lets a defined
// type answer with its own spelling rather than its underlying one.
// See [SampleRefFor] for what the walk reaches and what it refuses.
func (Source) SamplesOf(t *node.TypeRef, hint string, r Resolver) (sample, alternate Sample) {
	return SampleRefFor(t, hint, r)
}

// SubstituteParams returns t with the declaration's type parameters
// replaced by their derived witnesses.
func (Source) SubstituteParams(t *node.TypeRef, params []*node.TypeParam) *node.TypeRef {
	return SubstituteParamsNode(t, params)
}

// SubstituteRef returns r with the declaration's type parameters
// replaced by their derived witnesses.
func (Source) SubstituteRef(r emit.Ref, params []*node.TypeParam) emit.Ref {
	witnesses := Witnesses(params)
	if len(witnesses) == 0 {
		return r
	}
	by := make(map[string]emit.Ref, len(params))
	for i, p := range params {
		if p != nil {
			by[p.Name] = witnesses[i]
		}
	}
	return SubstituteTypeParams(r, by)
}

// LiteralFor renders text as a Go literal of t.
func (Source) LiteralFor(t *node.TypeRef, text string, r Resolver) (string, bool) {
	return LiteralFor(t, text, r)
}

// Settable returns the members of s a constructor in another package
// can set.
//
// Embeds come first and are marked promoted. An embedded type is a
// member named after itself, which a composite literal sets as a
// unit; promoting the fields inside it instead would offer two ways
// to write the same thing that disagree about whether the embedded
// value is set.
//
// Unexported members are absent: a constructor in another package
// cannot name them, and one in the same package would offer a way
// past the invariants the type declared them to protect.
func (Source) Settable(s *node.Struct) []emit.Member {
	if s == nil {
		return nil
	}
	out := make([]emit.Member, 0, len(s.Embeds)+len(s.Fields))
	for _, e := range s.Embeds {
		name, _ := EmbedIdent(e)
		if name == "" || !IsExported(name) {
			continue
		}
		out = append(out, emit.Member{
			Name:     name,
			Type:     FromNode(e.Type),
			Promoted: true,
		})
	}
	for _, f := range ExportedFields(s) {
		out = append(out, emit.Member{
			Name:   f.Name,
			Type:   FieldType(f),
			Meta:   f.Meta(),
			Pos:    f.Pos(),
			Source: f,
		})
	}
	return out
}

// TypeParams lifts s's generic parameter list into the emit form.
func (Source) TypeParams(s *node.Struct) []*emit.TypeParam { return TypeParams(s) }

// TypeArgs renders s's generic parameter list in use position.
func (Source) TypeArgs(s *node.Struct) string { return TypeArgs(s) }

// Witnesses returns one concrete type per parameter, or nil when any
// carries a constraint that cannot be reasoned about.
func (Source) Witnesses(params []*node.TypeParam) []emit.Ref { return Witnesses(params) }

// TypeName joins parts into the identifier a generated type is
// declared under — `User` and `Builder` giving `UserBuilder`.
//
// Each part is Pascal-cased and concatenated, which is Go's own rule
// for a declared type. A part arriving in another case therefore
// still reads as Go rather than carrying its origin's convention
// through.
func (Source) TypeName(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(ExportedName(p))
	}
	return b.String()
}

// ConstructorName spells the identifier of the function building a
// value of the named type — Go's `New<Type>`.
func (Source) ConstructorName(base string) string { return ConstructorName(base) }

// ZeroLiteral returns Go's spelling of a type's zero value, resolving
// named types through r.
//
// A caller without a resolver keeps the narrower answer, which is
// correct rather than absent — see [ZeroLiteralFor].
func (Source) ZeroLiteral(t *node.TypeRef, r Resolver) (string, bool) {
	return ZeroLiteralFor(t, r)
}

// EnumOf projects a Go enum into the neutral vocabulary.
//
// The optional half of the read side, and the reason it is optional:
// a language with no enumerated declarations does not implement this,
// and a generator finds that out by asserting rather than by reading
// an empty answer back. See [EnumInfoOf].
func (Source) EnumOf(e *node.Enum, constants []*node.Constant) emit.EnumInfo {
	return EnumInfoOf(e, constants)
}

// SentinelName composes an error variable's identifier — `Err<Base>`.
func (Source) SentinelName(base string) string { return SentinelName(base) }

// IsSentinelName reports whether an identifier is spelled as one.
//
// The matcher beside the composer deliberately: a generator emitting
// under one rule and finding under another produces variables its own
// detector cannot see. See [IsSentinelName].
func (Source) IsSentinelName(ident string) bool { return IsSentinelName(ident) }

// ErrorOf projects a Go struct into the error contract it carries.
//
// The method set is walked through embeds and the literal's field set
// is not — see [ErrorInfoOf] for why the two differ.
func (Source) ErrorOf(s *node.Struct, r Resolver) (emit.ErrorInfo, bool) {
	return ErrorInfoOf(s, r)
}
