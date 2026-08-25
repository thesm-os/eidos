// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package acceptancetest_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// cmpModule is the comparison library the builder generator's checks
// import. A generated `_test.go` names it, so a project running the
// generator takes the dependency — which is what these fixtures have
// to model to compile at all.
const cmpModule = "github.com/google/go-cmp"

// requireCmp adds cmpModule to the copied fixture's go.mod, pointed at
// the copy this repository already resolves.
//
// The requirement is real — a consumer of the builder generator adds
// the same line — but its version is not: a fixture pinning one would
// need a go.sum beside it and a network to verify it against, and
// these tests run against a temp directory with neither. The replace
// makes the version unread, which is the arrangement
// [golangtest.Generated.WithRequire] uses for the same reason.
func requireCmp(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "go.mod")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("acceptancetest: reading %s: %v", path, err)
	}
	out := string(src) + "\nrequire " + cmpModule + " v0.0.0\n" +
		"\nreplace " + cmpModule + " => " + moduleDir(t, cmpModule) + "\n"
	// The path is the caller's own t.TempDir plus a fixed basename;
	// there is no external input in it to traverse with.
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil { //nolint:gosec // temp-dir fixture
		t.Fatalf("acceptancetest: writing %s: %v", path, err)
	}
}

// moduleDir returns where the go toolchain has module unpacked, so a
// replace can name a directory rather than a version to fetch.
func moduleDir(t *testing.T, module string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Dir}}", module).Output()
	if err != nil {
		t.Fatalf("acceptancetest: locating %s: %v", module, err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		t.Fatalf("acceptancetest: %s reports no directory; is it required by this module?", module)
	}
	return dir
}
