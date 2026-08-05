// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package acceptancetest_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/eidostest/acceptancetest"
)

// multipkgFixture is the path to the in-tree multipkg testdata
// project, which exercises routing cases no single-package fixture
// reaches: two packages sharing a short name, cross-package generic
// instantiation, an internal-package path, and both the scoped and
// unscoped `+gen:out` forms on one type.
const multipkgFixture = "../testdata/multipkg"

// multipkgConfig is the fixture's config filename. It is not the
// reference binary's brand-derived default, so every invocation
// passes it explicitly rather than relying on upward discovery.
const multipkgConfig = ".eidos.yaml"

// TestRunOnMultipkg drives the reference binary over the multipkg
// fixture.
//
// The fixture shipped with a README specifying five acceptance
// assertions and no test that made them. That is worse than an
// absent fixture: ~370 lines across seven packages read as covered
// while nothing executed them, so the cases it was built for — an
// import-alias collision between two packages both named `events`,
// generic instantiation across a package boundary — were carried by
// no assertion anywhere in the suite.
func TestRunOnMultipkg(t *testing.T) {
	t.Parallel()

	t.Run("the run completes without diagnostics", func(t *testing.T) {
		t.Parallel()
		workdir := t.TempDir()
		acceptancetest.CopyDir(t, multipkgFixture, workdir)
		res := acceptancetest.RunCmd(t, workdir, "run", "--config", multipkgConfig, "./...")
		if res.ExitCode != 0 {
			t.Fatalf("run exit %d\nstderr:\n%s", res.ExitCode, res.Stderr)
		}
	})

	t.Run("generated output compiles and vets", func(t *testing.T) {
		t.Parallel()
		// This subtest carries most of the fixture's value. Two
		// packages named `events` cannot both be imported into
		// api/handler.go unless the writer's alias-suffix escalation
		// resolves the collision, and a generic instantiated across
		// a package boundary cannot render without correct type-arg
		// qualification. Both failures are compile errors, so the
		// compiler is the assertion — no hand-written check would be
		// as thorough.
		workdir := generateMultipkg(t)
		runGoTool(t, workdir, "build", "./...")
		// build skips _test.go files, and mockgen's output is
		// _mock_test.go, so vet is what reaches the mocks.
		runGoTool(t, workdir, "vet", "./...")
	})

	t.Run("the unscoped out= directive routes every plugin's output", func(t *testing.T) {
		t.Parallel()
		// `+gen:out product_codegen.go` on domain.Product carries no
		// plugin selector, so it is the standalone unscoped form and
		// applies to each plugin emitting against that type.
		workdir := generateMultipkg(t)
		if !fileExists(t, filepath.Join(workdir, "domain", "product_codegen.go")) {
			t.Fatalf("unscoped out= produced no product_codegen.go; got %v",
				generatedFiles(t, workdir))
		}
	})

	t.Run("the plugin-scoped out= directive routes only its own plugin", func(t *testing.T) {
		t.Parallel()
		// `+gen:out product_mock_test.go plugin=mockgen` names one
		// plugin, so it must move mockgen's output and leave every
		// other plugin's where the unscoped directive put it.
		workdir := generateMultipkg(t)
		if !fileExists(t, filepath.Join(workdir, "domain", "product_mock_test.go")) {
			t.Fatalf("plugin-scoped out= produced no product_mock_test.go; got %v",
				generatedFiles(t, workdir))
		}
	})

	t.Run("a second run is byte-identical", func(t *testing.T) {
		t.Parallel()
		// Determinism over a seven-package graph reaches ordering
		// paths a single package cannot: cross-package import sets,
		// alias assignment under collision, and slot ordering across
		// several contributing plugins.
		workdir := generateMultipkg(t)
		first := hashTree(t, workdir)
		res := acceptancetest.RunCmd(t, workdir, "run", "--config", multipkgConfig, "./...")
		if res.ExitCode != 0 {
			t.Fatalf("second run exit %d\nstderr:\n%s", res.ExitCode, res.Stderr)
		}
		if second := hashTree(t, workdir); second != first {
			t.Fatalf("second run differed from the first:\n%s\nvs\n%s", first, second)
		}
	})
}

// generateMultipkg copies the fixture into a fresh temp directory,
// runs the reference binary over it, and returns the working
// directory.
func generateMultipkg(t *testing.T) string {
	t.Helper()
	workdir := t.TempDir()
	acceptancetest.CopyDir(t, multipkgFixture, workdir)
	res := acceptancetest.RunCmd(t, workdir, "run", "--config", multipkgConfig, "./...")
	if res.ExitCode != 0 {
		t.Fatalf("run exit %d\nstderr:\n%s", res.ExitCode, res.Stderr)
	}
	return workdir
}

// runGoTool runs `go <args...>` in workdir and fails the test with
// the tool's stderr when it exits non-zero.
func runGoTool(t *testing.T, workdir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = workdir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go %s failed: %v\nstderr:\n%s", strings.Join(args, " "), err, stderr.String())
	}
}

// fileExists reports whether path names an existing regular file.
func fileExists(t *testing.T, path string) bool {
	t.Helper()
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// generatedFiles lists every .go file under root relative to it, so
// a failing assertion can report what was produced instead of only
// what was missing.
func generatedFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	walkGoFiles(t, root, func(rel string, _ []byte) { out = append(out, rel) })
	sort.Strings(out)
	return out
}

// hashTree returns a stable digest of every .go file under root,
// keyed by relative path, for the idempotency comparison.
func hashTree(t *testing.T, root string) string {
	t.Helper()
	sums := map[string]string{}
	walkGoFiles(t, root, func(rel string, body []byte) {
		sum := sha256.Sum256(body)
		sums[rel] = hex.EncodeToString(sum[:])
	})
	paths := make([]string, 0, len(sums))
	for p := range sums {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, p := range paths {
		b.WriteString(p + " " + sums[p] + "\n")
	}
	return b.String()
}

// walkGoFiles calls fn for every .go file under root, with the path
// relative to root and the file's contents.
func walkGoFiles(t *testing.T, root string, fn func(rel string, body []byte)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".go" {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("relativise %s: %w", path, relErr)
		}
		body, readErr := os.ReadFile(path) //nolint:gosec // path is under the test's own temp dir
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		fn(filepath.ToSlash(rel), body)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}
