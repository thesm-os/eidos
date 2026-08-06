// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package protobuf

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
)

// The protobuf frontend stamps proto-specific facts under the
// `proto.*` namespace. The frontend-provenance marker is the one
// exception: it carries the bare name `frontend` because it is a
// framework-level convention shared across every frontend (the Go
// frontend stamps the same key with value `"golang"`, future
// frontends with their own plugin names). Cross-frontend consumers
// — the protogo bridge annotator, the cross-namespace audit step —
// pivot on the marker to scope language-namespaced meta to the
// correct source.
//
// Per-frontend keys go through [meta.NewKey] (single-owner singletons
// that panic on duplicate registration — the desired behaviour for
// the namespaced `proto.*` surface). The cross-frontend marker key
// goes through [meta.EnsureKey] so multiple frontends can declare
// it independently without an init-order coupling: whichever
// frontend's package initialises first registers the key; the
// later-initialising frontend's [meta.EnsureKey] returns the same
// singleton instead of panicking.
//
// Dynamic per-option keys (proto custom options carrying their full
// dotted name) also go through [meta.EnsureKey] via [OptionMetaKey]
// so each option full-name resolves to a single canonical
// [meta.Key] across the run.
var (
	// MetaFrontend stamps the producing frontend's plugin name on
	// every [node.Package] entry the frontend emits. The value is
	// the string `"protobuf"` for proto-derived packages; the Go
	// frontend stamps the matching `"golang"` on its packages. The
	// marker is the cross-frontend scope mechanism: bridge
	// annotators filter their walk to packages carrying the
	// expected marker value, and the cross-namespace audit
	// assertion proves no `<lang>.*` meta leaks onto sources from
	// other frontends.
	MetaFrontend = meta.EnsureKey(
		"frontend",
		meta.StringParser,
	) //nolint:gochecknoglobals // cross-frontend registry-singleton key

	// MetaFieldNumber stamps the proto tag number on every
	// [node.Field] derived from a proto field.
	MetaFieldNumber = meta.NewKey(
		"proto.field.number",
		meta.IntParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaFieldJSONName stamps the field's resolved JSON name.
	// Defaults to the lowerCamelCase form derived from the source
	// name; overridden by an explicit `[json_name = "..."]`
	// option on the field.
	MetaFieldJSONName = meta.NewKey(
		"proto.field.json_name",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaFieldDeprecated stamps true on fields declared with
	// `[deprecated = true]`. Absent on every other field — the
	// convenience-alias contract is presence-only.
	MetaFieldDeprecated = meta.NewKey(
		"proto.field.deprecated",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaFieldPacked stamps true or false on repeated scalar
	// fields that carry an explicit `[packed = ...]` override.
	// Absent on every field that uses the proto3 default packing.
	MetaFieldPacked = meta.NewKey(
		"proto.field.packed",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaFieldOptional stamps true on fields that use proto3
	// explicit-presence (the `optional` keyword on a scalar field
	// in proto3.15+). Absent on non-optional fields.
	MetaFieldOptional = meta.NewKey(
		"proto.field.optional",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaFieldOneof stamps the name of the oneof group on each
	// member field. Absent on fields that don't belong to a oneof.
	MetaFieldOneof = meta.NewKey(
		"proto.field.oneof",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaFieldTrailingDoc stamps the source-form text of a
	// trailing same-line comment attached to a field. Multi-line
	// trailing comments join with `\n`. Absent on fields without a
	// trailing comment.
	MetaFieldTrailingDoc = meta.NewKey(
		"proto.field.trailing_doc",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaMessageTrailingDoc stamps the trailing comment that
	// follows a `message` declaration's opening brace on the same
	// line. Absent on messages without such a comment.
	MetaMessageTrailingDoc = meta.NewKey(
		"proto.message.trailing_doc",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaMessageReservedNumbers stamps the expanded list of
	// reserved tag numbers on a message — single numbers and
	// range endpoints both contribute. Absent on messages without
	// any reservations.
	MetaMessageReservedNumbers = meta.NewKey(
		"proto.message.reserved.numbers",
		int32SliceParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaMessageReservedNames stamps reserved field names on a
	// message. Absent on messages without any reserved names.
	MetaMessageReservedNames = meta.NewKey(
		"proto.message.reserved.names",
		stringSliceParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaEnumAllowAlias stamps true on enums that declare
	// `option allow_alias = true;`. Absent on every other enum.
	MetaEnumAllowAlias = meta.NewKey(
		"proto.enum.allow_alias",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaEnumReservedNumbers stamps the expanded list of reserved
	// tag numbers on an enum.
	MetaEnumReservedNumbers = meta.NewKey(
		"proto.enum.reserved.numbers",
		int32SliceParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaEnumReservedNames stamps reserved variant names on an
	// enum.
	MetaEnumReservedNames = meta.NewKey(
		"proto.enum.reserved.names",
		stringSliceParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaEnumTrailingDoc stamps the trailing same-line comment
	// attached to an enum declaration. Absent on enums without
	// such a comment.
	MetaEnumTrailingDoc = meta.NewKey(
		"proto.enum.trailing_doc",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaEnumVariantNumber stamps the variant's proto tag number
	// as the natural typed value. Consumers reading the typed
	// numeric form go through this key; the source-form integer
	// string lives on [node.EnumVariant.Value].
	MetaEnumVariantNumber = meta.NewKey(
		"proto.enum_variant.number",
		meta.IntParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaEnumVariantTrailingDoc stamps the trailing same-line
	// comment attached to an enum variant declaration. Absent on
	// variants without such a comment.
	MetaEnumVariantTrailingDoc = meta.NewKey(
		"proto.enum_variant.trailing_doc",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaServiceTrailingDoc stamps the trailing same-line comment
	// attached to a service declaration. Absent on services
	// without such a comment.
	MetaServiceTrailingDoc = meta.NewKey(
		"proto.service.trailing_doc",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaRPCTrailingDoc stamps the trailing same-line comment
	// attached to an `rpc` declaration. Absent on RPCs without
	// such a comment.
	MetaRPCTrailingDoc = meta.NewKey(
		"proto.service.rpc.trailing_doc",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaRPCStreamClient stamps true on methods whose request
	// side is declared `stream`. Absent on unary and
	// server-streaming RPCs.
	MetaRPCStreamClient = meta.NewKey(
		"proto.service.rpc.stream.client",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaRPCStreamServer stamps true on methods whose response
	// side is declared `stream`. Absent on unary and
	// client-streaming RPCs.
	MetaRPCStreamServer = meta.NewKey(
		"proto.service.rpc.stream.server",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaOneofInterface stamps the qualified name of the
	// synthesized [node.Interface] on every variant [node.Field]
	// belonging to a oneof group. Consumers walk from a variant
	// field to its interface by reading this key.
	MetaOneofInterface = meta.NewKey(
		"proto.oneof.interface",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaOneofMessage stamps the qualified name of the host
	// [node.Struct] on the synthesized oneof [node.Interface].
	// Consumers walk from the interface back to its host message
	// by reading this key.
	MetaOneofMessage = meta.NewKey(
		"proto.oneof.message",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaOneofTrailingDoc stamps the trailing same-line comment
	// attached to a oneof's opening brace on the synthesized
	// [node.Interface]. Lives under its own per-host-kind key
	// rather than reusing the message-level surface so consumers
	// that filter trailing-doc by host kind see the oneof entries
	// without dispatching on shape.
	MetaOneofTrailingDoc = meta.NewKey(
		"proto.oneof.trailing_doc",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaWellKnown stamps the bare name of a well-known type on
	// every [node.TypeRef] that references one. Consumers dispatch
	// on the bare name (`Timestamp`, `Duration`, `Any`, …) without
	// parsing the qualified `google.protobuf.<name>` path.
	MetaWellKnown = meta.NewKey(
		"proto.wellknown",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key
)

// int32SliceParser decodes a JSON-encoded int32 slice from raw.
// Used as the [meta.Parser] for the reserved-numbers keys whose
// stored payload is a typed slice; the parser is only invoked on
// the cache-roundtrip path, where the bag was JSON-marshalled and
// is lazy-decoded back into the typed form.
func int32SliceParser(raw string) ([]int32, error) {
	if raw == "" {
		return nil, nil
	}
	var out []int32
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("protobuf: decode int32 slice: %w", err)
	}
	return out, nil
}

// stringSliceParser decodes a JSON-encoded string slice from raw.
// Mirrors [int32SliceParser] for the reserved-names keys.
func stringSliceParser(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("protobuf: decode string slice: %w", err)
	}
	return out, nil
}

// MetaOptionPrefix is the namespace under which proto custom-option
// entries are stamped on host meta bags. For a custom option named
// `my.package.feature_flag`, the converter stamps
// `proto.option.my.package.feature_flag=<typed value>` per the
// option-channel convention. Convenience aliases for high-traffic
// standard options live under sibling `proto.*` keys (e.g.
// `proto.deprecated`, `proto.json_name`); the raw form under
// MetaOptionPrefix is always present alongside the alias.
const MetaOptionPrefix = "proto.option."

// OptionMetaKey returns the typed [meta.Key] for the proto custom
// option whose full-name is name. The returned key lives under
// [MetaOptionPrefix]; the supplied parser determines the typed Go
// value the resolved option carries — signed-int width scalars,
// unsigned-int width scalars, floats, bool, string, bytes,
// enum-name strings, message-shaped payloads, and their composite
// forms. Dynamic names go through [meta.EnsureKey] so the registry
// retains exactly one canonical key per option full-name even when
// the converter visits the same option from multiple call sites.
func OptionMetaKey[T any](name string, parser meta.Parser[T]) meta.Key[T] {
	return meta.EnsureKey(MetaOptionPrefix+name, parser)
}

// optionKeyName returns the canonical option-channel key fragment
// for field. Custom extensions stamp under their proto FQN;
// standard options on google.protobuf.FileOptions and friends
// stamp under the short well-known name (`go_package`,
// `deprecated`, `allow_alias`, …).
func optionKeyName(field protoreflect.FieldDescriptor) string {
	if field.IsExtension() {
		return string(field.FullName())
	}
	return string(field.Name())
}

// stampFrontendMarker records the cross-frontend provenance marker
// on pkg's meta bag. Every produced package carries this stamp;
// bridge annotators and the cross-namespace audit step pivot on it
// to scope their walks to the correct frontend's sources.
func stampFrontendMarker(pkg *node.Package) {
	MetaFrontend.Set(pkg.EnsureMeta(), FrontendName, FrontendName)
}
