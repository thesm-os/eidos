// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// tree writes files into a fresh directory and returns its path.
func tree(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, n := range names {
		path := filepath.Join(root, n)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte("export interface A {}\n"), 0o600); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}
	return root
}

// basenames reduces resolved paths to their file names, sorted.
func basenames(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, filepath.Base(p))
	}
	slices.Sort(out)
	return out
}

func TestExpandPattern(t *testing.T) {
	t.Parallel()

	t.Run("a recursive pattern descends", func(t *testing.T) {
		t.Parallel()
		root := tree(t, "a.ts", "deep/b.ts", "deep/deeper/c.ts")
		got, err := expandPattern("./...", Options{Dir: root})
		if err != nil {
			t.Fatalf("expandPattern: %v", err)
		}
		if !slices.Equal(basenames(got), []string{"a.ts", "b.ts", "c.ts"}) {
			t.Fatalf("resolved = %v, want every file beneath the root", basenames(got))
		}
	})

	t.Run("a directory pattern does not descend", func(t *testing.T) {
		t.Parallel()
		root := tree(t, "a.ts", "deep/b.ts")
		got, err := expandPattern("./", Options{Dir: root})
		if err != nil {
			t.Fatalf("expandPattern: %v", err)
		}
		if !slices.Equal(basenames(got), []string{"a.ts"}) {
			t.Fatalf("resolved = %v, want only the top level", basenames(got))
		}
	})

	t.Run("a single file resolves to itself", func(t *testing.T) {
		t.Parallel()
		root := tree(t, "a.ts", "b.ts")
		got, err := expandPattern("a.ts", Options{Dir: root})
		if err != nil {
			t.Fatalf("expandPattern: %v", err)
		}
		if !slices.Equal(basenames(got), []string{"a.ts"}) {
			t.Fatalf("resolved = %v, want a.ts alone", basenames(got))
		}
	})

	t.Run("a glob expands through the filesystem", func(t *testing.T) {
		t.Parallel()
		root := tree(t, "a.ts", "b.ts", "deep/c.ts")
		got, err := expandPattern("*.ts", Options{Dir: root})
		if err != nil {
			t.Fatalf("expandPattern: %v", err)
		}
		if !slices.Equal(basenames(got), []string{"a.ts", "b.ts"}) {
			t.Fatalf("resolved = %v, want the top-level matches", basenames(got))
		}
	})

	t.Run("an absolute pattern is used as given", func(t *testing.T) {
		t.Parallel()
		// A caller that already resolved the path should not have it
		// joined onto the configured directory a second time.
		root := tree(t, "a.ts")
		got, err := expandPattern(filepath.Join(root, "a.ts"), Options{Dir: "/elsewhere"})
		if err != nil {
			t.Fatalf("expandPattern: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("resolved = %v, want the absolute file", got)
		}
	})

	t.Run("an empty pattern is rejected", func(t *testing.T) {
		t.Parallel()
		for _, p := range []string{"", "   "} {
			if _, err := expandPattern(p, Options{Dir: t.TempDir()}); !errors.Is(err, ErrEmptyPattern) {
				t.Errorf("expandPattern(%q) = %v, want ErrEmptyPattern", p, err)
			}
		}
	})

	t.Run("a pattern matching nothing reports it", func(t *testing.T) {
		t.Parallel()
		// A typo and an empty directory report the same way, which is
		// what separates "nothing to do" from a mistake in the
		// invocation.
		root := tree(t, "a.ts")
		for _, p := range []string{"./absent/...", "*.rs", "nope.ts"} {
			if _, err := expandPattern(p, Options{Dir: root}); !errors.Is(err, ErrNoMatch) {
				t.Errorf("expandPattern(%q) = %v, want ErrNoMatch", p, err)
			}
		}
	})

	t.Run("a directory holding only excluded files matches nothing", func(t *testing.T) {
		t.Parallel()
		root := tree(t, "a.test.ts", "b.js")
		if _, err := expandPattern("./...", Options{Dir: root}); !errors.Is(err, ErrNoMatch) {
			t.Fatalf("expandPattern = %v, want ErrNoMatch", err)
		}
	})

	t.Run("node_modules is pruned during the walk", func(t *testing.T) {
		t.Parallel()
		root := tree(t, "a.ts", "node_modules/pkg/i.ts")
		got, err := expandPattern("./...", Options{Dir: root, SkipNodeModules: true})
		if err != nil {
			t.Fatalf("expandPattern: %v", err)
		}
		if !slices.Equal(basenames(got), []string{"a.ts"}) {
			t.Fatalf("resolved = %v, want node_modules pruned", basenames(got))
		}
	})

	t.Run("results are sorted and deduplicated", func(t *testing.T) {
		t.Parallel()
		// A run's order must not depend on the filesystem's.
		root := tree(t, "c.ts", "a.ts", "b.ts")
		got, err := expandPattern("./...", Options{Dir: root})
		if err != nil {
			t.Fatalf("expandPattern: %v", err)
		}
		if !slices.IsSorted(got) {
			t.Fatalf("resolved = %v, want sorted", got)
		}
	})
}

func TestReadSource(t *testing.T) {
	t.Parallel()

	t.Run("reads a file's bytes", func(t *testing.T) {
		t.Parallel()
		root := tree(t, "a.ts")
		got, err := readSource(filepath.Join(root, "a.ts"))
		if err != nil {
			t.Fatalf("readSource: %v", err)
		}
		if len(got) == 0 {
			t.Fatal("readSource returned no bytes")
		}
	})

	t.Run("a missing file is an error naming the path", func(t *testing.T) {
		t.Parallel()
		_, err := readSource("/nonexistent/a.ts")
		if err == nil {
			t.Fatal("reading a missing file succeeded")
		}
	})
}
