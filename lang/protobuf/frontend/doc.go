// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package frontend is the proto3 frontend for eidos. It loads
// `.proto` sources via [github.com/bufbuild/protocompile] and
// converts the resulting descriptor set into the language-agnostic
// [node] model — messages, enums, services, oneofs, maps, repeated
// fields, nested types, well-known references, custom options,
// reserved ranges, and comments all surface as the corresponding
// node kinds with proto-specific facts stamped under the `proto.*`
// meta namespace.
//
// # Scope
//
// The frontend supports `syntax = "proto3";` only. Sources whose
// first declaration is `edition = "..."` (the proto editions
// migration path, edition 2023 and later) surface a positioned
// `diag.Error` and the file is skipped; remaining files in the
// same load continue to parse. proto2 is not supported.
//
// # Pipeline integration
//
// The frontend implements [plugin.Frontend] and is registered on a
// [pipeline.Builder] via [pipeline.Builder.WithFrontend]:
//
//	pipeline.New().
//	    WithFrontend(protobuf.New()).
//	    WithBackend(backend_golang.New()).
//	    Build()
//
// Load is invoked once per [plugin.FrontendContext.Pattern]. Each
// pattern is interpreted as a proto-source root or import path;
// protocompile's `Compiler` resolves transitive imports against
// the configured `import_paths` plus the search root.
//
// # Output vocabulary
//
// The frontend produces source-side [node.Package] entries keyed by
// the proto package qualifier on `Path` and the last dotted
// segment on `Name`. Type-reference values in `TypeRef.Name` carry
// the proto source surface verbatim (e.g. `int32`, `sint32`,
// `fixed64`, `float`, `bytes`); cross-language translation is the
// `protogo` bridge annotator's responsibility (see
// `bridge/protogo`), not the frontend's.
//
// # Meta-key catalog
//
// Every proto-specific fact the frontend records lands under the
// `proto.*` meta namespace. The catalog below pairs each key
// with the host kind it stamps on and the value semantic. The
// dynamic per-option keys ride under [MetaOptionPrefix] with the
// option's proto FQN as the suffix.
//
//   - frontend — cross-frontend provenance marker on every
//     produced [node.Package]; value is the frontend's plugin
//     name (`"protobuf"`).
//   - proto.field.number — declared tag number on every field.
//   - proto.field.json_name — resolved JSON name on every field
//     (lowerCamelCase derivation or the `[json_name = ...]`
//     override).
//   - proto.field.deprecated — present on fields carrying
//     `[deprecated = true]`.
//   - proto.field.packed — present on repeated scalar fields
//     with an explicit `[packed = ...]` override.
//   - proto.field.optional — present on proto3-explicit-presence
//     fields (the `optional` keyword).
//   - proto.field.oneof — present on oneof-member fields; value
//     is the host oneof's name.
//   - proto.field.trailing_doc — trailing same-line comment on a
//     field declaration.
//   - proto.message.reserved.numbers — flat integer list of
//     reserved tag numbers on a message; ranges expanded.
//   - proto.message.reserved.names — list of reserved field
//     names on a message.
//   - proto.message.trailing_doc — trailing same-line comment on
//     a message declaration.
//   - proto.enum.allow_alias — present on enums carrying
//     `[allow_alias = true]`.
//   - proto.enum.reserved.numbers — flat list of reserved enum
//     values; ranges expanded.
//   - proto.enum.reserved.names — list of reserved variant names.
//   - proto.enum.trailing_doc — trailing same-line comment on an
//     enum declaration.
//   - proto.enum_variant.number — typed numeric value on every
//     variant; sibling to the source-form string on
//     [node.EnumVariant.Value].
//   - proto.enum_variant.trailing_doc — trailing same-line
//     comment on a variant declaration.
//   - proto.oneof.message — qualified name of the host message
//     on the synthesized oneof interface.
//   - proto.oneof.interface — qualified name of the synthesized
//     oneof interface on every variant field.
//   - proto.oneof.trailing_doc — trailing same-line comment on
//     a oneof block.
//   - proto.service.trailing_doc — trailing same-line comment on
//     a service declaration.
//   - proto.service.rpc.stream.client — present on RPCs whose
//     request side is `stream`.
//   - proto.service.rpc.stream.server — present on RPCs whose
//     response side is `stream`.
//   - proto.service.rpc.trailing_doc — trailing same-line comment
//     on an RPC declaration.
//   - proto.wellknown — bare type name on every well-known
//     reference (`Timestamp`, `Duration`, `Any`, etc.) so
//     consumers dispatch without parsing the qualified path.
//   - proto.option.<full-name> — custom and standard option
//     values, keyed by the option's proto FQN; per-option value
//     type follows the documented value-type table.
//
// # Bridge annotator
//
// Proto→Go pipelines pair the frontend with the `protogo`
// annotator. protogo stamps `go.*` meta on every proto-derived
// type-ref-bearing host (field, method param, method return);
// the Go backend's render-site rules prefer the `go.*` meta over
// the underlying proto-native name. Pipelines targeting non-Go
// outputs swap the bridge for the matching language pair (a
// future `protorust` / `prototypescript`) or skip the bridge
// entirely for proto-native targets.
package frontend
