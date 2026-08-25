// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

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

func TestTypeParamEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("an unconstrained parameter carries no bound", func(t *testing.T) {
		t.Parallel()
		i := onlyInterface(t, `interface A<T> { v: T; }`)
		if i.TypeParams[0].Constraint != nil {
			t.Fatalf("Constraint = %+v, want nil", i.TypeParams[0].Constraint)
		}
	})

	t.Run("a default without a bound is recorded", func(t *testing.T) {
		t.Parallel()
		i := onlyInterface(t, `interface A<T = string> { v: T; }`)
		if i.TypeParams[0].Constraint != nil {
			t.Error("a defaulted parameter gained a bound it does not have")
		}
		if def, _ := typescript.MetaTypeParamDefault.Get(i.TypeParams[0].Meta()); def != "string" {
			t.Errorf("default = %q, want string", def)
		}
	})

	t.Run("several parameters each keep their own bound", func(t *testing.T) {
		t.Parallel()
		i := onlyInterface(t, `interface A<K extends string, V extends object> { k: K; v: V; }`)
		if len(i.TypeParams) != 2 {
			t.Fatalf("TypeParams = %d, want 2", len(i.TypeParams))
		}
		for n, want := range []string{"string", "object"} {
			c := i.TypeParams[n].Constraint
			if c == nil || c.Raw != want {
				t.Errorf("param %d constraint = %+v, want %s", n, c, want)
			}
		}
	})

	t.Run("a method declares its own parameters", func(t *testing.T) {
		t.Parallel()
		i := onlyInterface(t, `interface A { m<U extends object>(x: U): U; }`)
		m := i.Methods[0]
		if len(m.TypeParams) != 1 || m.TypeParams[0].Name != "U" {
			t.Fatalf("method TypeParams = %+v, want one named U", m.TypeParams)
		}
		if m.TypeParams[0].Owner != node.Node(m) {
			t.Error("method type-param Owner not wired to the method")
		}
	})
}
