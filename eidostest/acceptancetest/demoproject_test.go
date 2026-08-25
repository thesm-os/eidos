// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package acceptancetest_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/eidostest/acceptancetest"
	"go.thesmos.sh/eidos/manifest"
)

// demoFixture is the path to the in-tree demoproject testdata
// relative to this package's test files. demoproject ships
// representative +gen: directives for six generators — repo,
// builder, mock, register, enum and sentinel — and is the
// canonical end-to-end acceptance fixture. The list grows; no
// assertion in this file may be keyed to its present membership.
const demoFixture = "../testdata/demoproject"

// TestRunOnDemoProject pins the binary against the demoproject
// fixture as a full end-to-end scenario: run produces the
// expected generated files alongside source, exits cleanly,
// and writes a manifest. This is the high-value acceptance
// test — every plugin in the pipeline contributes, the routing
// layer composes filenames, and the backend renders + writes
// real Go files.
func TestRunOnDemoProject(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	acceptancetest.CopyDir(t, demoFixture, workdir)

	res := acceptancetest.RunCmd(t, workdir, "run", "./...")
	if res.ExitCode != 0 {
		t.Fatalf("run on demoproject exit %d\nstderr:\n%s\nstdout:\n%s",
			res.ExitCode, res.Stderr, res.Stdout)
	}

	// Generated files appear alongside source — one per
	// (plugin, source-struct) pair that opted in via +gen: directive.
	for _, rel := range []string{
		// repogen targets blog.Article and blog.User
		"blog/article_repo.go",
		"blog/user_repo.go",

		// builder targets blog.Article, blog.User, blog.Comment
		"blog/article_builder.gen.go",
		"blog/user_builder.gen.go",
		"blog/comment_builder.gen.go",

		// registrygen targets blog.Article
		"blog/article_registry.go",

		// mockgen targets the user-authored blog.Searcher interface
		// plus the repogen-emitted Article/User Repository interfaces
		"blog/searcher_mock_test.go",
		"blog/article_mock_test.go",
		"blog/user_mock_test.go",

		// enum (multi-output) targets blog.Status — production
		// surface in _enum.gen.go, paired round-trip tests in
		// _enum.gen_test.go (auto-shifted to package blog_test).
		"blog/status_enum.gen.go",
		"blog/status_enum.gen_test.go",

		// sentinel scans the +gen:sentinel-annotated blog
		// package for Err* vars + custom error types and emits
		// pinned tests into a sibling _sentinel.gen_test.go file.
		// Anchored to auth_errors.go (the first contributing
		// file in source order).
		"blog/auth_errors_sentinel.gen_test.go",
	} {
		path := filepath.Join(workdir, rel)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected generated file %s missing: %v", rel, err)
		}
	}

	// The manifest records the run for change tracking + prune
	// + check workflows.
	manifestPath := filepath.Join(workdir, ".eidos-reference", "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Errorf("manifest.json should be written; got: %v", err)
	}
}

// TestRunOnDemoProject_GeneratedOutputCompiles pins the
// byte-level correctness of the generated output: after the
// binary runs, `go build ./...` of the demoproject tree must
// succeed. A regression in any backend rendering pass, import
// resolution, or `gofmt` finalisation surfaces here as a
// compile error.
func TestRunOnDemoProject_GeneratedOutputCompiles(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	acceptancetest.CopyDir(t, demoFixture, workdir)

	runRes := acceptancetest.RunCmd(t, workdir, "run", "./...")
	if runRes.ExitCode != 0 {
		t.Fatalf("run exit %d\nstderr:\n%s", runRes.ExitCode, runRes.Stderr)
	}

	cmd := exec.CommandContext(t.Context(), "go", "build", "./...")
	cmd.Dir = workdir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build of generated demoproject failed: %v\nstderr:\n%s",
			err, stderr.String())
	}
	// `go build ./...` skips _test.go files. Run `go vet ./...`
	// after the build so the generator's test-side output (e.g.
	// the sentinel plugin's _sentinel.gen_test.go) also passes
	// language-level checks — vet's scanning includes _test.go
	// files in the package compilation graph.
	vetCmd := exec.CommandContext(t.Context(), "go", "vet", "./...")
	vetCmd.Dir = workdir
	var vetStderr bytes.Buffer
	vetCmd.Stderr = &vetStderr
	if err := vetCmd.Run(); err != nil {
		t.Fatalf("go vet of generated demoproject failed: %v\nstderr:\n%s",
			err, vetStderr.String())
	}
}

// TestRunOnDemoProject_IsIdempotent pins the determinism
// contract end-to-end: running the binary twice against the
// same source tree produces byte-identical generated files.
// Non-deterministic output anywhere in the pipeline (map
// iteration, time-derived values, unstable import sets,
// non-deterministic slot ordering) surfaces as a snapshot diff
// across the two runs.
//
// The comparison is over the whole `.go` tree, and the generated
// set is derived by differencing a pre-run snapshot against a
// post-run one rather than by recognising filenames. Both runs are
// separate processes, so `-race` says nothing here; what the gate
// is exposed to instead is scope, and scope is what the subtests
// below assert.
func TestRunOnDemoProject_IsIdempotent(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	acceptancetest.CopyDir(t, demoFixture, workdir)

	// The pre-run baseline is what lets the snapshot be
	// allowlist-free. workdir is a t.TempDir seeded by CopyDir from
	// a version-controlled fixture, so "which files did the run
	// produce" has an exact answer — post ∖ pre — and no filename
	// convention has to stand in for it. A suffix filter is
	// fail-open in the omission direction: a file it does not match
	// is absent from both snapshots, so deleting it is not a
	// difference.
	source := snapshotGenerated(t, workdir)

	firstRun := acceptancetest.RunCmd(t, workdir, "run", "./...")
	if firstRun.ExitCode != 0 {
		t.Fatalf("first run exit %d\nstderr:\n%s", firstRun.ExitCode, firstRun.Stderr)
	}
	first := snapshotGenerated(t, workdir)

	secondRun := acceptancetest.RunCmd(t, workdir, "run", "./...")
	if secondRun.ExitCode != 0 {
		t.Fatalf("second run exit %d\nstderr:\n%s", secondRun.ExitCode, secondRun.Stderr)
	}
	second := snapshotGenerated(t, workdir)

	generated := newPaths(first, source)

	// The subtests below read the three snapshots and never touch
	// the filesystem, so they are safe to run in parallel against
	// one shared workdir. t.TempDir's cleanup is deferred until
	// every parallel subtest has finished.

	t.Run("the generated tree is byte-identical across two runs", func(t *testing.T) {
		t.Parallel()
		if maps.Equal(first, second) {
			return
		}
		t.Errorf("generated tree differs across two runs (non-deterministic output)")
		for path, hash1 := range first {
			if hash2, ok := second[path]; !ok {
				t.Errorf("  file disappeared on second run: %s", path)
			} else if hash1 != hash2 {
				t.Errorf("  file content changed: %s (%s → %s)", path, hash1[:12], hash2[:12])
			}
		}
		for path := range second {
			if _, ok := first[path]; !ok {
				t.Errorf("  file appeared only on second run: %s", path)
			}
		}
	})

	t.Run("the generated set is every file the run produced", func(t *testing.T) {
		t.Parallel()
		// The manifest is a cross-check, not the oracle: a file the
		// pipeline writes without recording would be invisible to
		// it, which is why the comparison above walks the tree. Here
		// it answers the one question the walk cannot — is the set
		// being compared the whole set?
		//
		// The cardinality equality alone is not enough. Two empty
		// snapshots compare equal, so a run that emitted nothing
		// would satisfy 0 == 0 and leave the determinism subtest
		// asserting over nothing. The non-empty arm is the floor.
		claimed := manifestOutputCount(t, workdir)
		switch {
		case len(generated) == 0:
			t.Errorf("run produced no generated files (manifest claims %d outputs); "+
				"the cross-run comparison would have nothing to compare", claimed)
		case len(generated) != claimed:
			t.Errorf("run produced %d generated files, manifest claims %d outputs\ngenerated set:\n  %s",
				len(generated), claimed, strings.Join(slices.Sorted(maps.Keys(generated)), "\n  "))
		}
	})

	t.Run("the enum plugin's two outputs are byte-compared across runs", func(t *testing.T) {
		t.Parallel()
		// enum is the fixture's only multi-output plugin: one origin
		// fans onto two targets, and the _enum.gen_test.go target trips
		// the external-test package shift. Longest routing path in
		// the fixture, so the one most worth hashing.
		assertCompared(t, first, second, "blog/status_enum.gen.go", "blog/status_enum.gen_test.go")
	})

	t.Run("the sentinel plugin's output is byte-compared across runs", func(t *testing.T) {
		t.Parallel()
		// sentinel is the only plugin that picks its anchor node by
		// scanning a whole package, so a collection-order change
		// renames its output rather than corrupting it — the
		// disappeared/appeared arms above are what catch that, and
		// they are unreachable for a file neither snapshot holds.
		assertCompared(t, first, second, "blog/auth_errors_sentinel.gen_test.go")
	})

	t.Run("the run leaves fixture source byte-identical", func(t *testing.T) {
		t.Parallel()
		// Widening the walk pulls the fixture's own source into the
		// compared set. No in-tree plugin writes back to source, and
		// this pins that. A future annotator that legitimately
		// rewrites source needs an explicit exclusion here, not a
		// restored allowlist.
		for path, before := range source {
			after, ok := second[path]
			switch {
			case !ok:
				t.Errorf("run deleted fixture source file: %s", path)
			case after != before:
				t.Errorf("run rewrote fixture source file: %s (%s → %s)", path, before[:12], after[:12])
			}
		}
	})
}

// TestSnapshotGenerated pins the snapshot helper's scope without a
// binary run. It is the regression barrier that survives any future
// fixture change: the defect this test exists for was a filename
// allowlist that silently excluded two plugins, and only a
// direct assertion on the helper catches its return.
func TestSnapshotGenerated(t *testing.T) {
	t.Parallel()

	t.Run("the snapshot helper hashes a file matching no plugin suffix", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		seedFile(t, dir, "pkg/foo_enum.go", "package pkg\n")

		snapshot := snapshotGenerated(t, dir)
		if _, ok := snapshot["pkg/foo_enum.go"]; !ok {
			t.Errorf("snapshot should hash every .go file; pkg/foo_enum.go missing from %v",
				slices.Sorted(maps.Keys(snapshot)))
		}
	})

	t.Run("the snapshot helper ignores files outside the Go tree", func(t *testing.T) {
		t.Parallel()
		// The extension filter is a scope decision, not a stability
		// workaround: this gate's subject is rendered Go, and hashing
		// the manifest or the cache blobs would make a bookkeeping
		// format change read as non-determinism.
		dir := t.TempDir()
		seedFile(t, dir, ".eidos-reference/manifest.json", `{"version":1,"outputs":[]}`)
		seedFile(t, dir, "README.md", "# fixture\n")

		if snapshot := snapshotGenerated(t, dir); len(snapshot) != 0 {
			t.Errorf("snapshot should hold only .go files; got %v", slices.Sorted(maps.Keys(snapshot)))
		}
	})
}

// newPaths returns the entries of after whose paths are absent from
// before — the set difference that turns two tree snapshots into
// "what this run produced". Allocates one map sized to after.
func newPaths(after, before map[string]string) map[string]string {
	out := make(map[string]string, len(after))
	for path, hash := range after {
		if _, existed := before[path]; !existed {
			out[path] = hash
		}
	}
	return out
}

// assertCompared fails t for every path that is not a key of both
// snapshots. A path missing from both is the failure mode worth
// naming: the cross-run comparison silently says nothing about it.
func assertCompared(t *testing.T, first, second map[string]string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		_, inFirst := first[path]
		_, inSecond := second[path]
		if !inFirst || !inSecond {
			t.Errorf("%s is not compared across runs (first run: %t, second run: %t)",
				path, inFirst, inSecond)
		}
	}
}

// manifestOutputCount returns the number of outputs the run recorded
// in workdir's manifest, reading it through the same [manifest.Read]
// the binary's prune and check paths use so a schema change surfaces
// here rather than in a hand-rolled decoder.
func manifestOutputCount(t *testing.T, workdir string) int {
	t.Helper()
	m, err := manifest.Read(filepath.Join(workdir, ".eidos-reference", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	return len(m.Outputs)
}

// seedFile creates rel — a slash-separated path, with its parent
// directories — under dir holding body. Used by the snapshot
// helper's unit tests, which need files on disk but no pipeline run.
func seedFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// snapshotGenerated returns a path→content-hash map of every `.go`
// file under workdir, keyed by slash-separated relative path. The
// caller subtracts a pre-run snapshot to obtain the set the run
// produced; hashes rather than bodies keep two whole-tree snapshots
// cheap to hold at once.
//
// The walk carries no filename allowlist, deliberately. Suffix
// filtering is fail-open in the omission direction — a file matching
// no suffix is absent from both snapshots, so its disappearance is
// not a difference and a plugin emitting nothing at all still
// compares equal. It also has to be maintained by hand against every
// generator the binary registers, and was not: the enum and sentinel
// plugins landed after the list and were never added to it.
//
// The `.go` extension filter is the one exclusion, and it is scope
// rather than a stability workaround. This gate's subject is
// rendered Go; hashing the manifest and cache blobs under
// `.eidos-reference/` would make a bookkeeping format change read as
// non-determinism. Fixture source is in scope and stays in scope — a
// pipeline path that writes back to source must turn this red.
func snapshotGenerated(t *testing.T, workdir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(workdir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || filepath.Ext(path) != ".go" {
			return walkErr
		}
		// gosec G122: controlled test workdir, no symlink threat.
		data, readErr := os.ReadFile(path) //nolint:gosec
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		rel, relErr := filepath.Rel(workdir, path)
		if relErr != nil {
			return fmt.Errorf("rel %s: %w", path, relErr)
		}
		sum := sha256.Sum256(data)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("snapshotGenerated: %v", err)
	}
	return out
}
