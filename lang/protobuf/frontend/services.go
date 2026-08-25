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

// convertServices walks fd's service declarations and appends one
// [node.Interface] per service to pkg. Each contained rpc becomes
// a [node.Method] child with single-entry Params and Returns
// slices carrying the request and response message refs. The
// method's Receiver is nil and ReceiverName is empty — the
// framework's interface-method convention says the receiver is
// implicit in the interface.
//
// Per-method streaming meta records the four flavours (unary,
// client-stream, server-stream, bidirectional) via the documented
// proto.service.rpc.stream.client / .server keys. Leading and
// trailing comments and `+gen:` directives flow through the same
// attachment surface message and enum declarations use.
func convertServices(ctx *plugin.FrontendContext, pkg *node.Package, fd protoreflect.FileDescriptor) {
	services := fd.Services()
	for i := range services.Len() {
		appendServiceInterface(ctx, pkg, fd, services.Get(i))
	}
}

// appendServiceInterface converts sd into a [node.Interface] on
// pkg with one [node.Method] per declared rpc. The method ordering
// matches source order; per-method meta lands inline.
func appendServiceInterface(
	ctx *plugin.FrontendContext, pkg *node.Package,
	fd protoreflect.FileDescriptor, sd protoreflect.ServiceDescriptor,
) {
	iface := &node.Interface{
		BaseNode: node.BaseNode{SourcePos: sourcePos(fd, sd)},
		Name:     string(sd.Name()),
		Package:  pkg.Path,
		Methods:  convertRPCs(ctx, fd, sd),
	}
	attachInterfaceDocs(ctx, iface, fd, sd)
	stampHostOptions(ctx.Diag.For(FrontendName), iface.EnsureMeta(), sd.Options(), sourcePos(fd, sd))
	pkg.Interfaces = append(pkg.Interfaces, iface)
}

// convertRPCs returns one [node.Method] per rpc declared on sd
// in source order. Each method carries the request type on Params
// and the response type on Returns; streaming flavours stamp the
// matching client/server stream meta.
func convertRPCs(
	ctx *plugin.FrontendContext, fd protoreflect.FileDescriptor, sd protoreflect.ServiceDescriptor,
) []*node.Method {
	methods := sd.Methods()
	count := methods.Len()
	if count == 0 {
		return nil
	}
	out := make([]*node.Method, 0, count)
	for i := range count {
		md := methods.Get(i)
		pos := sourcePos(fd, md)
		m := &node.Method{
			BaseNode: node.BaseNode{SourcePos: pos},
			Name:     string(md.Name()),
			Params: []*node.Param{{
				Type: namedTypeRef(md.Input().ParentFile(), md.Input().FullName()),
			}},
			// Proto has no named results: an RPC declares one
			// output message and no binding for it.
			Returns: node.AnonReturns(
				namedTypeRef(md.Output().ParentFile(), md.Output().FullName()),
			),
		}
		stampStreamingMeta(m, md, pos)
		attachRPCDocs(ctx, m, fd, md)
		stampHostOptions(ctx.Diag.For(FrontendName), m.EnsureMeta(), md.Options(), pos)
		out = append(out, m)
	}
	return out
}

// stampStreamingMeta records the per-RPC streaming-flavour meta:
// proto.service.rpc.stream.client stamps true when the request
// side is `stream`; proto.service.rpc.stream.server stamps true
// when the response side is `stream`. Bidirectional streams set
// both keys; unary RPCs carry neither.
func stampStreamingMeta(m *node.Method, md protoreflect.MethodDescriptor, pos position.Pos) {
	if md.IsStreamingClient() {
		MetaRPCStreamClient.SetAt(m.EnsureMeta(), true, meta.AuthorityPlugin, FrontendName, pos)
	}
	if md.IsStreamingServer() {
		MetaRPCStreamServer.SetAt(m.EnsureMeta(), true, meta.AuthorityPlugin, FrontendName, pos)
	}
}
