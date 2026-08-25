// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

// wellKnownTypes is the set of `google.protobuf.<name>` types
// the frontend stamps with [MetaWellKnown]. The set mirrors the
// documented well-known catalog: the structural / utility types
// (Timestamp, Duration, Empty, Any, FieldMask, Struct, Value,
// ListValue, NullValue) plus every scalar-wrapper type. Future
// well-knowns extend the set without changing the stamping
// path.
//
//nolint:gochecknoglobals // immutable lookup table.
var wellKnownTypes = map[string]bool{
	"Timestamp":   true,
	"Duration":    true,
	"Empty":       true,
	"Any":         true,
	"FieldMask":   true,
	"Struct":      true,
	"Value":       true,
	"ListValue":   true,
	"NullValue":   true,
	"BoolValue":   true,
	"StringValue": true,
	"Int32Value":  true,
	"Int64Value":  true,
	"UInt32Value": true,
	"UInt64Value": true,
	"FloatValue":  true,
	"DoubleValue": true,
	"BytesValue":  true,
}

// wellKnownName returns the bare type name when the qualified
// path identifies a documented well-known type, or the empty
// string when the path is not a well-known. The full-name path
// is the proto FQN (`google.protobuf.Timestamp`); only entries
// in [wellKnownTypes] under the `google.protobuf` package match.
func wellKnownName(pkg, name string) string {
	if pkg != "google.protobuf" {
		return ""
	}
	if !wellKnownTypes[name] {
		return ""
	}
	return name
}
