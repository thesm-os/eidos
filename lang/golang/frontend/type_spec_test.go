// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"
)

// TestConvertTypeSpec_Dispatch verifies each type-spec underlying
// shape lands on the right slice in the converted package.
func TestConvertTypeSpec_Dispatch(t *testing.T) {
	t.Parallel()
	t.Run("struct dispatches to Structs", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{}\n",
		})
		if pkg.StructByName("S") == nil {
			t.Fatalf("S not in Structs slice")
		}
	})

	t.Run("interface dispatches to Interfaces", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype I interface{ M() }\n",
		})
		if pkg.InterfaceByName("I") == nil {
			t.Fatalf("I not in Interfaces slice")
		}
	})

	t.Run("everything else dispatches to Aliases", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Status int\n",
		})
		if pkg.AliasByName("Status") == nil {
			t.Fatalf("Status not in Aliases slice")
		}
	})

	t.Run("docs and directives flow through to the dispatched node", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\n// +gen:repo\n// FooRepo is the repository.\ntype Foo struct{}\n",
		})
		s := pkg.StructByName("Foo")
		if s == nil {
			t.Fatalf("Foo missing")
		}
		if len(s.DirectiveList) != 1 {
			t.Fatalf("expected 1 directive, got %d", len(s.DirectiveList))
		}
	})
}
