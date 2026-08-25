// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/protobuf/frontend"
	"go.thesmos.sh/eidos/node"
)

// TestConvert_WellKnownTypes covers the well-known-type
// detection rule: every reference to a documented
// `google.frontend.*` type stamps the bare type name on the
// produced TypeRef via proto.wellknown. Consumers dispatch on
// the bare name without parsing the qualified path.
func TestConvert_WellKnownTypes(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "wellknown", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkgs := collectPackages(t, env)
	main := mainPackage(t, pkgs, "eidos.protobuf.testdata.wellknown")
	event := findStruct(main, "Event")
	if event == nil {
		t.Fatalf("Struct Event missing")
	}
	wrapped := findStruct(main, "Wrapped")
	if wrapped == nil {
		t.Fatalf("Struct Wrapped missing")
	}

	utility := map[string]string{
		"at":       "Timestamp",
		"span":     "Duration",
		"payload":  "Any",
		"mask":     "FieldMask",
		"metadata": "Struct",
		"dynamic":  "Value",
		"items":    "ListValue",
		"done":     "Empty",
		"null":     "NullValue",
	}
	for field, want := range utility {
		t.Run("utility well-known "+want+" stamps proto.wellknown on the ref", func(t *testing.T) {
			t.Parallel()
			f := event.FieldByName(field)
			if f == nil {
				t.Fatalf("field %q missing on Event", field)
			}
			got, ok := frontend.MetaWellKnown.Get(f.Type.Meta())
			if !ok {
				t.Fatalf("proto.wellknown missing on Event.%s (Type=%+v)", field, f.Type)
			}
			if got != want {
				t.Fatalf("proto.wellknown on Event.%s = %q, want %q", field, got, want)
			}
		})
	}

	wrappers := map[string]string{
		"b":   "BoolValue",
		"s":   "StringValue",
		"i32": "Int32Value",
		"i64": "Int64Value",
		"u32": "UInt32Value",
		"u64": "UInt64Value",
		"f32": "FloatValue",
		"f64": "DoubleValue",
		"bs":  "BytesValue",
	}
	for field, want := range wrappers {
		t.Run("scalar-wrapper well-known "+want+" stamps proto.wellknown on the ref", func(t *testing.T) {
			t.Parallel()
			f := wrapped.FieldByName(field)
			if f == nil {
				t.Fatalf("field %q missing on Wrapped", field)
			}
			got, ok := frontend.MetaWellKnown.Get(f.Type.Meta())
			if !ok {
				t.Fatalf("proto.wellknown missing on Wrapped.%s (Type=%+v)", field, f.Type)
			}
			if got != want {
				t.Fatalf("proto.wellknown on Wrapped.%s = %q, want %q", field, got, want)
			}
		})
	}
}

// mainPackage returns the package whose Path matches path. The
// wellknown fixture imports several well-known descriptor sets;
// protocompile surfaces those as sibling packages in the resolved
// descriptor set, so the test filters down to the fixture's own
// package before reaching for its messages.
func mainPackage(t *testing.T, pkgs []*node.Package, path string) *node.Package {
	t.Helper()
	for _, p := range pkgs {
		if p.Path == path {
			return p
		}
	}
	t.Fatalf("package %q missing; got %+v", path, packagePaths(pkgs))
	return nil
}
