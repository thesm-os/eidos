// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package-internal tests for prune's source-resolution helpers.
//
// [goneSources] and [pruneCandidates] are unexported and have no
// behaviour reachable through PruneCommand that pins their edge cases
// — a missing go.mod, a prefix-sharing module path, a non-directory
// where a package should be. Those are decided here; the
// command-level behaviour stays blackbox in prune_test.go.
package cli

import (
	"os"
	"path/filepath"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
)

// newModule writes a go.mod declaring modPath at a fresh temp root
// and returns the root. Package directories are created by the caller
// so each subtest states exactly which sources exist.
func newModule(t *testing.T, modPath string) string {
	t.Helper()
	root := t.TempDir()
	body := "module " + modPath + "\n\ngo 1.26\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(body), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	return root
}

// TestGoneSources covers the filesystem half of the orphan
// classification: which candidate import paths no longer have a
// directory inside the module.
//
// Every uncertain case must resolve to "not gone". Being wrong in
// that direction leaves an entry to be pruned on a later run; being
// wrong in the other deletes a file the operator still wanted.
func TestGoneSources(t *testing.T) {
	t.Parallel()

	t.Run("a package with no directory is gone", func(t *testing.T) {
		t.Parallel()
		root := newModule(t, "example.com/m")
		got := goneSources(root, []string{"example.com/m/gone"})
		if _, ok := got["example.com/m/gone"]; !ok {
			t.Fatalf("expected the missing package to be gone; got %+v", got)
		}
	})

	t.Run("a package whose directory exists is not gone", func(t *testing.T) {
		t.Parallel()
		root := newModule(t, "example.com/m")
		if err := os.MkdirAll(filepath.Join(root, "here"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if got := goneSources(root, []string{"example.com/m/here"}); len(got) != 0 {
			t.Fatalf("an existing directory must never be gone; got %+v", got)
		}
	})

	t.Run("an empty directory is not gone", func(t *testing.T) {
		t.Parallel()
		// A package that fails to load is not a package that was
		// deleted, and the loader's silence cannot tell them apart.
		root := newModule(t, "example.com/m")
		if err := os.MkdirAll(filepath.Join(root, "empty"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if got := goneSources(root, []string{"example.com/m/empty"}); len(got) != 0 {
			t.Fatalf("an empty directory must not be gone; got %+v", got)
		}
	})

	t.Run("a path outside the module is never gone", func(t *testing.T) {
		t.Parallel()
		root := newModule(t, "example.com/m")
		if got := goneSources(root, []string{"github.com/other/pkg"}); len(got) != 0 {
			t.Fatalf("another module's lifecycle is not ours; got %+v", got)
		}
	})

	t.Run("a module path sharing a prefix is not treated as inside", func(t *testing.T) {
		t.Parallel()
		// `example.com/m` must not swallow `example.com/mother`: a
		// string-prefix test would map it to <root>/other, find
		// nothing, and delete another module's output.
		root := newModule(t, "example.com/m")
		if got := goneSources(root, []string{"example.com/mother/pkg"}); len(got) != 0 {
			t.Fatalf("prefix-sharing module must be outside; got %+v", got)
		}
	})

	t.Run("the module root itself is not gone when it exists", func(t *testing.T) {
		t.Parallel()
		root := newModule(t, "example.com/m")
		if got := goneSources(root, []string{"example.com/m"}); len(got) != 0 {
			t.Fatalf("the module root directory exists; got %+v", got)
		}
	})

	t.Run("a file where a package directory should be is gone", func(t *testing.T) {
		t.Parallel()
		root := newModule(t, "example.com/m")
		if err := os.WriteFile(filepath.Join(root, "notadir"), []byte("x"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if got := goneSources(root, []string{"example.com/m/notadir"}); len(got) != 1 {
			t.Fatalf("a non-directory cannot hold a package; got %+v", got)
		}
	})

	t.Run("no go.mod yields no classification", func(t *testing.T) {
		t.Parallel()
		// Without a module identity nothing can be resolved, so
		// nothing is deletable — the conservative direction.
		if got := goneSources(t.TempDir(), []string{"example.com/m/gone"}); got != nil {
			t.Fatalf("no module means no verdict; got %+v", got)
		}
	})

	t.Run("an unparseable go.mod yields no classification", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("not a module file\n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if got := goneSources(root, []string{"example.com/m/gone"}); got != nil {
			t.Fatalf("an unreadable module path must not classify; got %+v", got)
		}
	})

	t.Run("no candidates yields nil", func(t *testing.T) {
		t.Parallel()
		root := newModule(t, "example.com/m")
		if got := goneSources(root, nil); got != nil {
			t.Fatalf("nothing to probe means nil; got %+v", got)
		}
	})
}

// TestPruneCandidates covers which entries are worth probing at all.
// Restricting the set keeps the filesystem work proportional to the
// drift rather than to the manifest's size.
func TestPruneCandidates(t *testing.T) {
	t.Parallel()

	entry := func(dir, file, importPath, pipelineID string) manifest.Output {
		return manifest.Output{
			Target: emit.Target{
				Dir: dir, Filename: file, Package: "x", ImportPath: importPath,
			},
			PipelineID: pipelineID,
		}
	}

	t.Run("returns out-of-scope entries for this pipeline", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("r")
		prev.Add(entry("b", "a.go", "example.com/b", "p"))
		got := pruneCandidates(prev, map[string]struct{}{}, "p")
		if len(got) != 1 || got[0] != "example.com/b" {
			t.Fatalf("got %+v, want [example.com/b]", got)
		}
	})

	t.Run("skips packages the run loaded", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("r")
		prev.Add(entry("a", "a.go", "example.com/a", "p"))
		got := pruneCandidates(prev, map[string]struct{}{"example.com/a": {}}, "p")
		if len(got) != 0 {
			t.Fatalf("a loaded package needs no probe; got %+v", got)
		}
	})

	t.Run("skips other pipelines", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("r")
		prev.Add(entry("b", "a.go", "example.com/b", "other"))
		if got := pruneCandidates(prev, map[string]struct{}{}, "p"); len(got) != 0 {
			t.Fatalf("got %+v, want none", got)
		}
	})

	t.Run("deduplicates paths shared by several outputs", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("r")
		prev.Add(entry("b", "one.go", "example.com/b", "p"))
		prev.Add(entry("b", "two.go", "example.com/b", "p"))
		if got := pruneCandidates(prev, map[string]struct{}{}, "p"); len(got) != 1 {
			t.Fatalf("one stat per package, not per output; got %+v", got)
		}
	})

	t.Run("probes the real package behind a _test shift", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("r")
		prev.Add(entry("b", "a_test.go", "example.com/b_test", "p"))
		got := pruneCandidates(prev, map[string]struct{}{}, "p")
		if len(got) != 1 || got[0] != "example.com/b" {
			t.Fatalf("the _test path never had a directory; got %+v", got)
		}
	})

	t.Run("skips entries carrying no import path", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("r")
		prev.Add(entry("b", "a.go", "", "p"))
		if got := pruneCandidates(prev, map[string]struct{}{}, "p"); len(got) != 0 {
			t.Fatalf("an unrouted entry cannot be resolved; got %+v", got)
		}
	})

	t.Run("nil manifest yields nothing", func(t *testing.T) {
		t.Parallel()
		if got := pruneCandidates(nil, map[string]struct{}{}, "p"); got != nil {
			t.Fatalf("got %+v, want nil", got)
		}
	})

	t.Run("empty pipeline id yields nothing", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("r")
		prev.Add(entry("b", "a.go", "example.com/b", "p"))
		if got := pruneCandidates(prev, map[string]struct{}{}, ""); got != nil {
			t.Fatalf("got %+v, want nil", got)
		}
	})
}
