// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

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
