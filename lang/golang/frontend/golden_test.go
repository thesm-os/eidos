// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/node"
)

// goldenRoot is the absolute path of the testdata/golden directory,
// resolved at package init so [TestFrontend_Golden] never depends on
// the live working directory — sibling tests pivot os.Chdir while
// loading sources and a relative-path resolution would race the
// pivot. The variable is intentionally package-scoped so the helper
// functions that walk it (the per-fixture subtests) share one
// definitive answer.
var goldenRoot = func() string {
	cwd, err := os.Getwd()
	if err != nil {
		panic("frontend/golang_test: os.Getwd: " + err.Error())
	}
	return filepath.Join(cwd, "testdata", "golden")
}() //nolint:gochecknoglobals // package-init test fixture root

// TestFrontend_Golden is the frontend's integration-test suite. Each
// subdirectory of testdata/golden is one fixture pairing a Go source
// tree (every `*.go` file in the directory becomes one entry of the
// frontend's source map) with the expected JSON serialisation of the
// resulting [*node.Package]. A fresh run loads the source, marshals
// the converted package, and asserts byte-equality against
// `expected.json`; running with `-update-golden` rewrites the
// expected file from the current output so reviewers can inspect
// the diff before committing.
//
// The fixture-walker registers one subtest per fixture directory so
// `go test -run TestFrontend_Golden/<fixture>` targets a single
// case, and any new fixture only requires creating a directory
// under testdata/golden.
func TestFrontend_Golden(t *testing.T) {
	t.Parallel()
	// goldenRoot is resolved once at package init so the testdata
	// path is immune to the cwd pivots other parallel tests perform
	// via the loader's os.Chdir / chdirMu pattern.
	root := goldenRoot
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runGoldenFixture(t, filepath.Join(root, name))
		})
	}
}

// runGoldenFixture loads every Go source file under dir as the
// frontend's source map, drives the converter, and asserts the
// resulting [*node.Package] JSON against `dir/expected.json` via
// [pipelinetest.MatchesGoldenBytes]. Fixtures with no `.go` files fail
// fast — every fixture must declare at least one source file.
//
// Multi-file fixtures preserve the file basename as the source-map
// key so the converter sorts them deterministically and per-file
// nodes carry stable names across runs.
func runGoldenFixture(t *testing.T, dir string) {
	t.Helper()
	src := readFixtureSources(t, dir)
	if len(src) == 0 {
		t.Fatalf("fixture %s has no .go source files", dir)
	}
	pkg := requirePackage(t, src)
	body, err := json.MarshalIndent(canonicalisePackage(t, pkg), "", "  ")
	if err != nil {
		t.Fatalf("marshal package: %v", err)
	}
	body = append(body, '\n')
	pipelinetest.MatchesGoldenBytes(t, body, filepath.Join(dir, "expected.json"))
}

// readFixtureSources collects every `*.go` file directly under dir
// into a basename-keyed map suitable for the frontend's source-map
// fixture loader. Files in nested subdirectories are intentionally
// ignored — multi-file fixtures flatten the package layout so the
// converter sees one Go package per fixture directory.
func readFixtureSources(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read fixture %s: %v", dir, err)
	}
	out := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name())) //nolint:gosec // path is test-supplied fixture
		if err != nil {
			t.Fatalf("read %s/%s: %v", dir, entry.Name(), err)
		}
		out[entry.Name()] = string(body)
	}
	return out
}

// canonicalisePackage produces a deterministic, host-independent
// JSON view of pkg suitable for golden-file comparison. The
// transformation:
//
//   - Strips the temp-directory prefix from every source position
//     so the golden survives runs from different working
//     directories. Positions surface as basename:line:col.
//   - Walks the package via [node.Walk] to ensure every reachable
//     position is rewritten in place.
//
// The transformation operates on a marshal-decode round-trip so
// the original in-memory tree is left untouched — important
// because the converter's [*meta.Bag] entries also feed into the
// JSON output through their own MarshalJSON.
func canonicalisePackage(t *testing.T, pkg *node.Package) any {
	t.Helper()
	// *directive.Directive carries no json tags, so its fields surface
	// under their Go-capitalised names. The golden pins that verbatim
	// rather than chasing tag changes — musttag's warning is moot here.
	raw, err := json.Marshal(pkg) //nolint:musttag
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rewriteTempPaths(decoded)
	return sortMetaMaps(decoded)
}

// rewriteTempPaths walks v in place and rewrites every string value
// whose contents look like a host filesystem path leaking from the
// loader's temp directory. macOS resolves `os.TempDir()` to
// `/var/folders/...` while the actual files surface as
// `/private/var/folders/...` (the `/private` symlink prefix), so both
// forms must be recognised. Each match collapses to its basename so
// the golden survives runs from different temp directories.
//
// Non-path strings are left untouched — fields like the package
// import path (`example.com/golangtest`) must remain stable.
func rewriteTempPaths(v any) {
	tmp := os.TempDir()
	// macOS resolves the user temp dir to `/var/folders/...` but the
	// kernel surfaces the same files via `/private/var/folders/...`,
	// so both shapes must be recognised. Plain string concatenation
	// is correct here — filepath.Join would treat "/private" as a
	// path-with-separator literal and trip up gocritic.
	prefixes := []string{tmp, "/private" + tmp}
	rewriteTempPathsWith(v, prefixes)
}

// rewriteTempPathsWith is the recursive worker for [rewriteTempPaths].
// Splitting it out keeps the prefix calculation off the hot path so
// the temp-dir lookup happens once per fixture rather than once per
// node.
func rewriteTempPathsWith(v any, prefixes []string) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if s, ok := val.(string); ok && hasAnyPrefix(s, prefixes) {
				t[k] = filepath.Base(s)
				continue
			}
			rewriteTempPathsWith(val, prefixes)
		}
	case []any:
		for _, val := range t {
			rewriteTempPathsWith(val, prefixes)
		}
	}
}

// hasAnyPrefix reports whether s starts with any of the supplied
// prefixes. Pulled out so the path-rewriter stays readable.
func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if len(s) >= len(p) && s[:len(p)] == p {
			return true
		}
	}
	return false
}

// sortMetaMaps converts every map[string]any in v into a sorted
// representation so the golden output is determinstic across runs.
// JSON object key ordering is unspecified; without normalisation
// the same meta.Bag could marshal in different orders on different
// Go versions.
func sortMetaMaps(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for k := range typed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf := &bytes.Buffer{}
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			buf.Write(kb)
			buf.WriteByte(':')
			vb, _ := json.Marshal(sortMetaMaps(typed[k]))
			buf.Write(vb)
		}
		buf.WriteByte('}')
		var out any
		_ = json.Unmarshal(buf.Bytes(), &out)
		return out
	case []any:
		for i, val := range typed {
			typed[i] = sortMetaMaps(val)
		}
		return typed
	}
	return v
}
