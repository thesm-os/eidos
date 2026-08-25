// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

func TestEnumDecl(t *testing.T) {
	t.Parallel()

	only := func(t *testing.T, src string) *node.Enum {
		t.Helper()
		decls, _ := convert(t, src)
		if len(decls) != 1 {
			t.Fatalf("expected 1 declaration, got %d", len(decls))
		}
		e, ok := decls[0].(*node.Enum)
		if !ok {
			t.Fatalf("expected *node.Enum, got %T", decls[0])
		}
		return e
	}

	t.Run("bare and assigned members both become variants", func(t *testing.T) {
		t.Parallel()
		e := only(t, `enum Color { Red, Green = 2, Blue = 'b' }`)

		if e.Name != "Color" {
			t.Fatalf("Name = %q", e.Name)
		}
		if got := len(e.Variants); got != 3 {
			t.Fatalf("Variants = %d, want 3", got)
		}
		for i, want := range []struct{ name, value string }{
			{"Red", ""}, {"Green", "2"}, {"Blue", "'b'"},
		} {
			if e.Variants[i].Name != want.name || e.Variants[i].Value != want.value {
				t.Errorf("variant %d = (%q, %q), want (%q, %q)",
					i, e.Variants[i].Name, e.Variants[i].Value, want.name, want.value)
			}
		}
	})

	t.Run("an implicit value is left empty rather than computed", func(t *testing.T) {
		t.Parallel()
		// Recording `0` would put a literal in the graph that the
		// source never wrote. The rule that derives it is the
		// consumer's to apply.
		e := only(t, `enum E { A }`)
		if e.Variants[0].Value != "" {
			t.Fatalf("Value = %q, want empty", e.Variants[0].Value)
		}
	})

	t.Run("underlying type is derived from the member values", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct{ src, want string }{
			{`enum E { A = 'a', B = 'b' }`, "string"},
			{`enum E { A = 1, B = 2 }`, "number"},
			{`enum E { A }`, "number"},
			{`enum E { A = 'a', B }`, "number"},
		} {
			e := only(t, tc.src)
			if e.Underlying == nil || e.Underlying.Name != tc.want {
				t.Errorf("%s: underlying = %v, want %s", tc.src, e.Underlying, tc.want)
			}
		}
	})

	t.Run("a const enum is marked", func(t *testing.T) {
		t.Parallel()
		plain := only(t, `enum E { A }`)
		konst := only(t, `const enum F { A }`)

		if typescript.MetaConstEnum.Has(plain.Meta()) {
			t.Error("plain enum marked const")
		}
		if got, _ := typescript.MetaConstEnum.Get(konst.Meta()); !got {
			t.Error("const enum not marked")
		}
	})

	t.Run("variants carry their owner", func(t *testing.T) {
		t.Parallel()
		e := only(t, `enum E { A, B }`)
		for i, v := range e.Variants {
			if v.Owner != e {
				t.Errorf("variant %d Owner not wired", i)
			}
		}
	})
}
