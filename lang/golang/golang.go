// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"strings"
	"text/template"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// Language is the language identifier the Go adapter answers
// to. Plugins dispatch [plugin.Plugin.Outputs] /
// [plugin.Plugin.Templates] / [plugin.TemplateProvider.TemplateFuncs]
// on this constant to route per-language behaviour to the Go
// side.
const Language = "golang"

// IsExported reports whether name follows Go's
// exported-identifier rule — first rune upper-case ASCII for
// the simple cases plugins target. Empty input is unexported
// by definition.
func IsExported(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'
}

// ExportedFields returns the exported fields of s in source
// order. Plugin templates iterate the result to render
// setters / projections that mirror what user code can
// reach.
func ExportedFields(s *node.Struct) []*node.Field {
	out := make([]*node.Field, 0, len(s.Fields))
	for _, f := range s.Fields {
		if IsExported(f.Name) {
			out = append(out, f)
		}
	}
	return out
}

// IsSlice reports whether t is a non-byte slice. `[]byte`
// routes through [IsByteSlice] instead so plugin templates
// can pick a bytes-flavoured setter pair for that special
// case without seeing it twice.
func IsSlice(t *node.TypeRef) bool {
	return t != nil && t.IsSlice() && !IsByteSlice(t)
}

// IsMap reports whether t is a map type.
func IsMap(t *node.TypeRef) bool {
	return t != nil && t.IsMap()
}

// IsByteSlice reports whether t is `[]byte` — the only slice
// shape Go renders idiomatically through a bytes-string
// convenience setter pair rather than the variadic /
// append pair every other slice uses.
func IsByteSlice(t *node.TypeRef) bool {
	if t == nil || !t.IsSlice() || t.Elem == nil {
		return false
	}
	return t.Elem.IsBuiltin() && t.Elem.Name == typeByte
}

// FieldType returns the [emit.Ref] for a field's declared
// type — the shape plugin templates feed to the backend's
// `renderType` funcmap for setter parameters, internal
// value-field types, and composite-literal types.
func FieldType(f *node.Field) emit.Ref {
	return FromNode(f.Type)
}

// ElemType returns the [emit.Ref] for the slice / array
// element type of t.
func ElemType(t *node.TypeRef) emit.Ref {
	return FromNode(t.Elem)
}

// MapKeyType returns the [emit.Ref] for the map-key type of
// t.
func MapKeyType(t *node.TypeRef) emit.Ref {
	return FromNode(t.MapKey)
}

// MapValType returns the [emit.Ref] for the map-value type
// of t.
func MapValType(t *node.TypeRef) emit.Ref {
	return FromNode(t.MapValue)
}

// TypeParams lifts s's source-side type-parameter list into
// the [emit.TypeParam] form the backend's `renderTypeParams`
// funcmap entry consumes. Constraint conversion runs through
// [ConstraintFromNode] so external constraint types register
// their import on the rendered file automatically. Returns
// nil for non-generic structs so the template's
// `renderTypeParams` call emits no bracket list.
func TypeParams(s *node.Struct) []*emit.TypeParam {
	return TypeParamDecls(s.TypeParams)
}

// TypeArgs returns the bracketed parameter-name use form for
// s's type-parameter list (`[T, K]`) or the empty string for
// non-generic structs. Plugin templates use the result for
// receiver / return / composite-literal type-arg threading
// on each rendered method.
func TypeArgs(s *node.Struct) string {
	return TypeParamNames(s.TypeParams)
}

// SelfType returns the [emit.Ref] for s's own type
// instantiation — `Container` for non-generic structs,
// `Container[T]` for generics. Plugin templates use the
// result for emitted helper types' internal value fields,
// for `Build`-style return types, and for `From`-style
// parameter types. Same-package elision in the Go backend's
// `renderType` drops the package qualifier when the
// rendered file lives in the source package.
func SelfType(s *node.Struct) emit.Ref {
	return SelfRef(s.Package, s.Name, s.TypeParams)
}

// FuncMap returns the conventional template-side funcmap the
// shared Go helpers register under. Plugins compose this
// with their plugin-specific entries in their own
// [plugin.TemplateProvider.TemplateFuncs] implementation —
// the bundled keys cover the predicates / lifters every Go
// template needs so the plugin only declares its own
// extensions.
//
// Keys:
//
//	isExported     → IsExported
//	exportedFields → ExportedFields
//	isSlice        → IsSlice
//	isMap          → IsMap
//	isByteSlice    → IsByteSlice
//	fieldType      → FieldType
//	elemType       → ElemType
//	mapKeyType     → MapKeyType
//	mapValType     → MapValType
//	typeParams     → TypeParams
//	typeArgs       → TypeArgs
//	selfType       → SelfType
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"isExported":     IsExported,
		"exportedFields": ExportedFields,
		"isSlice":        IsSlice,
		"isMap":          IsMap,
		"isByteSlice":    IsByteSlice,
		"fieldType":      FieldType,
		"elemType":       ElemType,
		"mapKeyType":     MapKeyType,
		"mapValType":     MapValType,
		"typeParams":     TypeParams,
		"typeArgs":       TypeArgs,
		"selfType":       SelfType,
	}
}

// IsByte reports whether t is the builtin byte.
//
// Both spellings, because the frontend records whichever the author
// wrote — which is the edge case a private copy of this predicate
// gets wrong, and the one that turns a `[]byte` convenience setter
// into a `[]uint8` one nobody expected.
func IsByte(t *node.TypeRef) bool {
	return t != nil && t.IsBuiltin() && (t.Name == typeByte || t.Name == typeUint8)
}

// IsEmptyStruct reports whether t is the anonymous empty struct,
// which is what makes a map to it a set rather than a mapping.
//
// Both emptiness tests, because an anonymous struct may carry
// embedded types as well as declared fields, and one holding either
// is a value a caller has something to say about.
func IsEmptyStruct(t *node.TypeRef) bool {
	return t != nil && t.IsAnonStruct() && len(t.Fields) == 0 && len(t.Embeds) == 0
}

// IsVariadic reports whether p is a variadic parameter.
//
// Worth a predicate rather than a field read because forwarding a
// variadic parameter without its ellipsis passes the slice as a
// single element: it type-checks against `...any` and silently
// records one argument where the caller passed several.
func IsVariadic(p *node.Param) bool { return p != nil && p.Variadic }

// Instantiation renders the concrete type-argument list a generic
// declaration is instantiated at — `[string, int]` — or the empty
// string when args is empty.
//
// The third of Go's three type-parameter spellings, completing
// [TypeParams] (the declaration form, `[K comparable, V any]`) and
// [TypeArgs] (the use form, `[K, V]`). Mixing them produces code
// that either fails to compile or compiles and asserts the wrong
// thing, and naming only two of the three is what leaves a consumer
// inventing a name for the third — which is how two packages end up
// exporting one identifier for different forms.
//
// The concrete form is what a non-generic entry point needs: a Go
// test function cannot take type parameters, so anything driving a
// generic helper from one must instantiate it here.
func Instantiation(args ...string) string {
	if len(args) == 0 {
		return ""
	}
	return "[" + strings.Join(args, ", ") + "]"
}
