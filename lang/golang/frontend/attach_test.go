// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"
)

// TestAttachMethods covers the receiver → host routing pass.
func TestAttachMethods(t *testing.T) {
	t.Parallel()
	t.Run("struct methods route by receiver qualified name", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{}\n\nfunc (s S) A() {}\nfunc (s *S) B() {}\n",
		})
		s := pkg.StructByName("S")
		if len(s.Methods) != 2 {
			t.Fatalf("expected 2 methods on S, got %d", len(s.Methods))
		}
	})

	t.Run("methods on a type-defined alias attach to the Alias node", func(t *testing.T) {
		t.Parallel()
		// `type Seconds int; func (s Seconds) Mul(...)`. Methods
		// on a named-type alias attach to [node.Alias.Methods] —
		// Go permits methods on any named type whose underlying
		// is not an interface, so the converter must route them
		// to the host Alias rather than orphaning the method.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Seconds int\n\nfunc (s Seconds) Mul(by int) Seconds { return s }\n",
		})
		a := pkg.AliasByName("Seconds")
		if a == nil {
			t.Fatalf("Seconds alias missing")
		}
		if a.MethodByName("Mul") == nil {
			t.Fatalf("Mul method must attach to Seconds alias, got methods=%+v", a.Methods)
		}
	})
}
