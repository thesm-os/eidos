// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/lang/protobuf/frontend"
	"go.thesmos.sh/eidos/node"
)

// TestConvert_Messages covers the message → node.Struct mapping:
// each `message X { ... }` in a proto source produces one
// node.Struct named X in the producing package. Fields land in
// declaration order; scalar field types appear verbatim in the
// type-ref's Name field.
func TestConvert_Messages(t *testing.T) {
	t.Parallel()

	t.Run("Greeter from the simple fixture produces a node.Struct with one string field", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "simple", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkg := requireSinglePackage(t, env)
		if got := len(pkg.Structs); got != 1 {
			t.Fatalf("expected exactly 1 struct; got %d", got)
		}
		s := pkg.Structs[0]
		if s.Name != "Greeter" {
			t.Fatalf("Struct.Name = %q, want %q", s.Name, "Greeter")
		}
		if s.Package != pkg.Path {
			t.Fatalf("Struct.Package = %q, want %q", s.Package, pkg.Path)
		}
		if got := len(s.Fields); got != 1 {
			t.Fatalf("expected 1 field on Greeter; got %d", got)
		}
		f := s.Fields[0]
		if f.Name != "name" {
			t.Fatalf("Field.Name = %q, want %q", f.Name, "name")
		}
		if f.Type == nil {
			t.Fatalf("Field.Type is nil; want a TypeRef with Name=%q", "string")
		}
		if f.Type.Name != "string" {
			t.Fatalf("Field.Type.Name = %q, want %q (proto scalar verbatim)", f.Type.Name, "string")
		}
	})
}

// TestConvert_Messages_NestedDotJoined covers the nested-message
// emission rule: a message declared inside another message lands
// as an additional node.Struct whose Name is the parent's name
// joined to the child by a dot. The fixture's User message
// contains a nested Profile; the converter must emit both User
// and User.Profile in the same package.
func TestConvert_Messages_NestedDotJoined(t *testing.T) {
	t.Parallel()

	t.Run("nested message produces a dot-joined Struct in the same package", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "messages", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkg := requireSinglePackage(t, env)
		if findStruct(pkg, "User") == nil {
			t.Fatalf("expected a Struct named %q; got %+v", "User", structNames(pkg))
		}
		nested := findStruct(pkg, "User.Profile")
		if nested == nil {
			t.Fatalf("expected a Struct named %q; got %+v", "User.Profile", structNames(pkg))
		}
		if nested.Package != pkg.Path {
			t.Fatalf("nested Struct.Package = %q, want %q", nested.Package, pkg.Path)
		}
	})
}

// TestConvert_Messages_SourcePos covers the BaseNode.SourcePos
// contract for proto-derived structs and fields: every produced
// node carries the source file it was declared in, plus a
// non-zero line, so downstream consumers (the backend's Source:
// header attribution, `eidos explain`, the manifest's
// provenance attribution) can resolve each entity back to its
// declaration site.
func TestConvert_Messages_SourcePos(t *testing.T) {
	t.Parallel()

	t.Run("each Struct and Field carries the originating proto file path and line", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "messages", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkg := requireSinglePackage(t, env)
		if len(pkg.Structs) == 0 {
			t.Fatalf("expected at least one Struct in the messages fixture")
		}
		for _, s := range pkg.Structs {
			pos := s.Pos()
			if pos.File == "" {
				t.Errorf("Struct %q carries empty Pos.File", s.Name)
			}
			if pos.Line == 0 {
				t.Errorf("Struct %q carries zero Pos.Line", s.Name)
			}
			for _, f := range s.Fields {
				fp := f.Pos()
				if fp.File == "" {
					t.Errorf("Field %q on %q carries empty Pos.File", f.Name, s.Name)
				}
				if fp.Line == 0 {
					t.Errorf("Field %q on %q carries zero Pos.Line", f.Name, s.Name)
				}
			}
		}
	})
}

// TestConvert_FieldTagAndJSONName covers the per-field tag-number
// and JSON-name meta. Every field carries proto.field.number with
// its declared tag, and every field carries proto.field.json_name
// — the default lowerCamelCase derivation OR the explicit override
// declared with `[json_name = "..."]`.
func TestConvert_FieldTagAndJSONName(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "messages", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkg := requireSinglePackage(t, env)
	user := findStruct(pkg, "User")
	if user == nil {
		t.Fatalf("Struct %q missing", "User")
	}

	t.Run("proto.field.number stamps the declared tag on every field", func(t *testing.T) {
		t.Parallel()
		cases := map[string]int{
			"id":            1,
			"legacy_field":  2,
			"display_name":  3,
			"optional_note": 4,
			"status":        5,
			"profile":       6,
		}
		for name, want := range cases {
			f := user.FieldByName(name)
			if f == nil {
				t.Fatalf("field %q missing on User", name)
			}
			got, ok := frontend.MetaFieldNumber.Get(f.Meta())
			if !ok {
				t.Fatalf("proto.field.number missing on %q", name)
			}
			if got != want {
				t.Fatalf("proto.field.number on %q = %d, want %d", name, got, want)
			}
		}
	})

	t.Run("proto.field.json_name carries the override when [json_name = ...] is set", func(t *testing.T) {
		t.Parallel()
		f := user.FieldByName("display_name")
		got, ok := frontend.MetaFieldJSONName.Get(f.Meta())
		if !ok {
			t.Fatalf("proto.field.json_name missing on %q", "display_name")
		}
		const want = "displayName"
		if got != want {
			t.Fatalf("proto.field.json_name on %q = %q, want %q", "display_name", got, want)
		}
	})

	t.Run("proto.field.json_name defaults to the lowerCamelCase form when no override is declared", func(t *testing.T) {
		t.Parallel()
		f := user.FieldByName("legacy_field")
		got, ok := frontend.MetaFieldJSONName.Get(f.Meta())
		if !ok {
			t.Fatalf("proto.field.json_name missing on %q", "legacy_field")
		}
		const want = "legacyField"
		if got != want {
			t.Fatalf("proto.field.json_name on %q = %q, want %q", "legacy_field", got, want)
		}
	})
}

// TestConvert_FieldFlagMeta covers the presence-based per-field
// meta keys: deprecated, optional, and oneof. Each key is stamped
// only on fields where it applies; the absence of the key means
// the underlying proto flag is unset (proto3's no-presence
// default).
func TestConvert_FieldFlagMeta(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "messages", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkg := requireSinglePackage(t, env)

	t.Run("proto.field.deprecated stamps true only on deprecated fields", func(t *testing.T) {
		t.Parallel()
		user := findStruct(pkg, "User")
		legacy := user.FieldByName("legacy_field")
		got, ok := frontend.MetaFieldDeprecated.Get(legacy.Meta())
		if !ok || !got {
			t.Fatalf("expected proto.field.deprecated = true on legacy_field; got (%v, %v)", got, ok)
		}
		id := user.FieldByName("id")
		if _, ok := frontend.MetaFieldDeprecated.Get(id.Meta()); ok {
			t.Fatalf("non-deprecated field id should not carry proto.field.deprecated")
		}
		// Coexistence: the convenience alias and the raw option
		// channel both stamp. Plugins targeting the alias path get
		// the typed bool; plugins iterating proto.option.* see the
		// same value under the standard name.
		raw := meta.EnsureKey(frontend.MetaOptionPrefix+"deprecated", meta.BoolParser)
		rawVal, rawOK := raw.Get(legacy.Meta())
		if !rawOK || !rawVal {
			t.Fatalf(
				"expected proto.option.deprecated = true alongside the alias; got (%v, %v)",
				rawVal, rawOK,
			)
		}
	})

	t.Run("proto.field.optional stamps true only on proto3-explicit-presence fields", func(t *testing.T) {
		t.Parallel()
		user := findStruct(pkg, "User")
		note := user.FieldByName("optional_note")
		got, ok := frontend.MetaFieldOptional.Get(note.Meta())
		if !ok || !got {
			t.Fatalf("expected proto.field.optional = true on optional_note; got (%v, %v)", got, ok)
		}
		id := user.FieldByName("id")
		if _, ok := frontend.MetaFieldOptional.Get(id.Meta()); ok {
			t.Fatalf("non-optional field id should not carry proto.field.optional")
		}
	})

	t.Run("proto.field.packed stamps the explicit override; absent otherwise", func(t *testing.T) {
		t.Parallel()
		container := findStruct(pkg, "Container")
		if container == nil {
			t.Fatalf("Struct %q missing", "Container")
		}
		tags := container.FieldByName("tags")
		if tags == nil {
			t.Fatalf("field tags missing on Container")
		}
		got, ok := frontend.MetaFieldPacked.Get(tags.Meta())
		if !ok {
			t.Fatalf("expected proto.field.packed stamp on tags (the [packed = false] override); not present")
		}
		if got {
			t.Fatalf("proto.field.packed on tags = true, want false (matches the [packed = false] override)")
		}
		user := findStruct(pkg, "User")
		id := user.FieldByName("id")
		if _, ok := frontend.MetaFieldPacked.Get(id.Meta()); ok {
			t.Fatalf("non-repeated field id should not carry proto.field.packed")
		}
	})

	t.Run("proto.field.oneof names the parent oneof group on oneof-member fields", func(t *testing.T) {
		t.Parallel()
		container := findStruct(pkg, "Container")
		if container == nil {
			t.Fatalf("Struct %q missing", "Container")
		}
		text := container.FieldByName("text")
		number := container.FieldByName("number")
		for _, f := range []*node.Field{text, number} {
			got, ok := frontend.MetaFieldOneof.Get(f.Meta())
			if !ok {
				t.Fatalf("expected proto.field.oneof on %q; not present", f.Name)
			}
			if got != "choice" {
				t.Fatalf("proto.field.oneof on %q = %q, want %q", f.Name, got, "choice")
			}
		}
		tag := container.FieldByName("tag")
		if _, ok := frontend.MetaFieldOneof.Get(tag.Meta()); ok {
			t.Fatalf("non-oneof field tag should not carry proto.field.oneof")
		}
	})
}

// TestConvert_FieldNamedTypes covers message-typed and enum-typed
// field references: the field's TypeRef is a TypeRefNamed whose
// Name is the dot-joined name (for nested messages) and whose
// Package is the qualifier of the declaring file.
func TestConvert_FieldNamedTypes(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "messages", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkg := requireSinglePackage(t, env)
	user := findStruct(pkg, "User")

	t.Run("message-typed field references the dot-joined nested name", func(t *testing.T) {
		t.Parallel()
		profile := user.FieldByName("profile")
		if profile.Type.TypeKind != node.TypeRefNamed {
			t.Fatalf("profile field TypeKind = %v, want TypeRefNamed", profile.Type.TypeKind)
		}
		const wantName = "User.Profile"
		if profile.Type.Name != wantName {
			t.Fatalf("profile field Type.Name = %q, want %q", profile.Type.Name, wantName)
		}
		if profile.Type.Package != pkg.Path {
			t.Fatalf("profile field Type.Package = %q, want %q", profile.Type.Package, pkg.Path)
		}
	})

	t.Run("enum-typed field references the enum's local name", func(t *testing.T) {
		t.Parallel()
		status := user.FieldByName("status")
		if status.Type.TypeKind != node.TypeRefNamed {
			t.Fatalf("status field TypeKind = %v, want TypeRefNamed", status.Type.TypeKind)
		}
		const wantName = "Status"
		if status.Type.Name != wantName {
			t.Fatalf("status field Type.Name = %q, want %q", status.Type.Name, wantName)
		}
		if status.Type.Package != pkg.Path {
			t.Fatalf("status field Type.Package = %q, want %q", status.Type.Package, pkg.Path)
		}
	})
}

// TestConvert_Messages_NestedNameCollision pins the dot-joined
// naming rule under leaf-name collision: two nested messages
// declared in different outer messages may share a leaf name
// without colliding in the produced Structs slice. User.Profile
// and Org.Profile both surface as distinct dot-joined entries.
func TestConvert_Messages_NestedNameCollision(t *testing.T) {
	t.Parallel()

	t.Run("two outers with same-named nested message produce distinct dot-joined Structs", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "messages", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkg := requireSinglePackage(t, env)
		if findStruct(pkg, "User.Profile") == nil {
			t.Fatalf("Struct %q missing; got %+v", "User.Profile", structNames(pkg))
		}
		if findStruct(pkg, "Org.Profile") == nil {
			t.Fatalf("Struct %q missing; got %+v", "Org.Profile", structNames(pkg))
		}
	})
}

// TestConvert_FieldCompositeTypes covers the composite TypeRef
// shapes: `repeated X` fields produce a TypeRefSlice wrapping
// the element type; `map<K, V>` fields produce a TypeRefMap
// carrying typed key + value refs. Element-kind dispatch covers
// scalar, message, and enum element types via the shared
// elementTypeRef helper.
func TestConvert_FieldCompositeTypes(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "messages", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkg := requireSinglePackage(t, env)
	user := findStruct(pkg, "User")

	t.Run("repeated scalar field produces TypeRefSlice wrapping a scalar element", func(t *testing.T) {
		t.Parallel()
		tags := user.FieldByName("tags")
		if tags == nil {
			t.Fatalf("field tags missing on User")
		}
		if tags.Type.TypeKind != node.TypeRefSlice {
			t.Fatalf("tags Type.TypeKind = %v, want TypeRefSlice", tags.Type.TypeKind)
		}
		if tags.Type.Elem == nil {
			t.Fatalf("tags Type.Elem is nil; want a TypeRef")
		}
		if tags.Type.Elem.Name != "string" {
			t.Fatalf("tags Type.Elem.Name = %q, want %q", tags.Type.Elem.Name, "string")
		}
	})

	t.Run("repeated enum field wraps a named-ref to the enum", func(t *testing.T) {
		t.Parallel()
		statuses := user.FieldByName("statuses")
		if statuses == nil {
			t.Fatalf("field statuses missing on User")
		}
		if statuses.Type.TypeKind != node.TypeRefSlice {
			t.Fatalf("statuses Type.TypeKind = %v, want TypeRefSlice", statuses.Type.TypeKind)
		}
		elem := statuses.Type.Elem
		if elem == nil || elem.TypeKind != node.TypeRefNamed {
			t.Fatalf("statuses element TypeRef = %+v, want TypeRefNamed", elem)
		}
		if elem.Name != "Status" {
			t.Fatalf("statuses element Name = %q, want %q", elem.Name, "Status")
		}
		if elem.Package != pkg.Path {
			t.Fatalf("statuses element Package = %q, want %q", elem.Package, pkg.Path)
		}
	})

	t.Run("repeated message field wraps a named-ref element", func(t *testing.T) {
		t.Parallel()
		friends := user.FieldByName("friends")
		if friends == nil {
			t.Fatalf("field friends missing on User")
		}
		if friends.Type.TypeKind != node.TypeRefSlice {
			t.Fatalf("friends Type.TypeKind = %v, want TypeRefSlice", friends.Type.TypeKind)
		}
		elem := friends.Type.Elem
		if elem == nil || elem.TypeKind != node.TypeRefNamed {
			t.Fatalf("friends element TypeRef = %+v, want TypeRefNamed", elem)
		}
		if elem.Name != "User.Profile" {
			t.Fatalf("friends element Name = %q, want %q", elem.Name, "User.Profile")
		}
		if elem.Package != pkg.Path {
			t.Fatalf("friends element Package = %q, want %q", elem.Package, pkg.Path)
		}
	})

	t.Run("map<K,V> field produces TypeRefMap with typed key + value", func(t *testing.T) {
		t.Parallel()
		attrs := user.FieldByName("attrs")
		if attrs == nil {
			t.Fatalf("field attrs missing on User")
		}
		if attrs.Type.TypeKind != node.TypeRefMap {
			t.Fatalf("attrs Type.TypeKind = %v, want TypeRefMap", attrs.Type.TypeKind)
		}
		if attrs.Type.MapKey == nil || attrs.Type.MapKey.Name != "string" {
			t.Fatalf("attrs MapKey = %+v, want TypeRefNamed(string)", attrs.Type.MapKey)
		}
		if attrs.Type.MapValue == nil || attrs.Type.MapValue.Name != "int32" {
			t.Fatalf("attrs MapValue = %+v, want TypeRefNamed(int32)", attrs.Type.MapValue)
		}
	})

	t.Run("synthetic map-entry message is filtered from the produced Structs slice", func(t *testing.T) {
		t.Parallel()
		// protocompile synthesises a hidden message named
		// `User.AttrsEntry` for the map<string,int32> field; the
		// converter must drop it so consumers don't see the
		// implementation detail.
		for _, s := range pkg.Structs {
			if s.Name == "User.AttrsEntry" {
				t.Fatalf("synthetic map-entry Struct %q should not surface", s.Name)
			}
		}
	})
}

// TestConvert_MessageReservedAndTrailingDoc covers message-level
// reserved-range meta plus the message-level trailing comment.
// Reserved tag numbers expand into an int slice (single numbers
// and ranges both contribute); reserved field names contribute to
// the matching string slice.
func TestConvert_MessageReservedAndTrailingDoc(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "messages", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkg := requireSinglePackage(t, env)
	user := findStruct(pkg, "User")

	t.Run("proto.message.reserved.numbers expands ranges into the explicit integer list", func(t *testing.T) {
		t.Parallel()
		got, ok := frontend.MetaMessageReservedNumbers.Get(user.Meta())
		if !ok {
			t.Fatalf("proto.message.reserved.numbers missing on User")
		}
		want := []int32{7, 9, 10, 11}
		if !slices.Equal(got, want) {
			t.Fatalf("proto.message.reserved.numbers on User = %v, want %v", got, want)
		}
	})

	t.Run("proto.message.reserved.names carries every reserved field name", func(t *testing.T) {
		t.Parallel()
		got, ok := frontend.MetaMessageReservedNames.Get(user.Meta())
		if !ok {
			t.Fatalf("proto.message.reserved.names missing on User")
		}
		want := []string{"obsolete_email"}
		if !slices.Equal(got, want) {
			t.Fatalf("proto.message.reserved.names on User = %v, want %v", got, want)
		}
	})
}

// findStruct returns the Struct whose Name matches the supplied
// name, or nil when absent. Tests query against the dot-joined
// names produced for nested messages (`User.Profile`).
func findStruct(pkg *node.Package, name string) *node.Struct {
	for _, s := range pkg.Structs {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// structNames returns the Name of every Struct on pkg.
func structNames(pkg *node.Package) []string {
	out := make([]string, 0, len(pkg.Structs))
	for _, s := range pkg.Structs {
		out = append(out, s.Name)
	}
	return out
}
