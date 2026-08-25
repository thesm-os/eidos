// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"slices"
	"testing"
)

// TestConvertFiles covers the file-level conversion path: files
// surface in deterministic order, imports populate per-file, and
// package-level imports are deduplicated.
func TestConvertFiles(t *testing.T) {
	t.Parallel()
	t.Run("files surface in deterministic order across runs", func(t *testing.T) {
		t.Parallel()
		src := map[string]string{
			"b.go": "package a\n",
			"a.go": "package a\n",
			"c.go": "package a\n",
		}
		first := requirePackage(t, src)
		second := requirePackage(t, src)
		firstNames := make([]string, len(first.Files))
		secondNames := make([]string, len(second.Files))
		for i, f := range first.Files {
			firstNames[i] = f.Name
		}
		for i, f := range second.Files {
			secondNames[i] = f.Name
		}
		if !slices.Equal(firstNames, secondNames) {
			t.Fatalf("file order is non-deterministic: %v vs %v", firstNames, secondNames)
		}
	})

	t.Run("per-file imports populate from the syntax tree", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport \"fmt\"\n\nvar _ = fmt.Sprintf\n",
		})
		f := pkg.FileByName("a.go")
		if f == nil {
			t.Fatalf("a.go missing")
		}
		if len(f.Imports) != 1 || f.Imports[0].Path != "fmt" {
			t.Fatalf("file imports = %+v", f.Imports)
		}
	})

	t.Run("package-level imports are deduplicated", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport \"fmt\"\n\nvar _ = fmt.Sprintf\n",
			"b.go": "package a\n\nimport \"fmt\"\n\nvar _ = fmt.Sprintf\n",
		})
		if pkg.ImportByPath("fmt") == nil {
			t.Fatalf("fmt missing in package imports")
		}
		count := 0
		for _, imp := range pkg.Imports {
			if imp.Path == "fmt" {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("expected 1 deduplicated fmt entry, got %d", count)
		}
	})

	t.Run("aliased import preserves the alias", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport f \"fmt\"\n\nvar _ = f.Sprintf\n",
		})
		imp := pkg.FileByName("a.go").Imports[0]
		if imp.Alias != "f" {
			t.Fatalf("import alias = %q, want f", imp.Alias)
		}
	})
}

// TestConvertDecls verifies dispatch routes var / const / type
// declarations to their per-kind converters and skips unknown
// declarations cleanly.
func TestConvertDecls(t *testing.T) {
	t.Parallel()
	t.Run("mixed declaration block routes by kind", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nvar V int\nconst C int = 1\ntype S struct{}\nfunc Do() {}\n",
		})
		if pkg.VariableByName("V") == nil {
			t.Fatalf("V missing")
		}
		if pkg.ConstantByName("C") == nil {
			t.Fatalf("C missing")
		}
		if pkg.StructByName("S") == nil {
			t.Fatalf("S missing")
		}
		if pkg.FunctionByName("Do") == nil {
			t.Fatalf("Do missing")
		}
	})
}

// TestFileBaseName / TestUnquoteImportPath cover the tiny path
// helpers exposed through convertFiles' shape.
func TestFileBaseName(t *testing.T) {
	t.Parallel()
	t.Run("filename derived from full path is the basename", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"sub/a.go": "package sub\n",
		})
		f := pkg.FileByName("a.go")
		if f == nil {
			t.Fatalf("a.go missing — basename derivation failed")
		}
	})
}

func TestUnquoteImportPath(t *testing.T) {
	t.Parallel()
	t.Run("import paths are stored without surrounding quotes", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport \"fmt\"\n\nvar _ = fmt.Sprintf\n",
		})
		imp := pkg.FileByName("a.go").Imports[0]
		if imp.Path != "fmt" {
			t.Fatalf("import Path = %q, want fmt", imp.Path)
		}
	})
}

// TestCollectPackageDoc covers the doc-comment hoisting that
// promotes a `doc.go`-style package comment into [node.Package.DocLines].
func TestCollectPackageDoc(t *testing.T) {
	t.Parallel()
	t.Run("first non-empty package comment surfaces as Package.DocLines", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"doc.go": "// Package a is the package docstring.\npackage a\n",
			"a.go":   "package a\n\ntype S struct{}\n",
		})
		if len(pkg.DocLines) == 0 {
			t.Fatalf("expected Package.DocLines from doc.go, got empty")
		}
	})
}
