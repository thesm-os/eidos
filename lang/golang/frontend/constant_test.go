// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/golang/frontend"
)

// TestConvertConstant covers typed, untyped, and iota-inheritance
// constants — plus the value-text faithfulness contract.
func TestConvertConstant(t *testing.T) {
	t.Parallel()
	t.Run("typed constant records its Type", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nconst Limit int = 10\n",
		})
		c := pkg.ConstantByName("Limit")
		if c == nil {
			t.Fatalf("Limit missing")
		}
		if c.Type == nil || c.Type.Name != "int" {
			t.Fatalf("expected int type, got %+v", c.Type)
		}
		if c.Value != "10" {
			t.Fatalf("value = %q, want 10", c.Value)
		}
	})

	t.Run("untyped constant leaves Type nil", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nconst Pi = 3.14\n",
		})
		c := pkg.ConstantByName("Pi")
		if c == nil {
			t.Fatalf("Pi missing")
		}
		if c.Type != nil {
			t.Fatalf("untyped constant must leave Type nil, got %+v", c.Type)
		}
	})

	t.Run("iota-driven inheritance pulls type onto trailing entries", func(t *testing.T) {
		t.Parallel()
		// Without an enum-style backing alias, iota constants remain
		// as standalone typed constants since they share a basic
		// underlying type.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nconst (\n\tA int = iota\n\tB\n\tC\n)\n",
		})
		for _, name := range []string{"A", "B", "C"} {
			c := pkg.ConstantByName(name)
			if c == nil {
				t.Fatalf("%s missing", name)
			}
			if c.Type == nil || c.Type.Name != "int" {
				t.Fatalf("%s Type = %+v, want int", name, c.Type)
			}
		}
	})

	t.Run("constant value text is preserved through MetaIotaValue", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nconst Code int = 42\n",
		})
		c := pkg.ConstantByName("Code")
		got, ok := frontend.MetaIotaValue.Get(c.Meta())
		if !ok || got != 42 {
			t.Fatalf("expected MetaIotaValue=42, got (%v, %v)", got, ok)
		}
	})

	t.Run("blank-identifier constants are skipped to avoid qname collisions", func(t *testing.T) {
		t.Parallel()
		// `const _ = iota` is the idiomatic way to consume an iota
		// slot without producing a referencable name; the
		// converter must skip it so a block with multiple `_`
		// entries does not collide on qualified-name registration.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nconst (\n\t_ = iota\n\tA\n\tB\n)\n",
		})
		for _, c := range pkg.Constants {
			if c.Name == "_" {
				t.Fatalf("blank-identifier constant leaked into Constants: %+v", c)
			}
		}
		// A and B still surface.
		if pkg.ConstantByName("A") == nil || pkg.ConstantByName("B") == nil {
			t.Fatalf("expected A and B present after blank skip, got %+v", pkg.Constants)
		}
	})

	t.Run("non-integer constant does not stamp MetaIotaValue", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nconst Label string = \"hi\"\n",
		})
		c := pkg.ConstantByName("Label")
		if frontend.MetaIotaValue.Has(c.Meta()) {
			t.Fatalf("string constant must not carry MetaIotaValue")
		}
	})

	t.Run("constant whose value overflows int64 does not stamp MetaIotaValue", func(t *testing.T) {
		t.Parallel()
		// constant.Int64Val returns exact=false when the typed value
		// overflows int64 — drives stampConstantMeta's !exact branch.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nconst Big = 1 << 80\n",
		})
		c := pkg.ConstantByName("Big")
		if c == nil {
			t.Fatalf("Big missing")
		}
		if frontend.MetaIotaValue.Has(c.Meta()) {
			t.Fatalf("overflowing constant must not carry MetaIotaValue")
		}
	})

	t.Run("constant with an unresolvable type annotation does not crash the converter", func(t *testing.T) {
		t.Parallel()
		// The type-checker still records C as *types.Const with an
		// Invalid value when the type annotation does not resolve;
		// the converter surfaces a diagnostic and continues.
		_, d := loadFromSource(t, map[string]string{
			"a.go": "package a\n\nconst C Missing = 1\n",
		})
		if !d.HasErrors() {
			t.Fatalf("expected an Error diagnostic for unresolved const type")
		}
	})
}

// TestConvertConstSpec_RedeclaredNameDoesNotPanic pins that a const
// the type-checker rejects degrades to a diagnostic rather than
// taking the converter down.
//
// go/types records a nil Defs entry for the losing side of a
// redeclaration. The conversion asserted that entry's concrete type
// unconditionally, on the documented-but-wrong premise that go/types
// "always" records a *types.Const — true for a package that
// type-checks, false the moment one does not. A frontend is the one
// component guaranteed to meet broken input: it runs against
// whatever the author last saved, so reporting and continuing is the
// contract.
func TestConvertConstSpec_RedeclaredNameDoesNotPanic(t *testing.T) {
	t.Parallel()

	// Each case puts the const in the later-sorting file so it is
	// the losing declaration; reversing the files would exercise the
	// winning side and pass regardless.
	cases := map[string]map[string]string{
		"const losing to a type": {
			"a.go": "package p\n\ntype Draft struct{}\n",
			"b.go": "package p\n\nconst Draft = 1\n",
		},
		"const losing to a const": {
			"a.go": "package p\n\nconst Dup = 1\n",
			"b.go": "package p\n\nconst Dup = 2\n",
		},
		// The shape found in the wild: an iota block whose member
		// collides with a type declared elsewhere in the package.
		"const losing inside an iota block": {
			"a.go": "package p\n\ntype Status int\n\ntype Draft struct{}\n",
			"b.go": "package p\n\nconst (\n\tActive Status = iota\n\tDraft\n)\n",
		},
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertRedeclarationReported(t, src)
		})
	}
}
