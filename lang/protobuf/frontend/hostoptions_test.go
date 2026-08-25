// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/lang/protobuf/frontend"
	"go.thesmos.sh/eidos/node"
)

// TestConvert_HostOptionStamping covers the per-host
// option-channel surface: every proto3 host kind that carries
// custom options (message, field, oneof, enum, enum-variant,
// service, method) sees the stamped value on its meta bag
// under proto.option.<full-name>.
func TestConvert_HostOptionStamping(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "hostoptions", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkgs := collectPackages(t, env)
	pkg := mainPackage(t, pkgs, "eidos.protobuf.testdata.hostoptions")

	const prefix = "eidos.protobuf.testdata.hostoptions."

	t.Run("message option stamps on the Struct meta bag", func(t *testing.T) {
		t.Parallel()
		doc := findStruct(pkg, "Document")
		got := requireStringOption(t, doc.Meta(), prefix+"message_tag")
		if got != "doc-host" {
			t.Fatalf("message_tag = %q, want %q", got, "doc-host")
		}
	})

	t.Run("field option stamps on the Field meta bag", func(t *testing.T) {
		t.Parallel()
		doc := findStruct(pkg, "Document")
		id := doc.FieldByName("id")
		got := requireStringOption(t, id.Meta(), prefix+"field_tag")
		if got != "id-host" {
			t.Fatalf("field_tag = %q, want %q", got, "id-host")
		}
	})

	t.Run("oneof option stamps on the synthesized Interface meta bag", func(t *testing.T) {
		t.Parallel()
		iface := findInterface(pkg, "Document_payload")
		if iface == nil {
			t.Fatalf("Interface Document_payload missing")
		}
		got := requireStringOption(t, iface.Meta(), prefix+"oneof_tag")
		if got != "payload-host" {
			t.Fatalf("oneof_tag = %q, want %q", got, "payload-host")
		}
	})

	t.Run("enum option stamps on the Enum meta bag", func(t *testing.T) {
		t.Parallel()
		color := findEnum(pkg, "Color")
		got := requireStringOption(t, color.Meta(), prefix+"enum_tag")
		if got != "color-host" {
			t.Fatalf("enum_tag = %q, want %q", got, "color-host")
		}
	})

	t.Run("enum-variant option stamps on the EnumVariant meta bag", func(t *testing.T) {
		t.Parallel()
		color := findEnum(pkg, "Color")
		var red *node.EnumVariant
		for _, v := range color.Variants {
			if v.Name == "COLOR_RED" {
				red = v
				break
			}
		}
		if red == nil {
			t.Fatalf("variant COLOR_RED missing")
		}
		got := requireStringOption(t, red.Meta(), prefix+"variant_tag")
		if got != "red-variant" {
			t.Fatalf("variant_tag = %q, want %q", got, "red-variant")
		}
	})

	t.Run("service option stamps on the Interface meta bag", func(t *testing.T) {
		t.Parallel()
		svc := findInterface(pkg, "Greeter")
		got := requireStringOption(t, svc.Meta(), prefix+"service_tag")
		if got != "greeter-host" {
			t.Fatalf("service_tag = %q, want %q", got, "greeter-host")
		}
	})

	t.Run("method option stamps on the Method meta bag", func(t *testing.T) {
		t.Parallel()
		svc := findInterface(pkg, "Greeter")
		hello := svc.MethodByName("Hello")
		got := requireStringOption(t, hello.Meta(), prefix+"method_tag")
		if got != "hello-host" {
			t.Fatalf("method_tag = %q, want %q", got, "hello-host")
		}
	})
}

// TestConvert_OptionValueTypes covers the documented value-type
// table for custom-option values: signed-int widening to int64,
// unsigned-int widening to uint64, float/double to float64,
// bytes to []byte, enum to variant Name string. Each form rides
// on a file-level custom option in the hostoptions fixture.
func TestConvert_OptionValueTypes(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "hostoptions", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkgs := collectPackages(t, env)
	pkg := mainPackage(t, pkgs, "eidos.protobuf.testdata.hostoptions")
	const prefix = "eidos.protobuf.testdata.hostoptions."

	t.Run("int32 option widens to int64", func(t *testing.T) {
		t.Parallel()
		key := meta.EnsureKey(frontend.MetaOptionPrefix+prefix+"v_int32", frontend.Int64Parser)
		got, ok := key.Get(pkg.Meta())
		if !ok {
			t.Fatalf("v_int32 not stamped")
		}
		if got != -7 {
			t.Fatalf("v_int32 = %d, want -7", got)
		}
	})

	t.Run("int64 option carries int64", func(t *testing.T) {
		t.Parallel()
		key := meta.EnsureKey(frontend.MetaOptionPrefix+prefix+"v_int64", frontend.Int64Parser)
		got, ok := key.Get(pkg.Meta())
		if !ok {
			t.Fatalf("v_int64 not stamped")
		}
		if got != 9876543210 {
			t.Fatalf("v_int64 = %d, want 9876543210", got)
		}
	})

	t.Run("uint32 / uint64 widen to uint64", func(t *testing.T) {
		t.Parallel()
		k32 := meta.EnsureKey(frontend.MetaOptionPrefix+prefix+"v_uint32", frontend.Uint64Parser)
		got32, ok := k32.Get(pkg.Meta())
		if !ok || got32 != 42 {
			t.Fatalf("v_uint32 = (%d, %v), want 42", got32, ok)
		}
		k64 := meta.EnsureKey(frontend.MetaOptionPrefix+prefix+"v_uint64", frontend.Uint64Parser)
		got64, ok := k64.Get(pkg.Meta())
		if !ok || got64 != 18446744073709551610 {
			t.Fatalf("v_uint64 = (%d, %v)", got64, ok)
		}
	})

	t.Run("float / double both surface as float64", func(t *testing.T) {
		t.Parallel()
		kf := meta.EnsureKey(frontend.MetaOptionPrefix+prefix+"v_float", frontend.Float64Parser)
		gotF, ok := kf.Get(pkg.Meta())
		if !ok || gotF != 3.5 {
			t.Fatalf("v_float = (%v, %v), want 3.5", gotF, ok)
		}
		kd := meta.EnsureKey(frontend.MetaOptionPrefix+prefix+"v_double", frontend.Float64Parser)
		gotD, ok := kd.Get(pkg.Meta())
		if !ok || gotD < 2.718 || gotD > 2.719 {
			t.Fatalf("v_double = (%v, %v), want ~2.71828", gotD, ok)
		}
	})

	t.Run("bytes option carries []byte", func(t *testing.T) {
		t.Parallel()
		key := meta.EnsureKey(frontend.MetaOptionPrefix+prefix+"v_bytes", frontend.BytesParser)
		got, ok := key.Get(pkg.Meta())
		if !ok {
			t.Fatalf("v_bytes not stamped")
		}
		if string(got) != "abc" {
			t.Fatalf("v_bytes = %q, want %q", string(got), "abc")
		}
	})

	t.Run("enum option carries the variant Name string", func(t *testing.T) {
		t.Parallel()
		key := meta.EnsureKey(frontend.MetaOptionPrefix+prefix+"v_enum", meta.StringParser)
		got, ok := key.Get(pkg.Meta())
		if !ok {
			t.Fatalf("v_enum not stamped")
		}
		if got != "MOOD_FOCUSED" {
			t.Fatalf("v_enum = %q, want %q", got, "MOOD_FOCUSED")
		}
	})
}

// TestConvert_OptionValueTypes_Composite covers the recursive
// composite value-type paths through messageValueToGo,
// listValueToGo, and mapValueToGo. The fixture's Document
// carries a single-message option on `id` (validate), a
// repeated string option on the message (tags), and a map
// option on the message (labels). Each form decodes to the
// natural Go container per the documented value-type table.
func TestConvert_OptionValueTypes_Composite(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "hostoptions", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkgs := collectPackages(t, env)
	pkg := mainPackage(t, pkgs, "eidos.protobuf.testdata.hostoptions")
	const prefix = "eidos.protobuf.testdata.hostoptions."

	t.Run("single-message field option decodes to a recursive map[string]any", func(t *testing.T) {
		t.Parallel()
		doc := findStruct(pkg, "Document")
		id := doc.FieldByName("id")
		key := meta.EnsureKey(frontend.MetaOptionPrefix+prefix+"validate", frontend.MapAnyParser)
		got, ok := key.Get(id.Meta())
		if !ok {
			t.Fatalf("validate option not stamped on Document.id")
		}
		if got["pattern"] != "^[a-z]+$" {
			t.Fatalf("validate.pattern = %v, want %q", got["pattern"], "^[a-z]+$")
		}
		// Scalar int subfield collapses to int64 per the
		// widening rule.
		if got["max_len"] != int64(64) {
			t.Fatalf("validate.max_len = %v (%T), want int64(64)", got["max_len"], got["max_len"])
		}
		examples, ok := got["examples"].([]any)
		if !ok {
			t.Fatalf("validate.examples not a []any; got %T", got["examples"])
		}
		if len(examples) != 2 || examples[0] != "a" || examples[1] != "b" {
			t.Fatalf("validate.examples = %+v, want [\"a\", \"b\"]", examples)
		}
	})

	t.Run("repeated message option decodes to []any in source order", func(t *testing.T) {
		t.Parallel()
		doc := findStruct(pkg, "Document")
		key := meta.EnsureKey(frontend.MetaOptionPrefix+prefix+"tags", frontend.SliceAnyParser)
		got, ok := key.Get(doc.Meta())
		if !ok {
			t.Fatalf("tags option not stamped on Document")
		}
		if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
			t.Fatalf("tags = %+v, want [\"alpha\", \"beta\"]", got)
		}
	})

	t.Run("nested map<K,V> within a message option decodes to map[string]any", func(t *testing.T) {
		t.Parallel()
		doc := findStruct(pkg, "Document")
		id := doc.FieldByName("id")
		key := meta.EnsureKey(frontend.MetaOptionPrefix+prefix+"validate", frontend.MapAnyParser)
		got, ok := key.Get(id.Meta())
		if !ok {
			t.Fatalf("validate option not stamped on Document.id")
		}
		notes, ok := got["notes"].(map[string]any)
		if !ok {
			t.Fatalf("validate.notes not a map[string]any; got %T", got["notes"])
		}
		if notes["owner"] != "platform" {
			t.Fatalf("validate.notes[owner] = %v, want %q", notes["owner"], "platform")
		}
	})
}

// TestConvert_EnumOptionResolutionRecipe covers the documented
// resolution recipe end-to-end: read the option's variant-name
// string, locate the enum on the store reader, then resolve the
// variant by name to reach both the source-form Value field and
// the typed numeric meta.
func TestConvert_EnumOptionResolutionRecipe(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "hostoptions", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkgs := collectPackages(t, env)
	pkg := mainPackage(t, pkgs, "eidos.protobuf.testdata.hostoptions")
	const prefix = "eidos.protobuf.testdata.hostoptions."

	t.Run("file option (v_enum) resolves to the matching Mood variant via name", func(t *testing.T) {
		t.Parallel()
		// Step 1: read the option's variant-name string.
		key := meta.EnsureKey(frontend.MetaOptionPrefix+prefix+"v_enum", meta.StringParser)
		variantName, ok := key.Get(pkg.Meta())
		if !ok {
			t.Fatalf("v_enum option missing")
		}
		// Step 2: locate the enum on the store-side reader-equivalent.
		mood := findEnum(pkg, "Mood")
		if mood == nil {
			t.Fatalf("Enum Mood missing")
		}
		// Step 3: resolve the variant by name.
		variant := mood.VariantByName(variantName)
		if variant == nil {
			t.Fatalf("variant %q missing on Mood", variantName)
		}
		// Step 4: the variant exposes both the source-form Value
		// field and the typed numeric meta.
		if variant.Value != "2" {
			t.Fatalf("MOOD_FOCUSED.Value = %q, want %q", variant.Value, "2")
		}
		num, ok := frontend.MetaEnumVariantNumber.Get(variant.Meta())
		if !ok || num != 2 {
			t.Fatalf("MOOD_FOCUSED proto.enum_variant.number = (%d, %v), want 2", num, ok)
		}
	})
}

// requireStringOption looks up the option key on bag and fails
// the test when the key is absent or carries a non-string value.
// The helper centralises the per-test boilerplate so each
// option-stamping subtest reads as one assertion line.
func requireStringOption(t *testing.T, bag *meta.Bag, name string) string {
	t.Helper()
	key := meta.EnsureKey(frontend.MetaOptionPrefix+name, meta.StringParser)
	got, ok := key.Get(bag)
	if !ok {
		t.Fatalf("option %q not stamped on bag", name)
	}
	return got
}
