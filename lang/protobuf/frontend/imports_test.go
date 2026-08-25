// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/node"
)

// TestConvert_Imports_SourcePos covers the BaseNode.SourcePos
// contract for proto-derived imports: each [node.Import] anchors
// to the file that declared it so downstream consumers can
// resolve an import line back to its origin without re-scanning
// the file's import list.
func TestConvert_Imports_SourcePos(t *testing.T) {
	t.Parallel()

	t.Run("each Import carries the declaring file path on Pos.File", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "imports", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkgs := collectPackages(t, env)
		if len(pkgs) == 0 {
			t.Fatalf("expected at least one Package")
		}
		for _, pkg := range pkgs {
			for _, f := range pkg.Files {
				for _, imp := range f.Imports {
					if imp.Pos().File == "" {
						t.Errorf("Import %q on file %q carries empty Pos.File", imp.Path, f.Path)
					}
				}
			}
			for _, imp := range pkg.Imports {
				if imp.Pos().File == "" {
					t.Errorf("Package-level Import %q carries empty Pos.File", imp.Path)
				}
			}
		}
	})
}

// TestConvert_RecordsImports covers the import-modelling
// contract: each `import "..."` declaration in a proto file
// produces a [node.Import] on the corresponding [node.File], and
// the owning [node.Package] carries the deduplicated union of
// every file's imports. Path is the imported .proto file path
// verbatim; Alias is empty (proto has no import aliases).
//
// The fixture covers two import shapes: an in-tree import that
// resolves through the configured import root, and a well-known
// import (`google/protobuf/timestamp.proto`) that resolves
// through protocompile's bundled descriptors (the frontend's
// [Options.IncludeWellKnown] default is true).
func TestConvert_RecordsImports(t *testing.T) {
	t.Parallel()

	t.Run("imports land on the contributing File and in the Package's deduplicated union", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "imports", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkgs := collectPackages(t, env)
		if got := len(pkgs); got != 1 {
			t.Fatalf("expected exactly 1 package; got %d (%+v)", got, packagePaths(pkgs))
		}
		pkg := pkgs[0]
		fileA := findFile(t, pkg, "a.proto")
		gotFile := importPaths(fileA.Imports)
		if !slices.Contains(gotFile, "b.proto") {
			t.Fatalf("expected a.proto to import b.proto; got %+v", gotFile)
		}
		if !slices.Contains(gotFile, "google/protobuf/timestamp.proto") {
			t.Fatalf("expected a.proto to import the well-known timestamp; got %+v", gotFile)
		}
		for _, imp := range fileA.Imports {
			if imp.Alias != "" {
				t.Fatalf("proto imports carry no alias; got Alias=%q on %q", imp.Alias, imp.Path)
			}
		}
		gotPkg := importPaths(pkg.Imports)
		if !slices.Contains(gotPkg, "b.proto") {
			t.Fatalf("expected Package.Imports to carry b.proto; got %+v", gotPkg)
		}
		if !slices.Contains(gotPkg, "google/protobuf/timestamp.proto") {
			t.Fatalf("expected Package.Imports to carry the well-known timestamp; got %+v", gotPkg)
		}
		seen := map[string]int{}
		for _, imp := range pkg.Imports {
			seen[imp.Path]++
		}
		for path, count := range seen {
			if count != 1 {
				t.Fatalf("Package.Imports should dedup; got %d entries for %q", count, path)
			}
		}
	})
}

// findFile locates the [node.File] with the supplied basename on
// pkg. Fails the test with a useful diagnostic when the file is
// absent — every import test depends on the file existing.
func findFile(t *testing.T, pkg *node.Package, name string) *node.File {
	t.Helper()
	for _, f := range pkg.Files {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("file %q not found in package %q (got %+v)", name, pkg.Path, fileNames(pkg))
	return nil
}

// importPaths returns the Path field of each [node.Import] in imps.
func importPaths(imps []*node.Import) []string {
	out := make([]string, 0, len(imps))
	for _, imp := range imps {
		out = append(out, imp.Path)
	}
	return out
}
