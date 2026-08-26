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

func TestEnumEdgeCases(t *testing.T) {
	t.Parallel()

	onlyEnum := func(t *testing.T, src string) *node.Enum {
		t.Helper()
		e, ok := onlyDecl(t, src).(*node.Enum)
		if !ok {
			t.Fatalf("expected *node.Enum, got %T", onlyDecl(t, src))
		}
		return e
	}

	t.Run("an empty enum declares the type with no variants", func(t *testing.T) {
		t.Parallel()
		e := onlyEnum(t, `enum E {}`)
		if len(e.Variants) != 0 {
			t.Fatalf("Variants = %d, want 0", len(e.Variants))
		}
		if e.Underlying == nil {
			t.Fatal("an empty enum lost its underlying type")
		}
	})

	t.Run("a quoted member name is kept as written", func(t *testing.T) {
		t.Parallel()
		e := onlyEnum(t, `enum E { 'quoted-name' = 1 }`)
		if len(e.Variants) != 1 || e.Variants[0].Name != "quoted-name" {
			t.Fatalf("Variants = %+v, want one named quoted-name", e.Variants)
		}
	})

	t.Run("a computed member is skipped rather than named after its expression", func(t *testing.T) {
		t.Parallel()
		e := onlyEnum(t, `enum E { A = 1, B = A << 1 }`)
		for _, v := range e.Variants {
			if v.Name == "" {
				t.Fatal("an unnamed variant reached the graph")
			}
		}
	})

	t.Run("a trailing comma does not add a variant", func(t *testing.T) {
		t.Parallel()
		e := onlyEnum(t, `enum E { A, B, }`)
		if len(e.Variants) != 2 {
			t.Fatalf("Variants = %d, want 2", len(e.Variants))
		}
	})

	t.Run("docs attach to a variant", func(t *testing.T) {
		t.Parallel()
		e := onlyEnum(t, "enum E {\n  /** The first. */\n  A,\n}")
		if len(e.Variants[0].Docs()) == 0 {
			t.Fatal("a documented variant carried no doc lines")
		}
	})

	t.Run("a const enum in a namespace keeps both facts", func(t *testing.T) {
		t.Parallel()
		decls, _ := convert(t, `namespace N { export const enum E { A } }`)
		e, ok := decls[0].(*node.Enum)
		if !ok {
			t.Fatalf("got %T", decls[0])
		}
		if ce, _ := typescript.MetaConstEnum.Get(e.Meta()); !ce {
			t.Error("const enum not marked")
		}
		if ns, _ := typescript.MetaNamespace.Get(e.Meta()); ns != "N" {
			t.Errorf("namespace = %q, want N", ns)
		}
	})
}
