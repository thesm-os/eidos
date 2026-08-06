// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package protobuf

import (
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// convertMessages walks fd's top-level message declarations and
// appends one [node.Struct] per message to pkg in declaration
// order. Nested messages produce additional [node.Struct] entries
// with dot-joined names — `Outer.Inner` for an `Inner` declared
// inside `Outer`.
//
// Each produced Struct carries the per-message meta (reserved
// ranges) and every contained Field carries the per-field meta
// (tag number, JSON name, deprecation flag, presence flag, oneof
// attribution).
func convertMessages(ctx *plugin.FrontendContext, pkg *node.Package, fd protoreflect.FileDescriptor) {
	msgs := fd.Messages()
	for i := range msgs.Len() {
		appendMessageStruct(ctx, pkg, fd, msgs.Get(i), "")
	}
}

// appendMessageStruct converts md into a [node.Struct] on pkg,
// then recurses into md.Messages() with the current message's
// name as the dot-joined prefix for any nested messages.
//
// prefix is the dot-joined ancestor chain for nested messages.
// Top-level callers pass an empty string; recursive calls pass
// the parent's dot-joined name so `message Outer { message Inner
// {} }` produces `Outer` and `Outer.Inner` entries.
func appendMessageStruct(
	ctx *plugin.FrontendContext, pkg *node.Package, fd protoreflect.FileDescriptor,
	md protoreflect.MessageDescriptor, prefix string,
) {
	if md.IsMapEntry() {
		// protocompile synthesises a hidden entry message for each
		// `map<K, V>` field. The map field's own surface is a
		// [node.TypeRefMap] composed alongside composite-type
		// handling; the synthetic host does not belong in the
		// produced graph.
		return
	}
	name := joinName(prefix, string(md.Name()))
	s := &node.Struct{
		BaseNode: node.BaseNode{SourcePos: sourcePos(fd, md)},
		Name:     name,
		Package:  pkg.Path,
		Fields:   convertFields(ctx, fd, md),
	}
	stampMessageReserved(s, fd, md)
	attachStructDocs(ctx, s, fd, md)
	stampHostOptions(ctx.Diag.For(FrontendName), s.EnsureMeta(), md.Options(), sourcePos(fd, md))
	pkg.Structs = append(pkg.Structs, s)
	// The order below is load-bearing:
	//   1. The host Struct is appended first so back-pointer
	//      stamps in synthesizeOneofs can target s directly.
	//   2. synthesizeOneofs runs at the current depth so nested
	//      messages don't inherit the outer's synthesized
	//      interfaces (each oneof attaches to its host).
	//   3. appendNestedEnums runs before recursion so enum
	//      declarations precede the nested-message Structs in
	//      source order across the produced graph.
	//   4. Recursion into md.Messages() descends into the next
	//      depth with the dot-joined parent prefix.
	synthesizeOneofs(ctx, pkg, s, fd, md, name)
	appendNestedEnums(ctx, pkg, fd, md, name)
	nested := md.Messages()
	for i := range nested.Len() {
		appendMessageStruct(ctx, pkg, fd, nested.Get(i), name)
	}
}

// convertFields returns one [node.Field] per declared field on
// md in declaration order with its per-field meta stamped. The
// per-field meta covers the documented protobuf surface: tag
// number, JSON name, deprecation, presence, and oneof membership.
func convertFields(
	ctx *plugin.FrontendContext,
	fd protoreflect.FileDescriptor, md protoreflect.MessageDescriptor,
) []*node.Field {
	fields := md.Fields()
	count := fields.Len()
	if count == 0 {
		return nil
	}
	out := make([]*node.Field, 0, count)
	for i := range count {
		desc := fields.Get(i)
		pos := sourcePos(fd, desc)
		f := &node.Field{
			BaseNode: node.BaseNode{SourcePos: pos},
			Name:     string(desc.Name()),
			Type:     convertFieldType(desc),
		}
		stampFieldMeta(f, desc, pos)
		attachFieldDocs(ctx, f, fd, desc)
		stampHostOptions(ctx.Diag.For(FrontendName), f.EnsureMeta(), desc.Options(), pos)
		out = append(out, f)
	}
	return out
}

// stampFieldMeta records the per-field meta documented on the
// protobuf surface: tag number on every field, JSON name on every
// field (default lowerCamelCase or explicit override), deprecated
// on deprecated fields, packed on explicit-packed fields, optional
// on proto3-explicit-presence fields, and oneof on oneof-member
// fields.
func stampFieldMeta(f *node.Field, desc protoreflect.FieldDescriptor, pos position.Pos) {
	bag := f.EnsureMeta()
	MetaFieldNumber.SetAt(bag, int(desc.Number()), meta.AuthorityPlugin, FrontendName, pos)
	MetaFieldJSONName.SetAt(bag, desc.JSONName(), meta.AuthorityPlugin, FrontendName, pos)
	if opts := fieldOptions(desc); opts != nil {
		if opts.GetDeprecated() {
			MetaFieldDeprecated.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
		}
		if opts.Packed != nil {
			MetaFieldPacked.SetAt(bag, opts.GetPacked(), meta.AuthorityPlugin, FrontendName, pos)
		}
	}
	if desc.HasOptionalKeyword() {
		MetaFieldOptional.SetAt(bag, true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if oo := desc.ContainingOneof(); oo != nil && !oo.IsSynthetic() {
		// proto3-explicit-presence fields are modelled as a synthetic
		// one-element oneof by protocompile; the `optional` keyword
		// is the user-facing surface, not a oneof membership. Skip
		// the synthetic oneof stamp so optional fields don't
		// accidentally carry proto.field.oneof.
		MetaFieldOneof.SetAt(bag, string(oo.Name()), meta.AuthorityPlugin, FrontendName, pos)
	}
}

// convertFieldType returns the [node.TypeRef] for desc.
//
// Cardinality is checked before kind: `map<K, V>` fields surface
// as [node.TypeRefMap] reading the key and value element kinds
// from the synthetic entry message; `repeated <X>` fields wrap
// the element kind in a [node.TypeRefSlice]. Scalar fields keep
// the proto source surface verbatim. Message- and enum-typed
// fields produce a named ref whose Package is the declaring
// file's package qualifier and whose Name is the dot-joined
// declaration path.
func convertFieldType(desc protoreflect.FieldDescriptor) *node.TypeRef {
	switch {
	case desc.IsMap():
		entry := desc.Message()
		return &node.TypeRef{
			TypeKind: node.TypeRefMap,
			MapKey:   elementTypeRef(entry.Fields().ByName("key")),
			MapValue: elementTypeRef(entry.Fields().ByName("value")),
		}
	case desc.IsList():
		return &node.TypeRef{
			TypeKind: node.TypeRefSlice,
			Elem:     elementTypeRef(desc),
		}
	default:
		return elementTypeRef(desc)
	}
}

// elementTypeRef returns the per-element [node.TypeRef] for desc.
// The helper drives the scalar / message / enum dispatch that
// applies regardless of whether the field carries map or repeated
// cardinality — the composite wrappers in [convertFieldType] call
// through here for each component element.
func elementTypeRef(desc protoreflect.FieldDescriptor) *node.TypeRef {
	switch desc.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return namedTypeRef(desc.Message().ParentFile(), desc.Message().FullName())
	case protoreflect.EnumKind:
		return namedTypeRef(desc.Enum().ParentFile(), desc.Enum().FullName())
	default:
		return &node.TypeRef{TypeKind: node.TypeRefNamed, Name: scalarKindName(desc.Kind())}
	}
}

// namedTypeRef constructs a [node.TypeRefNamed] for a reference
// to fullName declared in parent. The TypeRef.Name is the
// dot-joined declaration path inside the parent's package; the
// TypeRef.Package is the parent's proto-package qualifier.
//
// Well-known type references stamp [MetaWellKnown] on the
// produced ref so consumers dispatch on the bare name without
// parsing the qualified path.
func namedTypeRef(parent protoreflect.FileDescriptor, fullName protoreflect.FullName) *node.TypeRef {
	pkgPath := string(parent.Package())
	local := string(fullName)
	if pkgPath != "" && len(local) > len(pkgPath)+1 && local[:len(pkgPath)] == pkgPath {
		local = local[len(pkgPath)+1:]
	}
	ref := &node.TypeRef{
		TypeKind: node.TypeRefNamed,
		Name:     local,
		Package:  pkgPath,
	}
	if wk := wellKnownName(pkgPath, local); wk != "" {
		MetaWellKnown.Set(ref.EnsureMeta(), wk, FrontendName)
	}
	return ref
}

// stampMessageReserved records the reserved-range meta on s. Tag
// numbers expand from `reserved 7, 9 to 11;` into [7, 9, 10, 11];
// reserved names land verbatim. Absent on messages without any
// reservations. The host's source position rides on each stamp
// so the bag's provenance trail anchors to the declaring file
// for `eidos explain`-style introspection.
func stampMessageReserved(s *node.Struct, fd protoreflect.FileDescriptor, md protoreflect.MessageDescriptor) {
	sl := fd.SourceLocations().ByDescriptor(md)
	pos := position.Pos{File: fd.Path(), Line: sl.StartLine + 1, Column: sl.StartColumn + 1}
	if nums := expandReservedRanges(md.ReservedRanges()); len(nums) > 0 {
		MetaMessageReservedNumbers.SetAt(s.EnsureMeta(), nums, meta.AuthorityPlugin, FrontendName, pos)
	}
	if names := reservedNames(md.ReservedNames()); len(names) > 0 {
		MetaMessageReservedNames.SetAt(s.EnsureMeta(), names, meta.AuthorityPlugin, FrontendName, pos)
	}
}

// expandReservedRanges returns the flat list of every reserved
// integer covered by ranges. protoreflect uses half-open ranges
// `[Start, End)` for message reservations and `[Start, End]` for
// enum reservations; this helper handles the message-side
// half-open form. Enum reservations route through
// [expandReservedEnumRanges].
func expandReservedRanges(ranges protoreflect.FieldRanges) []int32 {
	count := ranges.Len()
	if count == 0 {
		return nil
	}
	var out []int32
	for i := range count {
		r := ranges.Get(i)
		for n := r[0]; n < r[1]; n++ {
			out = append(out, int32(n))
		}
	}
	return out
}

// reservedNames materialises the protoreflect.Names iterator into
// a Go slice.
func reservedNames(names protoreflect.Names) []string {
	count := names.Len()
	if count == 0 {
		return nil
	}
	out := make([]string, 0, count)
	for i := range count {
		out = append(out, string(names.Get(i)))
	}
	return out
}

// scalarKindName returns the canonical proto-source name for a
// scalar [protoreflect.Kind]. Non-scalar kinds return the empty
// string; callers handle the named-ref case via [convertFieldType].
func scalarKindName(k protoreflect.Kind) string {
	switch k {
	case protoreflect.BoolKind:
		return "bool"
	case protoreflect.Int32Kind:
		return "int32"
	case protoreflect.Sint32Kind:
		return "sint32"
	case protoreflect.Uint32Kind:
		return "uint32"
	case protoreflect.Int64Kind:
		return "int64"
	case protoreflect.Sint64Kind:
		return "sint64"
	case protoreflect.Uint64Kind:
		return "uint64"
	case protoreflect.Sfixed32Kind:
		return "sfixed32"
	case protoreflect.Fixed32Kind:
		return "fixed32"
	case protoreflect.FloatKind:
		return "float"
	case protoreflect.Sfixed64Kind:
		return "sfixed64"
	case protoreflect.Fixed64Kind:
		return "fixed64"
	case protoreflect.DoubleKind:
		return "double"
	case protoreflect.StringKind:
		return "string"
	case protoreflect.BytesKind:
		return "bytes"
	default:
		return ""
	}
}

// joinName composes a dot-joined name from a parent prefix and a
// child segment. Empty prefix returns the child verbatim — used
// by top-level message conversion where no ancestor exists.
func joinName(prefix, segment string) string {
	if prefix == "" {
		return segment
	}
	return prefix + "." + segment
}
