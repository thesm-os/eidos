// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// synthesizeOneofs walks md's user-declared oneof groups and
// appends one [node.Interface] per group to pkg. Synthetic
// oneofs that protocompile produces for proto3 `optional` fields
// are skipped — they are not user-declared groups and have no
// source-side comment, options, or directive surface.
//
// The synthesized interface name follows the dot-for-nesting,
// underscore-between-host-and-oneof convention: a oneof named
// `choice` declared inside `Outer.Inner` produces
// `Outer.Inner_choice`. The interface itself carries no methods
// and no embeds; variant identity flows through meta keys
// ([MetaFieldOneof], [MetaOneofInterface], [MetaOneofMessage])
// rather than through synthetic accessor methods.
//
// Custom options declared on the oneof block (`option (custom.opt)
// = ...;` inside the `oneof` body) route through the
// general-purpose custom-option channel; the per-host stamping
// path that landed alongside this synthesizer covers file-level
// options today and extends to oneof-host options when the
// option-channel surface broadens.
//
// host is the [node.Struct] this oneof attaches to (already
// appended to pkg.Structs by the caller); hostName is the
// dot-joined declaration path of the containing message —
// top-level messages pass their bare name, nested messages
// pass the `Outer.Inner` form.
func synthesizeOneofs(
	ctx *plugin.FrontendContext, pkg *node.Package, host *node.Struct,
	fd protoreflect.FileDescriptor, md protoreflect.MessageDescriptor,
	hostName string,
) {
	oneofs := md.Oneofs()
	for i := range oneofs.Len() {
		oo := oneofs.Get(i)
		if oo.IsSynthetic() {
			continue
		}
		appendOneofInterface(ctx, pkg, host, fd, oo, hostName)
	}
}

// appendOneofInterface synthesizes one [node.Interface] for the
// user-declared oneof oo and stamps the linkage meta. Each
// variant field on host is updated in place with a back-pointer
// to the synthesized interface via [MetaOneofInterface]; the
// interface itself carries a forward pointer to its host message
// via [MetaOneofMessage].
func appendOneofInterface(
	ctx *plugin.FrontendContext, pkg *node.Package, host *node.Struct,
	fd protoreflect.FileDescriptor,
	oo protoreflect.OneofDescriptor, hostName string,
) {
	ifaceName := oneofInterfaceName(hostName, string(oo.Name()))
	sl := fd.SourceLocations().ByDescriptor(oo)
	pos := position.Pos{File: fd.Path(), Line: sl.StartLine + 1, Column: sl.StartColumn + 1}
	iface := &node.Interface{
		BaseNode: node.BaseNode{SourcePos: pos},
		Name:     ifaceName,
		Package:  pkg.Path,
	}
	MetaOneofMessage.SetAt(
		iface.EnsureMeta(), pkg.Path+"."+hostName,
		meta.AuthorityPlugin, FrontendName, pos,
	)
	attachOneofDocs(ctx, iface, fd, oo)
	stampHostOptions(ctx.Diag.For(FrontendName), iface.EnsureMeta(), oo.Options(), pos)
	pkg.Interfaces = append(pkg.Interfaces, iface)
	stampOneofVariantBackPointers(host, oo, iface.QName(), pos)
}

// stampOneofVariantBackPointers walks the variant fields oo
// declares and stamps [MetaOneofInterface] on each so consumers
// can resolve a variant back to its synthesized interface
// without scanning the package's interface list. The host
// struct is already populated by the caller; the back-pointer
// rides on each Field's meta bag.
func stampOneofVariantBackPointers(
	host *node.Struct,
	oo protoreflect.OneofDescriptor, ifaceQName string, pos position.Pos,
) {
	fields := oo.Fields()
	for i := range fields.Len() {
		f := host.FieldByName(string(fields.Get(i).Name()))
		if f == nil {
			continue
		}
		MetaOneofInterface.SetAt(
			f.EnsureMeta(), ifaceQName,
			meta.AuthorityPlugin, FrontendName, pos,
		)
	}
}

// oneofInterfaceName composes the dot-for-nesting,
// underscore-between-host-and-oneof convention name. For a
// top-level `Container` carrying a oneof named `choice` the
// result is `Container_choice`; for a nested `Outer.Inner`
// carrying a oneof named `choice` the result is
// `Outer.Inner_choice`. joinName isn't reused here because the
// boundary between the message path and the oneof name is an
// underscore, not a dot.
func oneofInterfaceName(hostName, oneofName string) string {
	return hostName + "_" + oneofName
}
