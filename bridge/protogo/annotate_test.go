// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package protogo_test

import (
	"testing"

	"go.thesmos.sh/eidos/bridge/protogo"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/frontend/protobuf"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

// fieldOf builds a one-struct proto store carrying a single field
// of the supplied type, drives the bridge over it, and returns the
// annotated field. The struct walk is the field-aware path — the
// only one that can read `proto.field.optional` off the field's own
// meta bag — so every field-level rule below goes through it.
func fieldOf(t *testing.T, typ *node.TypeRef, prep func(*node.Field)) *node.Field {
	t.Helper()
	s := protoStore(t, func(b *storefixture.Builder) {
		b.Struct("Msg", func(sb *storefixture.StructBuilder) {
			sb.Field("user_id", typ, nil)
		})
	})
	f := store.NewReader(s).Structs().Slice()[0].Fields[0]
	if prep != nil {
		prep(f)
	}
	annotateProto(t, s)
	return f
}

// TestAnnotate_Idempotency pins the bridge's override contract: a
// stamp that is already present wins. Users set `go.name` / `go.type`
// by hand on a proto declaration to escape the translation tables,
// and a bridge that overwrote them would make the escape hatch
// silently inoperative — the run would still succeed, just not with
// the user's spelling.
func TestAnnotate_Idempotency(t *testing.T) {
	t.Parallel()

	t.Run("a pre-stamped go.name on a field is preserved", func(t *testing.T) {
		t.Parallel()
		f := fieldOf(t, storefixture.Named("string"), func(f *node.Field) {
			protogo.MetaGoName.Set(f.EnsureMeta(), "CustomerRef", "manual")
		})
		if got, _ := protogo.MetaGoName.Get(f.Meta()); got != "CustomerRef" {
			t.Fatalf("go.name = %q, want the pre-stamped CustomerRef", got)
		}
	})

	t.Run("a field with no pre-stamped go.name is translated", func(t *testing.T) {
		t.Parallel()
		f := fieldOf(t, storefixture.Named("string"), nil)
		if got, _ := protogo.MetaGoName.Get(f.Meta()); got != "UserID" {
			t.Fatalf("go.name = %q, want UserID", got)
		}
	})

	t.Run("a pre-stamped go.type on a field type is preserved", func(t *testing.T) {
		t.Parallel()
		f := fieldOf(t, storefixture.Named("string"), func(f *node.Field) {
			protogo.MetaGoType.Set(f.Type.EnsureMeta(), "json.RawMessage", "manual")
		})
		if got, _ := protogo.MetaGoType.Get(f.Type.Meta()); got != "json.RawMessage" {
			t.Fatalf("go.type = %q, want the pre-stamped json.RawMessage", got)
		}
	})

	t.Run("a pre-stamped go.type on an rpc parameter is preserved", func(t *testing.T) {
		t.Parallel()
		// The interface walk reaches TypeRefs directly rather than
		// through a field, so it carries its own idempotency guard.
		s := protoStore(t, func(b *storefixture.Builder) {
			b.Interface("Greeter", func(i *storefixture.InterfaceBuilder) {
				i.Method("Say", func(m *storefixture.MethodBuilder) {
					m.Param("name", storefixture.Named("string"))
				})
			})
		})
		p := store.NewReader(s).Interfaces().Slice()[0].Methods[0].Params[0]
		protogo.MetaGoType.Set(p.Type.EnsureMeta(), "mypkg.Name", "manual")
		annotateProto(t, s)
		if got, _ := protogo.MetaGoType.Get(p.Type.Meta()); got != "mypkg.Name" {
			t.Fatalf("go.type = %q, want the pre-stamped mypkg.Name", got)
		}
	})
}

// TestAnnotate_OptionalWrap pins the proto3 `optional` rule. An
// optional scalar is a pointer in generated Go so the zero value
// stays distinguishable from "not set"; stamping the bare scalar
// would erase that distinction at the render site.
func TestAnnotate_OptionalWrap(t *testing.T) {
	t.Parallel()

	t.Run("an optional scalar field is stamped as a pointer", func(t *testing.T) {
		t.Parallel()
		f := fieldOf(t, storefixture.Named("string"), func(f *node.Field) {
			protobuf.MetaFieldOptional.Set(f.EnsureMeta(), true, "test")
		})
		if got, _ := protogo.MetaGoType.Get(f.Type.Meta()); got != "*string" {
			t.Fatalf("go.type = %q, want *string", got)
		}
	})

	t.Run("a non-optional scalar field is stamped bare", func(t *testing.T) {
		t.Parallel()
		f := fieldOf(t, storefixture.Named("string"), nil)
		if got, _ := protogo.MetaGoType.Get(f.Type.Meta()); got != "string" {
			t.Fatalf("go.type = %q, want string", got)
		}
	})
}

// TestAnnotate_UntranslatableTypes pins the bridge's refusal to
// guess. An empty composition means "no translation available", and
// the caller must skip the stamp entirely so the render site falls
// back to the TypeRef name verbatim. A partial or invented stamp
// would render Go that does not compile.
func TestAnnotate_UntranslatableTypes(t *testing.T) {
	t.Parallel()

	// named is a message reference — deliberately outside the scalar
	// and well-known tables, so it composes to nothing.
	named := func() *node.TypeRef { return storefixture.Named("OtherMessage") }

	t.Run("a bare message reference is not stamped", func(t *testing.T) {
		t.Parallel()
		f := fieldOf(t, named(), nil)
		if got, ok := protogo.MetaGoType.Get(f.Type.Meta()); ok {
			t.Fatalf("go.type = %q, want no stamp for a message reference", got)
		}
	})

	t.Run("a slice of message references is not stamped", func(t *testing.T) {
		t.Parallel()
		f := fieldOf(t, storefixture.Slice(named()), nil)
		if got, ok := protogo.MetaGoType.Get(f.Type.Meta()); ok {
			t.Fatalf("go.type = %q, want no stamp for a slice of messages", got)
		}
	})

	t.Run("a map with an untranslatable value is not stamped", func(t *testing.T) {
		t.Parallel()
		f := fieldOf(t, storefixture.Map(storefixture.Named("string"), named()), nil)
		if got, ok := protogo.MetaGoType.Get(f.Type.Meta()); ok {
			t.Fatalf("go.type = %q, want no stamp for a map of messages", got)
		}
	})

	t.Run("a map with an untranslatable key is not stamped", func(t *testing.T) {
		t.Parallel()
		f := fieldOf(t, storefixture.Map(named(), storefixture.Named("string")), nil)
		if got, ok := protogo.MetaGoType.Get(f.Type.Meta()); ok {
			t.Fatalf("go.type = %q, want no stamp for a map keyed by a message", got)
		}
	})

	t.Run("a slice of scalars is stamped", func(t *testing.T) {
		t.Parallel()
		f := fieldOf(t, storefixture.Slice(storefixture.Named("int32")), nil)
		if got, _ := protogo.MetaGoType.Get(f.Type.Meta()); got != "[]int32" {
			t.Fatalf("go.type = %q, want []int32", got)
		}
	})
}

// TestAnnotate_PointerAndDegenerateComposites pins the two
// composition arms the field walk reaches only through nesting: the
// pointer wrap, and a composite whose element carries no type at
// all. The latter must compose to nothing rather than to a
// half-formed spelling like "[]" — which would render as invalid Go.
func TestAnnotate_PointerAndDegenerateComposites(t *testing.T) {
	t.Parallel()

	t.Run("a pointer to a scalar is stamped with a pointer spelling", func(t *testing.T) {
		t.Parallel()
		f := fieldOf(t, storefixture.Pointer(storefixture.Named("int64")), nil)
		if got, _ := protogo.MetaGoType.Get(f.Type.Meta()); got != "*int64" {
			t.Fatalf("go.type = %q, want *int64", got)
		}
	})

	t.Run("a pointer to a message reference is not stamped", func(t *testing.T) {
		t.Parallel()
		f := fieldOf(t, storefixture.Pointer(storefixture.Named("OtherMessage")), nil)
		if got, ok := protogo.MetaGoType.Get(f.Type.Meta()); ok {
			t.Fatalf("go.type = %q, want no stamp for a pointer to a message", got)
		}
	})

	t.Run("a slice carrying no element type is not stamped", func(t *testing.T) {
		t.Parallel()
		f := fieldOf(t, &node.TypeRef{TypeKind: node.TypeRefSlice}, nil)
		if got, ok := protogo.MetaGoType.Get(f.Type.Meta()); ok {
			t.Fatalf("go.type = %q, want no stamp for an elementless slice", got)
		}
	})
}

// TestAnnotate_PackageMeta pins the package-level derivation from
// the proto `go_package` option. The import path it yields is what
// lets the Go backend register cross-package references on the
// rendered file's ImportSet; without it a reference renders with no
// import and the output does not compile.
func TestAnnotate_PackageMeta(t *testing.T) {
	t.Parallel()

	// withOption builds a proto package carrying the raw
	// `go_package` option value and returns it after annotation.
	withOption := func(t *testing.T, raw string) *node.Package {
		t.Helper()
		s := protoStore(t, nil)
		pkg := store.NewReader(s).Packages().Slice()[0]
		if raw != "" {
			meta.EnsureKey(protobuf.MetaOptionPrefix+"go_package", meta.StringParser).
				Set(pkg.EnsureMeta(), raw, "test")
		}
		annotateProto(t, s)
		return pkg
	}

	t.Run("derives the Go import path from the go_package option", func(t *testing.T) {
		t.Parallel()
		pkg := withOption(t, "example.com/gen/pb;pb")
		if got, _ := protogo.MetaGoImport.Get(pkg.Meta()); got != "example.com/gen/pb" {
			t.Fatalf("go.import = %q, want example.com/gen/pb", got)
		}
	})

	t.Run("derives the Go package name from the go_package option", func(t *testing.T) {
		t.Parallel()
		pkg := withOption(t, "example.com/gen/pb;pbv1")
		if got, _ := protogo.MetaGoName.Get(pkg.Meta()); got != "pbv1" {
			t.Fatalf("go.name = %q, want pbv1", got)
		}
	})

	t.Run("a package with no go_package option carries no import path", func(t *testing.T) {
		t.Parallel()
		pkg := withOption(t, "")
		if got, ok := protogo.MetaGoImport.Get(pkg.Meta()); ok {
			t.Fatalf("go.import = %q, want no stamp without the option", got)
		}
	})

	t.Run("a pre-stamped go.import is preserved", func(t *testing.T) {
		t.Parallel()
		s := protoStore(t, nil)
		pkg := store.NewReader(s).Packages().Slice()[0]
		protogo.MetaGoImport.Set(pkg.EnsureMeta(), "example.com/manual", "manual")
		meta.EnsureKey(protobuf.MetaOptionPrefix+"go_package", meta.StringParser).
			Set(pkg.EnsureMeta(), "example.com/derived;pb", "test")
		annotateProto(t, s)
		if got, _ := protogo.MetaGoImport.Get(pkg.Meta()); got != "example.com/manual" {
			t.Fatalf("go.import = %q, want the pre-stamped example.com/manual", got)
		}
	})
}
