// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"os"
	"path/filepath"
	"testing"
)

// What is left here is what is left of caching in this package.
//
// Composing the key, looking it up, re-wiring an entry and writing one
// back moved to plugin.CacheLoad, and so did the tests for them: the
// composition fingerprint reaching the key, a corrupt entry reading as
// a miss, owner pointers rebuilt after a round trip. Every frontend
// inherits those rather than each restating them.
//
// The input hash stays, because which bytes are a directory's inputs
// is the one question the framework cannot answer for TypeScript.

// TestHashInputs covers the half of caching this frontend still owns.
func TestHashInputs(t *testing.T) {
	t.Parallel()

	write := func(t *testing.T, body string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "a.ts")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		return path
	}

	t.Run("the same inputs hash alike", func(t *testing.T) {
		t.Parallel()
		path := write(t, "export interface A {}")
		first, err := hashInputs([]string{path}, defaultOptions())
		if err != nil {
			t.Fatalf("hashInputs: %v", err)
		}
		second, _ := hashInputs([]string{path}, defaultOptions())
		if first != second {
			t.Error("the same inputs hashed differently, so nothing would ever hit")
		}
	})

	t.Run("changed content changes the hash", func(t *testing.T) {
		t.Parallel()
		a := write(t, "export interface A {}")
		b := write(t, "export interface B {}")
		ha, _ := hashInputs([]string{a}, defaultOptions())
		hb, _ := hashInputs([]string{b}, defaultOptions())
		if ha == hb {
			t.Error("edited source hashed alike, so a run would serve the old graph")
		}
	})

	t.Run("changed options change the hash", func(t *testing.T) {
		t.Parallel()
		path := write(t, "export interface A {}")
		base := defaultOptions()
		other := defaultOptions()
		other.IncludeDeclarations = !base.IncludeDeclarations

		hb, _ := hashInputs([]string{path}, base)
		ho, _ := hashInputs([]string{path}, other)
		if hb == ho {
			t.Error("options that change what is converted must change the hash")
		}
	})

	t.Run("the hash does not depend on walk order", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		names := []string{"a.ts", "b.ts"}
		paths := make([]string, 0, len(names))
		for _, name := range names {
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte("export interface X {}"), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			paths = append(paths, path)
		}
		forward, _ := hashInputs(paths, defaultOptions())
		backward, _ := hashInputs([]string{paths[1], paths[0]}, defaultOptions())
		if forward != backward {
			t.Error("the walk's reporting order reached the hash, so the same " +
				"directory would miss depending on how it was listed")
		}
	})

	t.Run("a file that vanished is reported rather than hashed as empty", func(t *testing.T) {
		t.Parallel()
		// Silently hashing an unreadable file as empty is the failure
		// worth naming: two directories whose files all vanished would
		// hash alike and serve each other's graphs.
		if _, err := hashInputs(
			[]string{filepath.Join(t.TempDir(), "gone.ts")}, defaultOptions(),
		); err == nil {
			t.Error("a missing file produced a hash instead of an error")
		}
	})
}
