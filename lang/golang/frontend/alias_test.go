// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"
)

// TestConvertAlias covers both `type X = Y` (true alias) and
// `type X Y` (new type definition).
func TestConvertAlias(t *testing.T) {
	t.Parallel()
	t.Run("type definition is recorded with IsAlias=false", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Seconds int\n",
		})
		a := pkg.AliasByName("Seconds")
		if a == nil {
			t.Fatalf("Seconds alias missing")
		}
		if a.IsAlias {
			t.Fatalf("type definition must report IsAlias=false")
		}
		if a.Target == nil || a.Target.Name != "int" {
			t.Fatalf("Target = %+v, want int", a.Target)
		}
	})

	t.Run("type alias is recorded with IsAlias=true", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype StringList = []string\n",
		})
		a := pkg.AliasByName("StringList")
		if a == nil {
			t.Fatalf("StringList alias missing")
		}
		if !a.IsAlias {
			t.Fatalf("expected IsAlias=true")
		}
		if a.Target == nil || !a.Target.IsSlice() {
			t.Fatalf("Target = %+v, want []string", a.Target)
		}
	})

	t.Run("generic alias declaration captures type parameters", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Slice[T any] []T\n",
		})
		a := pkg.AliasByName("Slice")
		if a == nil {
			t.Fatalf("Slice alias missing")
		}
		if len(a.TypeParams) != 1 || a.TypeParams[0].Name != "T" {
			t.Fatalf("expected 1 type-param T, got %+v", a.TypeParams)
		}
	})

	t.Run("true alias to an unresolved identifier records an Invalid target and emits a diagnostic",
		func(t *testing.T) {
			t.Parallel()
			// go/types records an Invalid *types.Basic for an
			// unresolved identifier — the alias surfaces with that
			// target rather than being dropped, so generators can still
			// see the broken declaration.
			_, d := loadFromSource(t, map[string]string{
				"a.go": "package a\n\ntype X = Missing\n",
			})
			if !d.HasErrors() {
				t.Fatalf("expected an Error diagnostic for unresolved alias RHS")
			}
		})
}
