// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/lang/protobuf/frontend"
	"go.thesmos.sh/eidos/node"
)

// TestConvert_SinglePackage covers the canonical happy path: a
// fixture with one `.proto` source declaring `package x.y.z;`
// produces exactly one [node.Package]. The package's Name is the
// last dotted segment; Path is the full proto-package qualifier.
// The frontend-provenance marker stamps the producing plugin's
// name on the package meta bag per the framework convention.
func TestConvert_SinglePackage(t *testing.T) {
	t.Parallel()

	t.Run("simple fixture produces one node.Package with the documented name, path, and marker", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "simple", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkgs := collectPackages(t, env)
		if got := len(pkgs); got != 1 {
			t.Fatalf("expected exactly 1 package; got %d (%+v)", got, packagePaths(pkgs))
		}
		pkg := pkgs[0]
		const wantPath = "eidos.protobuf.testdata.simple"
		const wantName = "simple"
		if pkg.Path != wantPath {
			t.Fatalf("Package.Path = %q, want %q", pkg.Path, wantPath)
		}
		if pkg.Name != wantName {
			t.Fatalf("Package.Name = %q, want %q", pkg.Name, wantName)
		}
		got, ok := frontend.MetaFrontend.Get(pkg.Meta())
		if !ok {
			t.Fatalf("MetaFrontend missing on simple-fixture package")
		}
		if got != frontend.FrontendName {
			t.Fatalf("MetaFrontend = %q, want %q", got, frontend.FrontendName)
		}
	})
}

// TestConvert_FilesAndPackage_SourcePos covers the
// BaseNode.SourcePos contract for the package / file container
// nodes. Every produced [node.Package] and [node.File] anchors
// to a source-file path so consumers walking the package tree
// can resolve provenance to a declaring source without
// inspecting child declarations.
func TestConvert_FilesAndPackage_SourcePos(t *testing.T) {
	t.Parallel()

	t.Run("Package and File nodes carry the originating proto file path", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "simple", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkgs := collectPackages(t, env)
		if len(pkgs) == 0 {
			t.Fatalf("expected at least one Package")
		}
		for _, pkg := range pkgs {
			if pkg.Pos().File == "" {
				t.Errorf("Package %q carries empty Pos.File", pkg.Path)
			}
			if len(pkg.Files) == 0 {
				t.Errorf("Package %q has no Files entries", pkg.Path)
			}
			for _, f := range pkg.Files {
				if f.Pos().File == "" {
					t.Errorf("File %q (on package %q) carries empty Pos.File", f.Path, pkg.Path)
				}
			}
		}
	})
}

// TestConvert_MergesSamePackageFiles covers the package merge
// rule: multiple `.proto` files declaring the same proto package
// qualifier collapse into a single [node.Package]. The merge
// order is deterministic — alphabetical by file path within the
// import root — so the Files slice on the merged package reflects
// the canonical iteration order regardless of protocompile's
// dependency-resolution order.
func TestConvert_MergesSamePackageFiles(t *testing.T) {
	t.Parallel()

	t.Run("two files in the same proto package produce one merged node.Package", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "multifile-same-pkg", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkgs := collectPackages(t, env)
		if got := len(pkgs); got != 1 {
			t.Fatalf("expected exactly 1 package; got %d (%+v)", got, packagePaths(pkgs))
		}
		pkg := pkgs[0]
		const wantPath = "eidos.protobuf.testdata.multipkg"
		if pkg.Path != wantPath {
			t.Fatalf("Package.Path = %q, want %q", pkg.Path, wantPath)
		}
		if got := len(pkg.Files); got != 2 {
			t.Fatalf("expected 2 contributing files; got %d (%+v)", got, fileNames(pkg))
		}
		if pkg.Files[0].Name >= pkg.Files[1].Name {
			t.Fatalf(
				"Files should be alphabetical by basename; got %q before %q",
				pkg.Files[0].Name, pkg.Files[1].Name,
			)
		}
	})
}

// TestConvert_SplitsAcrossDistinctPackages covers the inverse of
// the merge rule: files spanning two distinct proto package
// qualifiers produce two separate [node.Package] entries. Each
// package's Files slice carries only the sources that declared
// the matching qualifier.
func TestConvert_SplitsAcrossDistinctPackages(t *testing.T) {
	t.Parallel()

	t.Run("two files in two distinct proto packages produce two node.Package entries", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "multifile-split-pkg", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkgs := collectPackages(t, env)
		if got := len(pkgs); got != 2 {
			t.Fatalf("expected exactly 2 packages; got %d (%+v)", got, packagePaths(pkgs))
		}
		gotPaths := packagePaths(pkgs)
		const alpha = "eidos.protobuf.testdata.split.alpha"
		const beta = "eidos.protobuf.testdata.split.beta"
		if !slices.Contains(gotPaths, alpha) || !slices.Contains(gotPaths, beta) {
			t.Fatalf("expected packages %q and %q; got %+v", alpha, beta, gotPaths)
		}
	})
}

// TestConvert_Deterministic pins the frontend's determinism
// contract: two consecutive loads against the same source set
// produce byte-identical [node.Package] slices in store-insertion
// order. The assertion is fixture-driven across every per-construct
// case the frontend produces (package merge, file-level options,
// import records) so a regression in any single contributor
// surfaces here rather than as a downstream cache miss.
func TestConvert_Deterministic(t *testing.T) {
	t.Parallel()
	cases := []string{
		"simple",
		"multifile-same-pkg",
		"multifile-split-pkg",
		"imports",
		"fileoptions",
		"fileoptions-collision",
		"messages",
		"services",
		"wellknown",
		"hostoptions",
	}
	for _, name := range cases {
		t.Run(name+" fixture serializes identically across two consecutive loads", func(t *testing.T) {
			t.Parallel()
			first := serializeFixture(t, name)
			second := serializeFixture(t, name)
			if !bytes.Equal(first, second) {
				t.Fatalf(
					"two loads of %q diverged:\n--- first  ---\n%s\n--- second ---\n%s",
					name, first, second,
				)
			}
		})
	}
}

// serializeFixture runs one Load against the named fixture and
// returns the JSON-marshalled package-slice payload. The encoding
// mirrors the protopipe harness's determinism assertion shape so
// the same byte-stability guarantee is exercised from both the
// per-frontend test (here) and the harness-level test.
func serializeFixture(t *testing.T, name string) []byte {
	t.Helper()
	env := loadFixture(t, name, "./...")
	if env.diag.HasErrors() {
		t.Fatalf("fixture %q produced error diagnostics: %+v", name, env.diag.Diagnostics())
	}
	// musttag flags this because node.Package's fields inherit
	// JSON tags through an embedded BaseNode rather than declaring
	// them on the host struct — the linter's struct-traversal
	// doesn't follow promotion. The marshalled output is well-formed
	// and stable; the byte-equality assertion below is the canonical
	// proof.
	body, err := json.Marshal(collectPackages(t, env)) //nolint:musttag // tags ride on the embedded BaseNode
	if err != nil {
		t.Fatalf("marshal packages for %q: %v", name, err)
	}
	return body
}

// TestConvert_WiresOwnerBackPointers pins the invariant the cache
// split relies on: a freshly-converted package must carry the same
// owner back-pointers a deserialised one does. JSON drops them —
// Owner fields are `json:"-"` to break the cycle — so the cache-hit
// path rewires explicitly. A conversion path that never wired them
// in the first place means a cold run and a warm run hand consumers
// different graphs, and every consumer that walks upward from a
// field (qualified naming, ownership-scoped directives) reads nil
// on the cold one.
func TestConvert_WiresOwnerBackPointers(t *testing.T) {
	t.Parallel()

	t.Run("a freshly converted field resolves its owning struct", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "simple", "./...")
		pkgs := collectPackages(t, env)
		if len(pkgs) != 1 {
			t.Fatalf("expected exactly 1 package; got %d (%+v)", len(pkgs), packagePaths(pkgs))
		}
		for _, s := range pkgs[0].Structs {
			for _, f := range s.Fields {
				if f.Owner == nil {
					t.Errorf("field %q on struct %q has a nil Owner after a fresh conversion", f.Name, s.QName())
				}
			}
		}
	})

	t.Run("a freshly converted file resolves its owning package", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "simple", "./...")
		pkgs := collectPackages(t, env)
		if len(pkgs) != 1 {
			t.Fatalf("expected exactly 1 package; got %d (%+v)", len(pkgs), packagePaths(pkgs))
		}
		for _, f := range pkgs[0].Files {
			if f.Owner == nil {
				t.Errorf("file %q has a nil Owner after a fresh conversion", f.Name)
			}
		}
	})
}

// fileNames returns the Name field of each file contributing to
// pkg — the cheapest diagnostic surface when a Files assertion fails.
func fileNames(pkg *node.Package) []string {
	out := make([]string, 0, len(pkg.Files))
	for _, f := range pkg.Files {
		out = append(out, f.Name)
	}
	return out
}

// collectPackages drains the node-side Packages bucket into a slice
// the caller asserts against. The harness's NodeView accumulates
// every package the frontend produced; the slice order is the
// store's insertion order.
func collectPackages(t *testing.T, env fixtureEnv) []*node.Package {
	t.Helper()
	var out []*node.Package
	env.store.Nodes().Packages().Range(func(p *node.Package) bool {
		out = append(out, p)
		return true
	})
	return out
}

// packagePaths returns the Path field of each package in pkgs —
// the cheapest diagnostic surface when an assertion fails.
func packagePaths(pkgs []*node.Package) []string {
	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, p.Path)
	}
	return out
}
