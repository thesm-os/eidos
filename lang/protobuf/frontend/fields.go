// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// fieldOptions extracts the descriptorpb.FieldOptions message
// from desc, or nil when the underlying descriptor carries no
// field-level options. Returning a concrete typed pointer lets
// callers read the standard options through their generated
// getters — `GetDeprecated`, `GetPacked`, `GetJsonName` — rather
// than walking the proto-reflect surface for booleans and strings
// the framework consumes natively.
func fieldOptions(desc protoreflect.FieldDescriptor) *descriptorpb.FieldOptions {
	opts := desc.Options()
	if opts == nil {
		return nil
	}
	fo, ok := opts.(*descriptorpb.FieldOptions)
	if !ok {
		return nil
	}
	return fo
}
