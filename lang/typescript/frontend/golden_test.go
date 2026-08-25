// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/lang/typescript/frontend"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// goldenRoot is the absolute path of testdata/golden, resolved once
// so a fixture's location never depends on the live working
// directory.
var goldenRoot = func() string {
	cwd, err := os.Getwd()
	if err != nil {
		panic("frontend_test: os.Getwd: " + err.Error())
	}
	return filepath.Join(cwd, "testdata", "golden")
}() //nolint:gochecknoglobals // package-init test fixture root

// TestFrontend_Golden pins the exact node graph each fixture
// produces.
//
// Every unit test in this package asserts one fact it was written to
// check, which is why each of them passed while the converter was
// dropping decorator order, collapsing repeated decorators, and
// flattening nothing in a nested union: nobody had thought to assert
// those. A golden file asserts the whole graph, so a change nobody
// anticipated still shows up as a diff.
//
// Run with `-update-golden` to rewrite the expected files from
// current output, then read the diff before committing it.
func TestFrontend_Golden(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(goldenRoot)
	if err != nil {
		t.Fatalf("read %s: %v", goldenRoot, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			t.Parallel()
			runGoldenFixture(t, filepath.Join(goldenRoot, entry.Name()))
		})
	}
}

// runGoldenFixture loads one fixture directory and compares the
// serialised package against its expected.json.
func runGoldenFixture(t *testing.T, dir string) {
	t.Helper()

	f := frontend.New()
	if err := f.SetOptions(opt.New(f.OptionsSchema(), map[string]string{
		"dir": dir,
		// A fixture is a hand-written source file, and one of them
		// carries the generated-file marker on purpose. Parsing them
		// all is the point of the corpus.
		"skip_generated_files": "false",
	})); err != nil {
		t.Fatalf("SetOptions: %v", err)
	}

	st, sink := store.New(), diag.New()
	ctx := &plugin.FrontendContext{
		Store:       st,
		Diag:        sink,
		Registry:    directive.NewRegistry(),
		Parser:      directive.DefaultParser(),
		Cache:       cache.NewNone(),
		Pattern:     "./...",
		Fingerprint: "golden",
	}
	if err := f.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, d := range sink.Diagnostics() {
		if d.Severity == diag.Error {
			t.Fatalf("unexpected error diagnostic: %s", d.Message)
		}
	}

	var pkgs []*node.Package
	st.Nodes().Packages().Range(func(p *node.Package) bool {
		pkgs = append(pkgs, p)
		return true
	})
	if len(pkgs) != 1 {
		t.Fatalf("packages = %d, want exactly 1 per fixture", len(pkgs))
	}

	pipelinetest.MatchesGoldenBytes(t, marshalPackage(t, pkgs[0], dir), filepath.Join(dir, "expected.json"))
}

// marshalPackage serialises pkg with the fixture's own directory
// stripped from every position.
//
// A position carries an absolute path, which differs per machine and
// per checkout. Left in, the corpus would pin the directory the
// fixtures were generated in and fail everywhere else — which is the
// same reproducibility failure the provenance hash exists to prevent,
// arriving through the test suite instead.
func marshalPackage(t *testing.T, pkg *node.Package, dir string) []byte {
	t.Helper()

	body, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		t.Fatalf("marshal package: %v", err)
	}
	body = bytes.ReplaceAll(body, []byte(dir+string(filepath.Separator)), nil)
	body = bytes.ReplaceAll(body, []byte(dir), []byte("."))
	return append(body, '\n')
}
