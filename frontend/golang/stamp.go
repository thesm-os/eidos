// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"go/constant"
	"go/types"
	"reflect"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// stampTypeRefMeta records language-flavoured facts on a
// [node.TypeRef] produced by the converter. Each stamped key
// carries full provenance — author "golang" at AuthorityPlugin
// with the ref's own source position — so [--explain] traces
// every Go-frontend-derived fact back to the type expression that
// motivated it.
//
// Currently records:
//
//   - [MetaIsContext] when the ref points at `context.Context`.
//   - [MetaIsError] when the ref is the predeclared `error`.
//   - [MetaIsStringer] when the underlying type implements
//     `fmt.Stringer`.
//   - [MetaIsComparable] when the underlying type satisfies the
//     `comparable` constraint.
//
// The caller ([converter.typeRefOf]) only invokes the stamper after
// [buildTypeRef] returns a non-nil ref, and ref is non-nil only for
// non-nil t — both arguments are guaranteed live.
func (c *converter) stampTypeRefMeta(ref *node.TypeRef, t types.Type) {
	bag := ref.Meta()
	pos := ref.Pos()
	if pos.IsZero() {
		// The converter assembles refs in two passes — buildTypeRef
		// returns a positionless ref, and the caller (overlay,
		// struct/iface populate) sets SourcePos afterwards. For
		// provenance the position needs to be set BEFORE stamping;
		// fall back to the type's own declaration position so
		// --explain traces still resolve to a real source location
		// when the caller hasn't yet wired its overlay.
		pos = c.declPosOf(t)
	}
	if isContextType(t) {
		MetaIsContext.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if isErrorType(t) {
		MetaIsError.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if implementsStringer(t) {
		MetaIsStringer.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if types.Comparable(t) {
		MetaIsComparable.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if isInterfaceType(t) {
		MetaIsInterface.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
}

// isInterfaceType reports whether t's underlying type is an
// interface, excluding type parameters.
//
// The exclusion is load-bearing rather than defensive. A type
// parameter reports its *constraint* as its underlying type, so
// `K comparable` and `K any` both underlie to an interface. Without
// the carve-out every generic callable would advertise its key as
// an interface, and consumers keying on [MetaIsInterface] to spot a
// collaborator would misroute the entire generic surface.
func isInterfaceType(t types.Type) bool {
	if _, isParam := t.(*types.TypeParam); isParam {
		return false
	}
	_, isInterface := t.Underlying().(*types.Interface)
	return isInterface
}

// declPosOf returns the source position where t was declared. Used
// as a fallback when the converter has not yet wired a more
// specific position onto the ref's [node.TypeRef.SourcePos]; named
// and type-param types expose [types.Object.Pos], basic types and
// composites do not and yield a zero [position.Pos].
func (c *converter) declPosOf(t types.Type) position.Pos {
	switch x := t.(type) {
	case *types.Named:
		if obj := x.Obj(); obj != nil {
			return posOf(c.fset, obj.Pos())
		}
	case *types.TypeParam:
		if obj := x.Obj(); obj != nil {
			return posOf(c.fset, obj.Pos())
		}
	}
	return position.Pos{}
}

// isContextType reports whether t is the `context.Context`
// interface — Go's canonical context-propagation type.
func isContextType(t types.Type) bool {
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == "context" && obj.Name() == "Context"
}

// isErrorType reports whether t is the predeclared `error`
// interface — a builtin without a package path. *types.Named.Obj()
// is guaranteed non-nil by go/types for any resolved named type.
func isErrorType(t types.Type) bool {
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Pkg() == nil && obj.Name() == "error"
}

// stringerInterface is the canonical `fmt.Stringer` interface used
// by the [implementsStringer] check. It is constructed once at
// package init from a synthetic [types.NewInterface] declaration
// matching the standard library's shape.
var stringerInterface = newStringerInterface() //nolint:gochecknoglobals // shared singleton

// newStringerInterface constructs the synthetic `fmt.Stringer`
// type — a single-method interface declaring `String() string`.
func newStringerInterface() *types.Interface {
	stringObj := types.Universe.Lookup("string")
	stringType := stringObj.Type()
	sig := types.NewSignatureType(nil, nil, nil, nil,
		types.NewTuple(types.NewVar(0, nil, "", stringType)), false)
	method := types.NewFunc(0, nil, "String", sig)
	iface := types.NewInterfaceType([]*types.Func{method}, nil)
	iface.Complete()
	return iface
}

// implementsStringer reports whether t implements the canonical
// `fmt.Stringer` interface (a `String() string` method) on either
// the value or pointer form. The caller ([stampTypeRefMeta]) has
// already filtered nil types, so t is always non-nil here.
func implementsStringer(t types.Type) bool {
	return types.Implements(t, stringerInterface) ||
		types.Implements(types.NewPointer(t), stringerInterface)
}

// stampStructMeta records struct-level Go facts on s with full
// provenance:
//
//   - [MetaEmbedsInterface] when at least one of s's embeds is an
//     interface (Go's promotion-by-embedding case).
func (*converter) stampStructMeta(s *node.Struct, st *types.Struct) {
	for v := range st.Fields() {
		if !v.Embedded() {
			continue
		}
		if isInterfaceUnderlying(v.Type()) {
			MetaEmbedsInterface.SetAt(s.Meta(), true, meta.AuthorityPlugin, FrontendName, s.Pos())
			return
		}
	}
}

// isInterfaceUnderlying reports whether t's underlying type is a
// [types.Interface]. The caller ([stampStructMeta]) only invokes
// it on resolved field types, so t is always non-nil here.
func isInterfaceUnderlying(t types.Type) bool {
	_, ok := t.Underlying().(*types.Interface)
	return ok
}

// stampInterfaceMeta records interface-level Go facts on i with
// full provenance:
//
//   - [MetaIsEmptyInterface] when the interface declares no
//     methods and no embeds.
//   - [MetaIsConstraintInterface] when the interface declares at
//     least one type-set or `~T` term.
func (*converter) stampInterfaceMeta(i *node.Interface, it *types.Interface, _ *types.TypeName) {
	bag := i.Meta()
	pos := i.Pos()
	if it.NumExplicitMethods() == 0 && it.NumEmbeddeds() == 0 {
		MetaIsEmptyInterface.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if hasConstraintEmbed(it) {
		MetaIsConstraintInterface.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
}

// hasConstraintEmbed reports whether at least one embedded entry of
// the interface is a type-set union or `~T` approximate term.
func hasConstraintEmbed(it *types.Interface) bool {
	for emb := range it.EmbeddedTypes() {
		if _, ok := emb.(*types.Union); ok {
			return true
		}
	}
	return false
}

// stampAliasMeta records the kind of the alias's underlying type
// in [MetaUnderlyingKind] with full provenance. obj is guaranteed
// non-nil by [convertAlias], which only invokes after its own
// non-nil check.
func (*converter) stampAliasMeta(a *node.Alias, obj *types.TypeName) {
	underlying := obj.Type().Underlying()
	MetaUnderlyingKind.SetAt(a.Meta(), underlyingKindOf(underlying), meta.AuthorityPlugin, FrontendName, a.Pos())
}

// underlyingKindOf returns a short string describing the kind of
// t. The caller ([stampAliasMeta]) routes here only for
// [node.Alias] declarations, which [convertTypeSpec] reaches only
// for underlying types outside the struct / interface family —
// the enumerated cases below are exhaustive for that input set.
// Inputs the type-checker could produce outside that set (e.g.
// *types.Union from a malformed constraint expression) yield ""
// via the zero-value of the named return, so downstream consumers
// can detect an unrecognised shape.
func underlyingKindOf(t types.Type) (kind string) {
	switch t.(type) {
	case *types.Basic:
		kind = "basic"
	case *types.Signature:
		kind = "func"
	case *types.Map:
		kind = "map"
	case *types.Slice:
		kind = "slice"
	case *types.Array:
		kind = "array"
	case *types.Pointer:
		kind = "pointer"
	case *types.Chan:
		kind = "chan"
	}
	return kind
}

// stampMethodMeta records method-level Go facts on m with full
// provenance:
//
//   - [MetaReceiverIsPointer] when the method is declared on a
//     pointer receiver.
func (*converter) stampMethodMeta(m *node.Method) {
	if m.Receiver != nil && m.Receiver.IsPointer() {
		MetaReceiverIsPointer.SetAt(m.Meta(), true, meta.AuthorityPlugin, FrontendName, m.Pos())
	}
}

// stampFunctionMeta records function-level Go facts on fn:
// iter.Seq / iter.Seq2 detection via the function's return type.
func (*converter) stampFunctionMeta(fn *node.Function, sig *types.Signature) {
	if sig == nil || sig.Results() == nil || sig.Results().Len() != 1 {
		return
	}
	rt := sig.Results().At(0).Type()
	named, ok := rt.(*types.Named)
	if !ok {
		return
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != "iter" {
		return
	}
	args := named.TypeArgs()
	bag := fn.Meta()
	pos := fn.Pos()
	switch obj.Name() {
	case "Seq":
		if args != nil && args.Len() == 1 {
			MetaIsIterSeq.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
			MetaIterValueType.SetAt(bag, args.At(0).String(), meta.AuthorityPlugin, FrontendName, pos)
		}
	case "Seq2":
		if args != nil && args.Len() == 2 {
			MetaIsIterSeq2.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
			MetaIterKeyType.SetAt(bag, args.At(0).String(), meta.AuthorityPlugin, FrontendName, pos)
			MetaIterValueType.SetAt(bag, args.At(1).String(), meta.AuthorityPlugin, FrontendName, pos)
		}
	}
}

// stampConstantMeta records constant-level Go facts on cst:
//
//   - [MetaIotaValue] when the constant has an integer-typed value.
func (*converter) stampConstantMeta(cst *node.Constant, obj *types.Const) {
	// obj is guaranteed non-nil by convertConstSpec's own check.
	v := obj.Val()
	if v == nil || v.Kind() != constant.Int {
		return
	}
	i, exact := constant.Int64Val(v)
	if !exact {
		return
	}
	MetaIotaValue.SetAt(cst.Meta(), int(i), meta.AuthorityPlugin, FrontendName, cst.Pos())
}

// stampVariantMeta forwards [MetaIotaValue] from an underlying
// constant onto an enum variant.
func (*converter) stampVariantMeta(v *node.EnumVariant, cst *node.Constant) {
	if val, ok := MetaIotaValue.Get(cst.Meta()); ok {
		MetaIotaValue.SetAt(v.Meta(), val, meta.AuthorityPlugin, FrontendName, v.Pos())
	}
}

// stampFieldTagMeta parses the struct-tag string on a field and
// stamps each `key:"value"` entry under the [MetaTagPrefix]
// namespace (e.g. `go.tag.json="id"`) with the field's position.
func (*converter) stampFieldTagMeta(f *node.Field) {
	if f.Tag == "" {
		return
	}
	pos := f.Pos()
	for _, entry := range tagEntriesOf(reflect.StructTag(f.Tag)) {
		setTagEntry(f.Meta(), MetaTagPrefix+entry.key, entry.value, pos)
	}
}

// tagEntry is one decoded struct-tag entry — the key, value pair
// the converter stamps onto a field's [meta.Bag].
type tagEntry struct {
	key   string
	value string
}

// tagEntriesOf decodes a [reflect.StructTag] into its declared
// key / value entries in source order. Entries whose value carries
// an invalid escape are skipped silently; the surrounding parser
// recovers and continues on to the next entry. Malformed shapes
// (missing colon, unterminated quote, etc.) terminate decoding at
// the offending position.
func tagEntriesOf(st reflect.StructTag) []tagEntry {
	var out []tagEntry
	s := string(st)
	for s != "" {
		i := 0
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		s = s[i:]
		if s == "" {
			return out
		}
		i = 0
		for i < len(s) && s[i] != ':' && s[i] != ' ' && s[i] != '\t' {
			i++
		}
		if i == 0 || i+1 >= len(s) || s[i] != ':' || s[i+1] != '"' {
			return out
		}
		key := s[:i]
		s = s[i+2:]
		i = 0
		for i < len(s) && s[i] != '"' {
			if s[i] == '\\' && i+1 < len(s) {
				i++
			}
			i++
		}
		if i >= len(s) {
			return out
		}
		value, err := unquoteTagValue(s[:i])
		s = s[i+1:]
		if err != nil {
			continue
		}
		out = append(out, tagEntry{key: key, value: value})
	}
	return out
}

// unquoteTagValue decodes a quoted struct-tag value's escapes.
func unquoteTagValue(s string) (string, error) {
	return strconvUnquoteWrapped("\"" + s + "\"")
}
