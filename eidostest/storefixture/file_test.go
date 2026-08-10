// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package storefixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/node"
)

// fileFixture builds a package with one file carrying a plain and an
// aliased import — the shape every assertion below reads.
func fileFixture() *node.Package {
	return storefixture.New().
		Package("shop", "example.com/shop").
		File("tier.go", func(f *storefixture.FileBuilder) {
			f.Import("context")
			f.ImportAs("pb", "example.com/gen/shopv1")
		}).
		PackageNode()
}

func TestFile(t *testing.T) {
	t.Parallel()

	t.Run("declares a file reachable by name", func(t *testing.T) {
		t.Parallel()
		f := fileFixture().FileByName("tier.go")
		if f == nil {
			t.Fatal("FileByName found no tier.go")
		}
		if f.Path != "shop/tier.go" {
			t.Fatalf("Path = %q, want shop/tier.go — the same directory a "+
				"declaration's synthetic position uses", f.Path)
		}
	})

	t.Run("records imports in source order", func(t *testing.T) {
		t.Parallel()
		// Order is a source fact and a resolver reporting the first
		// match depends on it.
		imports := fileFixture().FileByName("tier.go").Imports
		if len(imports) != 2 {
			t.Fatalf("got %d imports, want 2", len(imports))
		}
		if imports[0].Path != "context" || imports[1].Path != "example.com/gen/shopv1" {
			t.Fatalf("imports out of order: %q then %q", imports[0].Path, imports[1].Path)
		}
	})

	t.Run("an alias becomes the local name", func(t *testing.T) {
		t.Parallel()
		// The whole reason a file-scoped import block is worth
		// modelling: without the alias the qualifier is derivable from
		// the path's last segment, and with it the derived answer is
		// wrong.
		imp := fileFixture().FileByName("tier.go").ImportByPath("example.com/gen/shopv1")
		if imp == nil {
			t.Fatal("ImportByPath found no aliased import")
		}
		if got := imp.LocalName(); got != "pb" {
			t.Fatalf("LocalName = %q, want pb; the derived answer would be shopv1", got)
		}
	})

	t.Run("an unaliased import derives its local name from the path", func(t *testing.T) {
		t.Parallel()
		imp := fileFixture().FileByName("tier.go").ImportByPath("context")
		if imp.Alias != "" {
			t.Fatalf("Alias = %q, want empty for an unaliased import", imp.Alias)
		}
		if got := imp.LocalName(); got != "context" {
			t.Fatalf("LocalName = %q, want context", got)
		}
	})

	t.Run("the package union carries every file's imports", func(t *testing.T) {
		t.Parallel()
		// A frontend produces both views. A fixture populating only the
		// per-file one shows a package importing nothing while its
		// files import plenty, which is a shape no run produces.
		pkg := fileFixture()
		if pkg.ImportByPath("example.com/gen/shopv1") == nil {
			t.Fatal("the package union lost a file's import")
		}
		if got := pkg.ImportByPath("example.com/gen/shopv1").Alias; got != "pb" {
			t.Fatalf("union alias = %q, want pb", got)
		}
	})

	t.Run("naming a file twice accumulates into one", func(t *testing.T) {
		t.Parallel()
		// Two files of one name is a shape no filesystem produces, and
		// a second File call reads as adding to the first.
		pkg := storefixture.New().
			Package("shop", "example.com/shop").
			File("tier.go", func(f *storefixture.FileBuilder) { f.Import("context") }).
			File("tier.go", func(f *storefixture.FileBuilder) { f.Import("errors") }).
			PackageNode()
		if len(pkg.Files) != 1 {
			t.Fatalf("got %d files named tier.go, want 1", len(pkg.Files))
		}
		if len(pkg.Files[0].Imports) != 2 {
			t.Fatalf("got %d imports, want both calls' worth", len(pkg.Files[0].Imports))
		}
	})

	t.Run("the union does not collapse one path under two names", func(t *testing.T) {
		t.Parallel()
		// Two files binding one path to different local names are two
		// distinct facts; dedup by path alone loses the one a resolver
		// needs.
		pkg := storefixture.New().
			Package("shop", "example.com/shop").
			File("a.go", func(f *storefixture.FileBuilder) { f.ImportAs("pb", "example.com/gen") }).
			File("b.go", func(f *storefixture.FileBuilder) { f.ImportAs("gen", "example.com/gen") }).
			PackageNode()
		var aliases []string
		for _, imp := range pkg.Imports {
			if imp.Path == "example.com/gen" {
				aliases = append(aliases, imp.Alias)
			}
		}
		if len(aliases) != 2 {
			t.Fatalf("union holds %v, want both aliases", aliases)
		}
	})

	t.Run("declaring the same import twice records it once", func(t *testing.T) {
		t.Parallel()
		pkg := storefixture.New().
			Package("shop", "example.com/shop").
			Import("context").
			File("tier.go", func(f *storefixture.FileBuilder) { f.Import("context") }).
			PackageNode()
		var n int
		for _, imp := range pkg.Imports {
			if imp.Path == "context" {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("union holds context %d times, want 1", n)
		}
	})

	t.Run("a nil callback declares the file and nothing else", func(t *testing.T) {
		t.Parallel()
		pkg := storefixture.New().Package("shop", "example.com/shop").
			File("empty.go", nil).PackageNode()
		if f := pkg.FileByName("empty.go"); f == nil || len(f.Imports) != 0 {
			t.Fatalf("File(name, nil) produced %+v", f)
		}
	})
}

func TestBuilderImportAs(t *testing.T) {
	t.Parallel()

	t.Run("records an alias on the package union", func(t *testing.T) {
		t.Parallel()
		pkg := storefixture.New().
			ImportAs("pb", "example.com/gen/shopv1").
			PackageNode()
		imp := pkg.ImportByPath("example.com/gen/shopv1")
		if imp == nil || imp.LocalName() != "pb" {
			t.Fatalf("ImportAs produced %+v", imp)
		}
	})

	t.Run("Import still records no alias", func(t *testing.T) {
		t.Parallel()
		pkg := storefixture.New().Import("context").PackageNode()
		if got := pkg.ImportByPath("context"); got == nil || got.Alias != "" {
			t.Fatalf("Import produced %+v, want no alias", got)
		}
	})
}
