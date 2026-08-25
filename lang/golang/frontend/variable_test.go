// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"
)

// TestConvertVariable covers the var-spec conversion path: typed
// declarations, inferred-type declarations, and the initialiser
// expression preservation.
func TestConvertVariable(t *testing.T) {
	t.Parallel()
	t.Run("typed variable records its type", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nvar Name string = \"x\"\n",
		})
		v := pkg.VariableByName("Name")
		if v == nil {
			t.Fatalf("Name missing")
		}
		if v.Type == nil || v.Type.Name != "string" {
			t.Fatalf("expected string type, got %+v", v.Type)
		}
	})

	t.Run("inferred-type variable leaves Type nil", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nvar Count = 42\n",
		})
		v := pkg.VariableByName("Count")
		if v == nil {
			t.Fatalf("Count missing")
		}
		if v.Type != nil {
			t.Fatalf("inferred-type variable must leave Type nil, got %+v", v.Type)
		}
	})

	t.Run("initialiser expression is preserved verbatim", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nvar Greeting = \"hello, \" + \"world\"\n",
		})
		v := pkg.VariableByName("Greeting")
		if v == nil {
			t.Fatalf("Greeting missing")
		}
		if v.InitExpr == "" {
			t.Fatalf("expected non-empty InitExpr")
		}
	})

	t.Run("blank-identifier variables are skipped to avoid qname collisions", func(t *testing.T) {
		t.Parallel()
		// Two files in one package both declaring `var _ = fmt.X`
		// would collide on qualified name "pkg._" if the
		// converter recorded blank vars. Skipping `_` keeps
		// declaration-only side effects from polluting the store.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport \"fmt\"\n\nvar _ = fmt.Sprintf\n",
			"b.go": "package a\n\nimport \"fmt\"\n\nvar _ = fmt.Println\n",
		})
		for _, v := range pkg.Variables {
			if v.Name == "_" {
				t.Fatalf("blank-identifier variable leaked into Variables: %+v", v)
			}
		}
	})

	t.Run("multi-name var with one rhs shares the printed expression", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nfunc twoInts() (int, int) { return 1, 2 }\n\nvar A, B = twoInts()\n",
		})
		a := pkg.VariableByName("A")
		b := pkg.VariableByName("B")
		if a == nil || b == nil {
			t.Fatalf("A or B missing")
		}
		if a.InitExpr != b.InitExpr || a.InitExpr == "" {
			t.Fatalf("multi-name vars must share InitExpr, got A=%q B=%q", a.InitExpr, b.InitExpr)
		}
	})

	t.Run("variable with an unresolvable initialiser does not crash the converter", func(t *testing.T) {
		t.Parallel()
		// Drives convertVarSpec's TypesInfo.Defs lookup-miss path.
		_, d := loadFromSource(t, map[string]string{
			"a.go": "package a\n\nvar X = Missing()\n",
		})
		if !d.HasErrors() {
			t.Fatalf("expected an Error diagnostic for unresolved var init")
		}
	})
}
