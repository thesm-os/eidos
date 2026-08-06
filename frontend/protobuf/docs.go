// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package protobuf

import (
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// sourcePos returns the source position for desc relative to
// fd. The returned [position.Pos] carries the file path and the
// 1-indexed line / column protocompile reports; callers thread
// this through stamping helpers so the meta-bag provenance trail
// anchors to the declaring source.
func sourcePos(fd protoreflect.FileDescriptor, desc protoreflect.Descriptor) position.Pos {
	sl := fd.SourceLocations().ByDescriptor(desc)
	return position.Pos{File: fd.Path(), Line: sl.StartLine + 1, Column: sl.StartColumn + 1}
}

// docLinesAndTrailing returns the docs-attachable lines and the
// trailing same-line comment for the supplied source location.
//
// Per the documented comment-attachment rule, leading comments
// land in [node.BaseNode.DocLines] with the line-comment marker
// stripped; trailing same-line comments do NOT enter DocLines and
// instead route to a per-host `proto.<host>.trailing_doc` meta
// key. Detached comment blocks immediately preceding the
// declaration also attach to DocLines — protocompile materialises
// them on the SourceLocation as LeadingDetachedComments alongside
// the LeadingComments string.
func docLinesAndTrailing(sl protoreflect.SourceLocation) ([]string, string) {
	docs := commentBlockLines(sl.LeadingComments)
	for _, detached := range sl.LeadingDetachedComments {
		docs = append(docs, commentBlockLines(detached)...)
	}
	trailing := strings.TrimSpace(sl.TrailingComments)
	return docs, trailing
}

// commentBlockLines splits raw — a comment block protocompile
// surfaces with the `//` or `/* */` markers stripped — into one
// entry per source line. The leading-space convention proto
// tooling preserves (a single space after `//` on each line) is
// trimmed so the produced DocLines are immediately usable as
// rendered documentation.
//
// Empty or whitespace-only input returns nil so callers don't
// have to special-case empty blocks before appending.
func commentBlockLines(raw string) []string {
	trimmed := strings.TrimRight(raw, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, strings.TrimPrefix(line, " "))
	}
	return out
}

// attachStructDocs walks fd's source-location index for md and
// stamps any leading comments onto s.DocLines plus any trailing
// same-line comment under [MetaMessageTrailingDoc]. Leading-
// comment directives parsed from the same block route into
// s.DirectiveList so the pipeline's gate-fire step picks them up.
func attachStructDocs(
	ctx *plugin.FrontendContext, s *node.Struct,
	fd protoreflect.FileDescriptor, md protoreflect.MessageDescriptor,
) {
	sl := fd.SourceLocations().ByDescriptor(md)
	docs, trailing := docLinesAndTrailing(sl)
	if len(docs) > 0 {
		s.DocLines = append(s.DocLines, docs...)
	}
	pos := position.Pos{File: fd.Path(), Line: sl.StartLine + 1, Column: sl.StartColumn + 1}
	if dirs := directivesFor(ctx, sl, pos); len(dirs) > 0 {
		s.DirectiveList = append(s.DirectiveList, dirs...)
	}
	if trailing != "" {
		MetaMessageTrailingDoc.SetAt(
			s.EnsureMeta(), trailing, meta.AuthorityPlugin, FrontendName, pos,
		)
	}
}

// attachFieldDocs walks the source-location index for desc and
// stamps any leading comments onto f.DocLines plus any trailing
// same-line comment under [MetaFieldTrailingDoc]. Field-level
// directives parsed from the leading comments populate
// f.DirectiveList.
func attachFieldDocs(
	ctx *plugin.FrontendContext, f *node.Field,
	fd protoreflect.FileDescriptor, desc protoreflect.FieldDescriptor,
) {
	sl := fd.SourceLocations().ByDescriptor(desc)
	docs, trailing := docLinesAndTrailing(sl)
	if len(docs) > 0 {
		f.DocLines = append(f.DocLines, docs...)
	}
	pos := position.Pos{File: fd.Path(), Line: sl.StartLine + 1, Column: sl.StartColumn + 1}
	if dirs := directivesFor(ctx, sl, pos); len(dirs) > 0 {
		f.DirectiveList = append(f.DirectiveList, dirs...)
	}
	if trailing != "" {
		MetaFieldTrailingDoc.SetAt(
			f.EnsureMeta(), trailing, meta.AuthorityPlugin, FrontendName, pos,
		)
	}
}

// attachEnumDocs walks the source-location index for ed and
// stamps any leading comments onto e.DocLines plus any trailing
// same-line comment under [MetaEnumTrailingDoc]. Enum-level
// directives parsed from the leading comments populate
// e.DirectiveList.
func attachEnumDocs(
	ctx *plugin.FrontendContext, e *node.Enum,
	fd protoreflect.FileDescriptor, ed protoreflect.EnumDescriptor,
) {
	sl := fd.SourceLocations().ByDescriptor(ed)
	docs, trailing := docLinesAndTrailing(sl)
	if len(docs) > 0 {
		e.DocLines = append(e.DocLines, docs...)
	}
	pos := position.Pos{File: fd.Path(), Line: sl.StartLine + 1, Column: sl.StartColumn + 1}
	if dirs := directivesFor(ctx, sl, pos); len(dirs) > 0 {
		e.DirectiveList = append(e.DirectiveList, dirs...)
	}
	if trailing != "" {
		MetaEnumTrailingDoc.SetAt(
			e.EnsureMeta(), trailing, meta.AuthorityPlugin, FrontendName, pos,
		)
	}
}

// attachOneofDocs walks the source-location index for oo and
// stamps any leading comments onto the synthesized interface's
// DocLines, plus any `+gen:` directives parsed from the same
// block onto its DirectiveList. Trailing same-line comments on
// the oneof's opening brace land under [MetaOneofTrailingDoc],
// the per-host-kind trailing-doc key consumers filter on.
func attachOneofDocs(
	ctx *plugin.FrontendContext, iface *node.Interface,
	fd protoreflect.FileDescriptor, oo protoreflect.OneofDescriptor,
) {
	sl := fd.SourceLocations().ByDescriptor(oo)
	docs, trailing := docLinesAndTrailing(sl)
	if len(docs) > 0 {
		iface.DocLines = append(iface.DocLines, docs...)
	}
	pos := position.Pos{File: fd.Path(), Line: sl.StartLine + 1, Column: sl.StartColumn + 1}
	if dirs := directivesFor(ctx, sl, pos); len(dirs) > 0 {
		iface.DirectiveList = append(iface.DirectiveList, dirs...)
	}
	if trailing != "" {
		MetaOneofTrailingDoc.SetAt(
			iface.EnsureMeta(), trailing, meta.AuthorityPlugin, FrontendName, pos,
		)
	}
}

// attachInterfaceDocs walks the source-location index for sd and
// stamps any leading comments onto iface.DocLines plus any
// trailing same-line comment under [MetaServiceTrailingDoc].
// Service-level directives parsed from the leading comments
// populate iface.DirectiveList.
func attachInterfaceDocs(
	ctx *plugin.FrontendContext, iface *node.Interface,
	fd protoreflect.FileDescriptor, sd protoreflect.ServiceDescriptor,
) {
	sl := fd.SourceLocations().ByDescriptor(sd)
	docs, trailing := docLinesAndTrailing(sl)
	if len(docs) > 0 {
		iface.DocLines = append(iface.DocLines, docs...)
	}
	pos := position.Pos{File: fd.Path(), Line: sl.StartLine + 1, Column: sl.StartColumn + 1}
	if dirs := directivesFor(ctx, sl, pos); len(dirs) > 0 {
		iface.DirectiveList = append(iface.DirectiveList, dirs...)
	}
	if trailing != "" {
		MetaServiceTrailingDoc.SetAt(
			iface.EnsureMeta(), trailing, meta.AuthorityPlugin, FrontendName, pos,
		)
	}
}

// attachRPCDocs walks the source-location index for md and stamps
// any leading comments onto m.DocLines plus any trailing same-line
// comment under [MetaRPCTrailingDoc]. RPC-level directives parsed
// from the leading comments populate m.DirectiveList.
func attachRPCDocs(
	ctx *plugin.FrontendContext, m *node.Method,
	fd protoreflect.FileDescriptor, md protoreflect.MethodDescriptor,
) {
	sl := fd.SourceLocations().ByDescriptor(md)
	docs, trailing := docLinesAndTrailing(sl)
	if len(docs) > 0 {
		m.DocLines = append(m.DocLines, docs...)
	}
	pos := position.Pos{File: fd.Path(), Line: sl.StartLine + 1, Column: sl.StartColumn + 1}
	if dirs := directivesFor(ctx, sl, pos); len(dirs) > 0 {
		m.DirectiveList = append(m.DirectiveList, dirs...)
	}
	if trailing != "" {
		MetaRPCTrailingDoc.SetAt(
			m.EnsureMeta(), trailing, meta.AuthorityPlugin, FrontendName, pos,
		)
	}
}

// attachVariantDocs walks the source-location index for vd and
// stamps any leading comments onto v.DocLines plus any trailing
// same-line comment under [MetaEnumVariantTrailingDoc]. Variant-
// level directives parsed from the leading comments populate
// v.DirectiveList.
func attachVariantDocs(
	ctx *plugin.FrontendContext, v *node.EnumVariant,
	fd protoreflect.FileDescriptor, vd protoreflect.EnumValueDescriptor,
) {
	sl := fd.SourceLocations().ByDescriptor(vd)
	docs, trailing := docLinesAndTrailing(sl)
	if len(docs) > 0 {
		v.DocLines = append(v.DocLines, docs...)
	}
	pos := position.Pos{File: fd.Path(), Line: sl.StartLine + 1, Column: sl.StartColumn + 1}
	if dirs := directivesFor(ctx, sl, pos); len(dirs) > 0 {
		v.DirectiveList = append(v.DirectiveList, dirs...)
	}
	if trailing != "" {
		MetaEnumVariantTrailingDoc.SetAt(
			v.EnsureMeta(), trailing, meta.AuthorityPlugin, FrontendName, pos,
		)
	}
}
