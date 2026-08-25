// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/golang/frontend"
	"go.thesmos.sh/eidos/node"
)

// TestDetectEnums covers the typed-iota → Enum promotion path.
func TestDetectEnums(t *testing.T) {
	t.Parallel()
	t.Run("typed iota constants promote to an Enum", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Status int\n\nconst (\n\tStatusActive Status = iota\n\tStatusInactive\n\tStatusArchived\n)\n",
		})
		e := pkg.EnumByName("Status")
		if e == nil {
			t.Fatalf("Status not promoted to enum")
		}
		if len(e.Variants) != 3 {
			t.Fatalf("expected 3 variants, got %d", len(e.Variants))
		}
		if e.Underlying == nil || e.Underlying.Name != "int" {
			t.Fatalf("Enum.Underlying = %+v, want int", e.Underlying)
		}
		// Variant iota values preserved via MetaIotaValue.
		for i, v := range e.Variants {
			got, _ := frontend.MetaIotaValue.Get(v.Meta())
			if got != i {
				t.Fatalf("variant %d MetaIotaValue = %v, want %d", i, got, i)
			}
		}
	})

	t.Run("absorbed alias is removed from the Aliases slice", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Status int\n\nconst (\n\tStatusActive Status = iota\n)\n",
		})
		if pkg.AliasByName("Status") != nil {
			t.Fatalf("Status alias should have been absorbed by enum promotion")
		}
	})

	t.Run("typed constants without a matching alias stay as Constants", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nconst Limit int = 10\n",
		})
		if pkg.EnumByName("int") != nil {
			t.Fatalf("must not promote primitives into enum")
		}
		if pkg.ConstantByName("Limit") == nil {
			t.Fatalf("Limit should remain as a Constant entry")
		}
	})

	t.Run("untyped constants are never promoted", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nconst (\n\tA = iota\n\tB\n\tC\n)\n",
		})
		if len(pkg.Enums) != 0 {
			t.Fatalf("untyped iota constants must not produce enums")
		}
	})
}

// TestDetectEnums_MethodsSurvive pins the method set across enum
// promotion.
//
// The alias is the only node that held the methods, and removeAlias
// deletes it, so a method set left behind here was not relocated — it
// was discarded, with no diagnostic at any severity. The same
// `type Status int` kept its methods right up until a const block
// made it an enum.
//
// The fixture is the reported one: two declarations differing only in
// whether a const block follows, so the assertions can show the
// non-enum form is unaffected.
func TestDetectEnums_MethodsSurvive(t *testing.T) {
	t.Parallel()

	const src = "package a\n\n" +
		"type Colour int\n\n" +
		"func (c Colour) String() string { return \"colour\" }\n\n" +
		"type Status int\n\n" +
		"const (\n\tDraft Status = iota + 1\n\tLive\n)\n\n" +
		"func (s Status) String() string { return \"status\" }\n" +
		"func (s Status) MarshalText() ([]byte, error) { return nil, nil }\n"

	t.Run("the promoted enum carries its method set", func(t *testing.T) {
		t.Parallel()
		e := requirePackage(t, map[string]string{"a.go": src}).EnumByName("Status")
		if e == nil {
			t.Fatalf("Status not promoted to enum")
		}
		if len(e.Methods) != 2 {
			t.Fatalf("expected 2 methods on the enum; got %d (%+v)", len(e.Methods), e.Methods)
		}
	})

	t.Run("methods are reachable by name", func(t *testing.T) {
		t.Parallel()
		// This is the question a generator asks to decide whether a
		// round-trip law is available.
		e := requirePackage(t, map[string]string{"a.go": src}).EnumByName("Status")
		for _, name := range []string{"String", "MarshalText"} {
			if e.MethodByName(name) == nil {
				t.Fatalf("MethodByName(%q) = nil; got %+v", name, e.Methods)
			}
		}
		if e.MethodByName("Parse") != nil {
			t.Fatalf("Parse is not declared and must not be reported")
		}
	})

	t.Run("method order follows the source", func(t *testing.T) {
		t.Parallel()
		e := requirePackage(t, map[string]string{"a.go": src}).EnumByName("Status")
		if e.Methods[0].Name != "String" || e.Methods[1].Name != "MarshalText" {
			t.Fatalf("declaration order not preserved; got %+v", e.Methods)
		}
	})

	t.Run("each method's Owner points at the enum", func(t *testing.T) {
		t.Parallel()
		// removeAlias drops the alias from the package, so an Owner
		// left aimed at it would name a parent no longer in the graph.
		e := requirePackage(t, map[string]string{"a.go": src}).EnumByName("Status")
		for _, m := range e.Methods {
			if m.Owner != node.Node(e) {
				t.Fatalf("method %q Owner = %T, want the enum", m.Name, m.Owner)
			}
		}
	})

	t.Run("the non-enum form is unaffected", func(t *testing.T) {
		t.Parallel()
		a := requirePackage(t, map[string]string{"a.go": src}).AliasByName("Colour")
		if a == nil {
			t.Fatalf("Colour should remain an alias")
		}
		if len(a.Methods) != 1 || a.Methods[0].Name != "String" {
			t.Fatalf("Colour should still carry String; got %+v", a.Methods)
		}
	})

	t.Run("an enum with no methods carries none", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Plain int\n\nconst (\n\tOne Plain = iota\n)\n",
		})
		if e := pkg.EnumByName("Plain"); e == nil || len(e.Methods) != 0 {
			t.Fatalf("methodless enum should carry no methods; got %+v", e)
		}
	})
}
