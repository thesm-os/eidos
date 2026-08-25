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

// TestConvert_Enums_SourcePos covers the BaseNode.SourcePos
// contract for proto-derived enums and variants: every produced
// node carries the originating file plus a non-zero line so
// downstream consumers can resolve the declaration site.
func TestConvert_Enums_SourcePos(t *testing.T) {
	t.Parallel()

	t.Run("each Enum and EnumVariant carries the originating proto file path and line", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "messages", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkg := requireSinglePackage(t, env)
		if len(pkg.Enums) == 0 {
			t.Fatalf("expected at least one Enum in the messages fixture")
		}
		for _, e := range pkg.Enums {
			ep := e.Pos()
			if ep.File == "" {
				t.Errorf("Enum %q carries empty Pos.File", e.Name)
			}
			if ep.Line == 0 {
				t.Errorf("Enum %q carries zero Pos.Line", e.Name)
			}
			for _, v := range e.Variants {
				vp := v.Pos()
				if vp.File == "" {
					t.Errorf("EnumVariant %q on %q carries empty Pos.File", v.Name, e.Name)
				}
				if vp.Line == 0 {
					t.Errorf("EnumVariant %q on %q carries zero Pos.Line", v.Name, e.Name)
				}
			}
		}
	})
}

// TestConvert_Enums covers the enum → node.Enum mapping: each
// `enum X { ... }` produces a node.Enum named X in the producing
// package. Variants land as node.EnumVariant children with Value
// holding the source-form integer string; the typed numeric form
// rides on proto.enum_variant.number meta. Every enum carries
// node.Enum.Underlying = TypeRefNamed("int32") per the proto3
// width contract.
func TestConvert_Enums(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "messages", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkg := requireSinglePackage(t, env)

	t.Run("Status enum produces a node.Enum with int32 underlying and variants in source order", func(t *testing.T) {
		t.Parallel()
		e := findEnum(pkg, "Status")
		if e == nil {
			t.Fatalf("Enum %q missing", "Status")
		}
		if e.Package != pkg.Path {
			t.Fatalf("Enum.Package = %q, want %q", e.Package, pkg.Path)
		}
		if e.Underlying == nil || e.Underlying.Name != "int32" {
			t.Fatalf("Enum.Underlying = %+v, want TypeRefNamed(int32)", e.Underlying)
		}
		gotNames := variantNames(e)
		wantNames := []string{"STATUS_UNKNOWN", "STATUS_ACTIVE", "STATUS_BANNED"}
		if !slices.Equal(gotNames, wantNames) {
			t.Fatalf("Status variants = %+v, want %+v", gotNames, wantNames)
		}
		wantValues := []string{"0", "1", "2"}
		for i, v := range e.Variants {
			if v.Value != wantValues[i] {
				t.Fatalf("variant %q Value = %q, want %q", v.Name, v.Value, wantValues[i])
			}
			got, ok := frontend.MetaEnumVariantNumber.Get(v.Meta())
			if !ok {
				t.Fatalf("proto.enum_variant.number missing on %q", v.Name)
			}
			if got != i {
				t.Fatalf("proto.enum_variant.number on %q = %d, want %d", v.Name, got, i)
			}
		}
	})

	t.Run("AliasedStatus carries proto.enum.allow_alias = true + the raw option-channel form", func(t *testing.T) {
		t.Parallel()
		e := findEnum(pkg, "AliasedStatus")
		if e == nil {
			t.Fatalf("Enum %q missing", "AliasedStatus")
		}
		got, ok := frontend.MetaEnumAllowAlias.Get(e.Meta())
		if !ok || !got {
			t.Fatalf("proto.enum.allow_alias missing or false on AliasedStatus; got (%v, %v)", got, ok)
		}
		// Coexistence: alias and raw option-channel form both
		// land. Plugins iterating proto.option.* see the same
		// value under the standard name.
		raw := meta.EnsureKey(frontend.MetaOptionPrefix+"allow_alias", meta.BoolParser)
		rawVal, rawOK := raw.Get(e.Meta())
		if !rawOK || !rawVal {
			t.Fatalf(
				"expected proto.option.allow_alias = true alongside the alias; got (%v, %v)",
				rawVal, rawOK,
			)
		}
	})

	t.Run("Status reserved ranges expand into proto.enum.reserved.numbers and .names", func(t *testing.T) {
		t.Parallel()
		e := findEnum(pkg, "Status")
		nums, ok := frontend.MetaEnumReservedNumbers.Get(e.Meta())
		if !ok {
			t.Fatalf("proto.enum.reserved.numbers missing on Status")
		}
		wantNums := []int32{3, 4}
		if !slices.Equal(nums, wantNums) {
			t.Fatalf("proto.enum.reserved.numbers on Status = %v, want %v", nums, wantNums)
		}
		names, ok := frontend.MetaEnumReservedNames.Get(e.Meta())
		if !ok {
			t.Fatalf("proto.enum.reserved.names missing on Status")
		}
		wantNames := []string{"OLD_BANNED"}
		if !slices.Equal(names, wantNames) {
			t.Fatalf("proto.enum.reserved.names on Status = %v, want %v", names, wantNames)
		}
	})
}

// findEnum returns the Enum whose Name matches name, or nil when
// absent.
func findEnum(pkg *node.Package, name string) *node.Enum {
	for _, e := range pkg.Enums {
		if e.Name == name {
			return e
		}
	}
	return nil
}

// variantNames returns the Name field of every variant on e.
func variantNames(e *node.Enum) []string {
	out := make([]string, 0, len(e.Variants))
	for _, v := range e.Variants {
		out = append(out, v.Name)
	}
	return out
}
