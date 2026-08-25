// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"strconv"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// convertEnums walks fd's top-level enum declarations and
// appends one [node.Enum] per declaration to pkg. Every produced
// Enum carries [node.Enum.Underlying] set to the proto3 int32
// width (proto3 enums are uniformly int32) and one
// [node.EnumVariant] per declared variant in source order.
//
// Per-variant meta records the typed numeric value; per-enum
// meta records the allow-alias flag and any reserved-range
// reservations. Nested enums declared inside messages route
// through [appendNestedEnums] from the message walker so they
// land in the same Enums slice as their top-level peers.
func convertEnums(ctx *plugin.FrontendContext, pkg *node.Package, fd protoreflect.FileDescriptor) {
	enums := fd.Enums()
	for i := range enums.Len() {
		appendEnum(ctx, pkg, fd, enums.Get(i), "")
	}
}

// appendEnum converts ed into a [node.Enum] on pkg. prefix is
// the dot-joined ancestor chain for enums nested inside messages
// — top-level callers pass an empty string.
func appendEnum(
	ctx *plugin.FrontendContext, pkg *node.Package, fd protoreflect.FileDescriptor,
	ed protoreflect.EnumDescriptor, prefix string,
) {
	name := joinName(prefix, string(ed.Name()))
	e := &node.Enum{
		BaseNode:   node.BaseNode{SourcePos: sourcePos(fd, ed)},
		Name:       name,
		Package:    pkg.Path,
		Underlying: &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "int32"},
		Variants:   convertEnumVariants(ctx, fd, ed),
	}
	stampEnumMeta(e, fd, ed)
	attachEnumDocs(ctx, e, fd, ed)
	stampHostOptions(ctx.Diag.For(FrontendName), e.EnsureMeta(), ed.Options(), sourcePos(fd, ed))
	pkg.Enums = append(pkg.Enums, e)
}

// convertEnumVariants returns one [node.EnumVariant] per value
// declared on ed in source order. Each variant carries
// [node.EnumVariant.Value] as the decimal integer string and the
// typed numeric form on [MetaEnumVariantNumber] meta. proto3
// permits only decimal-integer enum values in source, so the
// normalised decimal form equals the verbatim source form; the
// equivalence is proto3-specific and would diverge under proto2
// or editions sources that allow other integer literals.
func convertEnumVariants(
	ctx *plugin.FrontendContext, fd protoreflect.FileDescriptor, ed protoreflect.EnumDescriptor,
) []*node.EnumVariant {
	values := ed.Values()
	count := values.Len()
	if count == 0 {
		return nil
	}
	out := make([]*node.EnumVariant, 0, count)
	for i := range count {
		v := values.Get(i)
		pos := sourcePos(fd, v)
		variant := &node.EnumVariant{
			BaseNode: node.BaseNode{SourcePos: pos},
			Name:     string(v.Name()),
			Value:    strconv.Itoa(int(v.Number())),
		}
		MetaEnumVariantNumber.SetAt(
			variant.EnsureMeta(), int(v.Number()),
			meta.AuthorityPlugin, FrontendName, pos,
		)
		attachVariantDocs(ctx, variant, fd, v)
		stampHostOptions(ctx.Diag.For(FrontendName), variant.EnsureMeta(), v.Options(), pos)
		out = append(out, variant)
	}
	return out
}

// stampEnumMeta records the per-enum meta documented on the
// protobuf surface: the allow-alias flag and reserved-range
// reservations. Each stamp threads the host's source position so
// the bag's provenance trail anchors to the declaring file for
// `eidos explain`-style introspection.
func stampEnumMeta(e *node.Enum, fd protoreflect.FileDescriptor, ed protoreflect.EnumDescriptor) {
	sl := fd.SourceLocations().ByDescriptor(ed)
	pos := position.Pos{File: fd.Path(), Line: sl.StartLine + 1, Column: sl.StartColumn + 1}
	if opts := enumOptions(ed); opts != nil && opts.GetAllowAlias() {
		MetaEnumAllowAlias.SetAt(e.EnsureMeta(), true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if nums := expandReservedEnumRanges(ed.ReservedRanges()); len(nums) > 0 {
		MetaEnumReservedNumbers.SetAt(e.EnsureMeta(), nums, meta.AuthorityPlugin, FrontendName, pos)
	}
	if names := reservedNames(ed.ReservedNames()); len(names) > 0 {
		MetaEnumReservedNames.SetAt(e.EnsureMeta(), names, meta.AuthorityPlugin, FrontendName, pos)
	}
}

// appendNestedEnums walks ed.Enums() on a message descriptor and
// appends each as a top-level entry on pkg with the parent
// message's name as the dot-joined prefix. The recursion is
// shallow because protoreflect surfaces every nested enum at the
// message it lives in; the file-level walker doesn't need to
// recurse into messages itself.
func appendNestedEnums(
	ctx *plugin.FrontendContext, pkg *node.Package, fd protoreflect.FileDescriptor,
	md protoreflect.MessageDescriptor, prefix string,
) {
	enums := md.Enums()
	for i := range enums.Len() {
		appendEnum(ctx, pkg, fd, enums.Get(i), prefix)
	}
}

// enumOptions extracts the descriptorpb.EnumOptions message from
// ed, or nil when the underlying descriptor carries no options.
func enumOptions(ed protoreflect.EnumDescriptor) *descriptorpb.EnumOptions {
	opts := ed.Options()
	if opts == nil {
		return nil
	}
	eo, ok := opts.(*descriptorpb.EnumOptions)
	if !ok {
		return nil
	}
	return eo
}

// expandReservedEnumRanges returns the flat integer list for
// enum reservations. protoreflect uses inclusive ranges for enum
// reservations (`[Start, End]`); this helper materialises every
// reserved value into the output slice.
func expandReservedEnumRanges(ranges protoreflect.EnumRanges) []int32 {
	count := ranges.Len()
	if count == 0 {
		return nil
	}
	var out []int32
	for i := range count {
		r := ranges.Get(i)
		for n := r[0]; n <= r[1]; n++ {
			out = append(out, int32(n))
		}
	}
	return out
}
