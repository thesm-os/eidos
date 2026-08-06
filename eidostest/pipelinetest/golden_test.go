// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipelinetest_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/eidostest/pipelinetest"
)

// withUpdateGolden flips the package-level -update-golden flag for
// the duration of fn and restores it afterwards. Subtests run
// sequentially in this file (no t.Parallel) because the flag is
// process-global.
func withUpdateGolden(t *testing.T, on bool, fn func()) {
	t.Helper()
	f := flag.Lookup("update-golden")
	if f == nil {
		t.Fatalf("update-golden flag should be registered")
	}
	prev := f.Value.String()
	if err := f.Value.Set(boolString(on)); err != nil {
		t.Fatalf("setting update-golden=%v: %v", on, err)
	}
	defer func() {
		if err := f.Value.Set(prev); err != nil {
			t.Fatalf("restoring update-golden: %v", err)
		}
	}()
	fn()
}

// boolString returns the canonical flag string for a bool.
func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// withArgs swaps os.Args for the duration of fn and restores it
// afterwards. os.Args is process-global, which is why every test in
// this file forgoes t.Parallel: Go releases parallel tests only once
// the sequential ones have finished, so the swap cannot be observed
// by a pipeline running in another subtest.
func withArgs(tb testing.TB, args []string, fn func()) {
	tb.Helper()
	prev := os.Args
	os.Args = args
	defer func() { os.Args = prev }()
	fn()
}

// renderCommand runs one synthetic pipeline under args and returns
// the bytes its backend captured for the "Command:" header field.
func renderCommand(tb testing.TB, args []string) []byte {
	tb.Helper()
	var body []byte
	withArgs(tb, args, func() {
		body = headerPipeline(tb).AssertFile("command.txt").Bytes()
	})
	return body
}

// readBytes is a test-only helper that reads a file and fails the
// test on error.
func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return b
}

func TestUpdateGolden(t *testing.T) {
	t.Run("reflects the registered flag value", func(t *testing.T) {
		withUpdateGolden(t, true, func() {
			if !pipelinetest.UpdateGolden() {
				t.Fatalf("UpdateGolden should reflect the flag value")
			}
		})
		withUpdateGolden(t, false, func() {
			if pipelinetest.UpdateGolden() {
				t.Fatalf("UpdateGolden should be false after restore")
			}
		})
	})
}

func TestFileAssertion_MatchesGolden(t *testing.T) {
	t.Run("succeeds when file content matches the golden", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "golden.txt")
		if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
			t.Fatalf("seeding golden: %v", err)
		}
		fileAssertion(t, "hello").MatchesGolden(path)
	})

	t.Run("Errorf when file content does not match the golden", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "golden.txt")
		if err := os.WriteFile(path, []byte("expected"), 0o600); err != nil {
			t.Fatalf("seeding golden: %v", err)
		}
		fake := newFakeT()
		fileAssertion(fake, "actual").MatchesGolden(path)
		if !fake.Failed() {
			t.Fatalf("expected Errorf on mismatched golden")
		}
		joined := strings.Join(fake.errs, "\n")
		if !strings.Contains(joined, "does not match golden") {
			t.Fatalf("error should mention golden mismatch; got %q", joined)
		}
	})

	t.Run("Errorf with -update-golden hint when golden is missing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.txt")
		fake := newFakeT()
		fileAssertion(fake, "hello").MatchesGolden(path)
		if !fake.Failed() {
			t.Fatalf("expected Errorf when golden is missing")
		}
		joined := strings.Join(fake.errs, "\n")
		if !strings.Contains(joined, "update-golden") {
			t.Fatalf("error should mention -update-golden; got %q", joined)
		}
	})

	t.Run("under -update-golden writes the file atomically", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nested", "golden.txt")
		withUpdateGolden(t, true, func() {
			fileAssertion(t, "fresh-content").MatchesGolden(path)
		})
		got := readBytes(t, path)
		if string(got) != "fresh-content" {
			t.Fatalf("update-golden wrote %q, want %q", got, "fresh-content")
		}
	})

	t.Run("under -update-golden overwrites an existing golden", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "golden.txt")
		if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
			t.Fatalf("seeding golden: %v", err)
		}
		withUpdateGolden(t, true, func() {
			fileAssertion(t, "updated").MatchesGolden(path)
		})
		got := readBytes(t, path)
		if string(got) != "updated" {
			t.Fatalf("update-golden left %q, want %q", got, "updated")
		}
	})

	t.Run("returns the assertion for chaining", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "golden.txt")
		if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
			t.Fatalf("seeding golden: %v", err)
		}
		a := fileAssertion(t, "ok")
		if a.MatchesGolden(path) != a {
			t.Fatalf("MatchesGolden should return its receiver for chaining")
		}
	})

	t.Run("Fatalf on read error other than missing file", func(t *testing.T) {
		// Drop a directory at the golden path so os.ReadFile returns
		// EISDIR — surfaces the non-missing read-error branch.
		dir := filepath.Join(t.TempDir(), "blocked")
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		fake := newFakeT()
		captureFatal(func() {
			fileAssertion(fake, "x").MatchesGolden(dir)
		})
		if !fake.Failed() {
			t.Fatalf("expected Fatalf on read error")
		}
		joined := strings.Join(fake.fatals, "\n")
		if !strings.Contains(joined, "failed to read golden") {
			t.Fatalf("fatal should mention read failure; got %q", joined)
		}
	})

	t.Run("Fatalf on write error in update mode", func(t *testing.T) {
		// Make the target directory read-only so writing the temp
		// file fails — surfaces the write-error branch of writeGolden.
		readOnly := filepath.Join(t.TempDir(), "ro")
		if err := os.Mkdir(readOnly, 0o500); err != nil {
			t.Fatalf("mkdir read-only: %v", err)
		}
		// Restore traversal permission so t.TempDir's cleanup can
		// remove the directory. Directory perms need the execute bit.
		t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) }) //nolint:gosec
		path := filepath.Join(readOnly, "golden.txt")

		fake := newFakeT()
		withUpdateGolden(t, true, func() {
			captureFatal(func() {
				fileAssertion(fake, "x").MatchesGolden(path)
			})
		})
		if !fake.Failed() {
			t.Fatalf("expected Fatalf on write error")
		}
		joined := strings.Join(fake.fatals, "\n")
		if !strings.Contains(joined, "failed to rewrite golden") {
			t.Fatalf("fatal should mention rewrite failure; got %q", joined)
		}
	})

	t.Run("Fatalf when parent path crosses a non-directory component", func(t *testing.T) {
		// Place a regular file where a parent directory of the
		// target is expected; MkdirAll fails because it cannot
		// create a subdirectory under a file.
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte{}, 0o600); err != nil {
			t.Fatalf("seeding blocker: %v", err)
		}
		path := filepath.Join(blocker, "sub", "golden.txt")

		fake := newFakeT()
		withUpdateGolden(t, true, func() {
			captureFatal(func() {
				fileAssertion(fake, "x").MatchesGolden(path)
			})
		})
		if !fake.Failed() {
			t.Fatalf("expected Fatalf when parent path is not a directory")
		}
	})

	t.Run("the same pipeline rendered under two different os.Args produces identical bytes", func(t *testing.T) {
		first := renderCommand(t, []string{"pipelinetest.test", "-test.run=TestFirst", "-test.paniconexit0"})
		second := renderCommand(t, []string{"pipelinetest.test", "-test.run=TestSecond", "-update-golden"})
		if !bytes.Equal(first, second) {
			t.Fatalf("rendered bytes vary with os.Args:\n first = %q\nsecond = %q", first, second)
		}
	})

	t.Run("the documented golden round-trip passes on its second run", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "command.golden")
		withUpdateGolden(t, true, func() {
			withArgs(t, []string{"pipelinetest.test", "-test.run=TestRoundTrip", "-update-golden"}, func() {
				headerPipeline(t).AssertFile("command.txt").MatchesGolden(path)
			})
		})

		fake := newFakeT()
		withArgs(t, []string{"pipelinetest.test", "-test.run=TestRoundTrip", "-test.count=1"}, func() {
			headerPipeline(fake).AssertFile("command.txt").MatchesGolden(path)
		})
		if fake.Failed() {
			t.Fatalf("a golden written under -update-golden must re-assert clean: %v", fake.errs)
		}
	})

	t.Run("Fatalf when the target itself is an existing directory in update mode", func(t *testing.T) {
		// Pre-create a directory at the golden path so Rename of the
		// temp file fails — surfaces the rename-error branch.
		dir := filepath.Join(t.TempDir(), "golden-is-dir")
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		fake := newFakeT()
		withUpdateGolden(t, true, func() {
			captureFatal(func() {
				fileAssertion(fake, "x").MatchesGolden(dir)
			})
		})
		if !fake.Failed() {
			t.Fatalf("expected Fatalf when target is a directory")
		}
	})
}
