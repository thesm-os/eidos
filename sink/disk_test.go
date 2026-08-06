// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sink_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/sink"
)

func TestNewDisk(t *testing.T) {
	t.Parallel()

	t.Run("captures the supplied root for later writes", func(t *testing.T) {
		t.Parallel()
		d := sink.NewDisk("/tmp/eidos-test")
		if d.Root() != "/tmp/eidos-test" {
			t.Fatalf("Root = %q, want /tmp/eidos-test", d.Root())
		}
	})
}

func TestDisk_Write(t *testing.T) {
	t.Parallel()

	t.Run("creates the destination file under root with the body", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		d := sink.NewDisk(root)
		assertNoError(t, d.Write(targetAt("a", "b.go"), []byte("hello")))
		got, err := os.ReadFile(filepath.Join(root, "a", "b.go"))
		assertNoError(t, err)
		if string(got) != "hello" {
			t.Fatalf("file content = %q, want hello", got)
		}
	})

	t.Run("creates intermediate directories under root", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		d := sink.NewDisk(root)
		assertNoError(t, d.Write(targetAt("nested/deeper", "x.go"), []byte("body")))
		if _, err := os.Stat(filepath.Join(root, "nested", "deeper", "x.go")); err != nil {
			t.Fatalf("expected nested file to exist; got %v", err)
		}
	})

	t.Run("overwrites an existing file atomically", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		d := sink.NewDisk(root)
		assertNoError(t, d.Write(targetAt("a", "b.go"), []byte("first")))
		assertNoError(t, d.Write(targetAt("a", "b.go"), []byte("second")))
		got, err := os.ReadFile(filepath.Join(root, "a", "b.go"))
		assertNoError(t, err)
		if string(got) != "second" {
			t.Fatalf("overwrite mismatch: %q", got)
		}
	})

	t.Run("skips the write when the destination already has identical bytes", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		d := sink.NewDisk(root)
		assertNoError(t, d.Write(targetAt("a", "b.go"), []byte("body")))
		path := filepath.Join(root, "a", "b.go")
		first, err := os.Stat(path)
		assertNoError(t, err)
		// Sleep through filesystem mtime granularity so any rewrite
		// would surface a fresh ModTime. macOS HFS+ resolves seconds;
		// APFS resolves nanoseconds but still benefits from the gap.
		time.Sleep(1100 * time.Millisecond)
		assertNoError(t, d.Write(targetAt("a", "b.go"), []byte("body")))
		second, err := os.Stat(path)
		assertNoError(t, err)
		if !first.ModTime().Equal(second.ModTime()) {
			t.Fatalf(
				"identical body rewrite should not touch mtime; first=%v second=%v",
				first.ModTime(), second.ModTime(),
			)
		}
	})

	t.Run("skips the write when only the header differs but the provenance hash matches", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		d := sink.NewDisk(root)
		// First write: a generated-style file with a Command line
		// matching the user's first invocation.
		bodyA := generatedBlob("eidos run ./blog/...", "package x\n", "deadbeef")
		assertNoError(t, d.Write(targetAt("a", "b.go"), bodyA))
		path := filepath.Join(root, "a", "b.go")
		first, err := os.Stat(path)
		assertNoError(t, err)
		// Second write: same body, different Command line (mirrors
		// a follow-up `eidos run ./...` invocation). The provenance
		// hash is identical so the sink must short-circuit.
		bodyB := generatedBlob("eidos run ./...", "package x\n", "deadbeef")
		time.Sleep(1100 * time.Millisecond)
		assertNoError(t, d.Write(targetAt("a", "b.go"), bodyB))
		second, err := os.Stat(path)
		assertNoError(t, err)
		if !first.ModTime().Equal(second.ModTime()) {
			t.Fatalf(
				"header-only delta with matching provenance hash must not rewrite; first=%v second=%v",
				first.ModTime(), second.ModTime(),
			)
		}
	})

	t.Run("rewrites when the on-disk file has been tampered past the provenance marker", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		d := sink.NewDisk(root)
		// First write: a normal generated file.
		body := generatedBlob("eidos run", "package x\n", "deadbeef")
		assertNoError(t, d.Write(targetAt("a", "b.go"), body))
		path := filepath.Join(root, "a", "b.go")
		// User tampers: appends a manual line past the end-of-generated
		// marker. The provenance line is still inside the file —
		// LastIndex would find it — but the file is no longer
		// terminated by the trailer.
		tampered := append([]byte(nil), body...)
		tampered = append(tampered, []byte("// MANUAL EDIT\n")...)
		assertNoError(t, os.WriteFile(path, tampered, 0o600))
		// Second write of identical body must rewrite the file
		// (eliminating the tampered tail) — the hash-match
		// short-circuit must not apply when the on-disk file has
		// content past the provenance trailer.
		assertNoError(t, d.Write(targetAt("a", "b.go"), body))
		got, err := os.ReadFile(path)
		assertNoError(t, err)
		if !bytes.Equal(got, body) {
			t.Fatalf(
				"tampered file must be rewritten; got %q, want %q",
				got, body,
			)
		}
	})

	t.Run("rewrites when the incoming body carries no provenance trailer", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		d := sink.NewDisk(root)
		// First write: a normal generated file, trailer intact and
		// at the tail, so the on-disk side of the comparison offers
		// a hash.
		assertNoError(t, d.Write(targetAt("a", "b.go"), generatedBlob("eidos run", "package x\n", "deadbeef")))
		path := filepath.Join(root, "a", "b.go")
		// Second write: an untrailered body. This is the shape a
		// plugin produces when it writes through the sink without
		// going via the Go backend's envelope. With no hash to
		// compare against, the sink has no evidence the contents
		// agree and must not skip.
		plain := []byte("package x\n\n// hand-rolled, no envelope\n")
		assertNoError(t, d.Write(targetAt("a", "b.go"), plain))
		got, err := os.ReadFile(path)
		assertNoError(t, err)
		if !bytes.Equal(got, plain) {
			t.Fatalf(
				"an untrailered body must overwrite a trailered file; got %q, want %q",
				got, plain,
			)
		}
	})

	t.Run("does not leave temporary files behind on success", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		d := sink.NewDisk(root)
		assertNoError(t, d.Write(targetAt("a", "b.go"), []byte("body")))
		entries, err := os.ReadDir(filepath.Join(root, "a"))
		assertNoError(t, err)
		if len(entries) != 1 || entries[0].Name() != "b.go" {
			t.Fatalf("unexpected dir contents: %+v", entries)
		}
	})

	t.Run("rejects empty Filename with ErrInvalidTarget", func(t *testing.T) {
		t.Parallel()
		d := sink.NewDisk(t.TempDir())
		err := d.Write(targetAt("a", ""), nil)
		if !errors.Is(err, sink.ErrInvalidTarget) {
			t.Fatalf("Write should return ErrInvalidTarget; got %v", err)
		}
	})

	t.Run("returns an error when MkdirAll fails", func(t *testing.T) {
		t.Parallel()
		// Create a regular file at a path that should be a directory.
		root := t.TempDir()
		conflictPath := filepath.Join(root, "a")
		assertNoError(t, os.WriteFile(conflictPath, nil, 0o600))
		d := sink.NewDisk(root)
		err := d.Write(targetAt("a/b", "x.go"), []byte("body"))
		if err == nil {
			t.Fatalf("Write should fail when MkdirAll cannot create the destination")
		}
	})

	t.Run("returns an error when WriteFile fails on a read-only directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		ro := filepath.Join(root, "a")
		assertNoError(t, os.MkdirAll(ro, 0o750))
		assertNoError(t, os.Chmod(ro, 0o500))         //nolint:gosec // intentional read-only for the test
		t.Cleanup(func() { _ = os.Chmod(ro, 0o750) }) //nolint:gosec // restore for TempDir cleanup
		d := sink.NewDisk(root)
		err := d.Write(targetAt("a", "x.go"), []byte("body"))
		if err == nil {
			t.Fatalf("Write should fail when WriteFile cannot create a file in a read-only dir")
		}
	})

	t.Run("returns an error when Rename targets an existing directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		// Pre-create x.go as a directory at the destination so Rename
		// can't replace it with a regular file.
		assertNoError(t, os.MkdirAll(filepath.Join(root, "a", "x.go"), 0o750))
		d := sink.NewDisk(root)
		err := d.Write(targetAt("a", "x.go"), []byte("body"))
		if err == nil {
			t.Fatalf("Write should fail when Rename collides with an existing directory")
		}
	})

	t.Run("absolute target.Dir bypasses root", func(t *testing.T) {
		t.Parallel()
		// A root-anchored target picks up the disk root; an
		// absolute-dir target writes directly to that path without
		// re-rooting. The router resolves Dir from a source file's
		// absolute path in alongside-source layouts and the sink
		// must honour the path as-is.
		root := t.TempDir()
		dest := t.TempDir()
		d := sink.NewDisk(root)
		assertNoError(t, d.Write(emit.Target{Dir: dest, Filename: "abs.go"}, []byte("absolute")))
		got, err := os.ReadFile(filepath.Join(dest, "abs.go"))
		assertNoError(t, err)
		if string(got) != "absolute" {
			t.Fatalf("absolute-dir write mismatch: %q", got)
		}
		// And the root tree stays empty — no `dest`-shaped subtree leaks in.
		entries, err := os.ReadDir(root)
		assertNoError(t, err)
		if len(entries) != 0 {
			t.Fatalf("absolute-dir write must not touch root; got %d entries", len(entries))
		}
	})
}

func TestDisk_ConcurrentWrites(t *testing.T) {
	t.Parallel()

	t.Run("Write serialises under -race", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		d := sink.NewDisk(root)
		var wg sync.WaitGroup
		for i := range 16 {
			wg.Go(func() {
				_ = d.Write(targetAt("d", string(rune('a'+i%26))+".go"), []byte("body"))
			})
		}
		wg.Wait()
	})
}
