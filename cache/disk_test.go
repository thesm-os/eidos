// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cache_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"go.thesmos.sh/eidos/cache"
)

// nameFor mirrors Disk's key-to-filename derivation so tests can
// locate an entry on disk without the unexported keyPath. It is
// deliberately a re-derivation rather than a copy of the constant:
// if the layout changes, these tests follow it.
func nameFor(key string) string { return cache.HashBytes([]byte(key)) }

// bucketFor returns the shard directory nameFor(key) lands in.
func bucketFor(key string) string { return nameFor(key)[:2] }

func TestNewDisk(t *testing.T) {
	t.Parallel()

	t.Run("captures the supplied root", func(t *testing.T) {
		t.Parallel()
		d := cache.NewDisk("/tmp/eidos-cache")
		if d.Root() != "/tmp/eidos-cache" {
			t.Fatalf("Root = %q, want /tmp/eidos-cache", d.Root())
		}
	})
}

func TestDisk_PutAndGet(t *testing.T) {
	t.Parallel()

	t.Run("Put-then-Get round-trips bytes", func(t *testing.T) {
		t.Parallel()
		d := cache.NewDisk(t.TempDir())
		key := "abcdef0123456789"
		assertNoError(t, d.Put(key, []byte("hello")))
		got, ok := d.Get(key)
		if !ok || string(got) != "hello" {
			t.Fatalf("round-trip mismatch: %q ok=%v", got, ok)
		}
	})

	t.Run("Put with a short key round-trips", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		d := cache.NewDisk(root)
		assertNoError(t, d.Put("a", []byte("body")))
		// Key length no longer affects the layout: the path derives
		// from the key's digest, which is always 64 hex characters.
		got, ok := d.Get("a")
		if !ok || string(got) != "body" {
			t.Fatalf("short-key round-trip mismatch: %q ok=%v", got, ok)
		}
	})

	t.Run("Put with the same key replaces the prior body", func(t *testing.T) {
		t.Parallel()
		d := cache.NewDisk(t.TempDir())
		key := "abcdef0123"
		assertNoError(t, d.Put(key, []byte("first")))
		assertNoError(t, d.Put(key, []byte("second")))
		got, _ := d.Get(key)
		if string(got) != "second" {
			t.Fatalf("Put should replace; got %q", got)
		}
	})
}

func TestDisk_PathPortability(t *testing.T) {
	t.Parallel()

	// CI runs Linux only, so nothing else in this suite would notice
	// a layout change that is illegal on Windows. The keys below are
	// the shape NewKey actually produces: a literal "plugin" prefix
	// (which previously collapsed every entry into one "pl" bucket),
	// ":" separators (not a legal NTFS filename character), and a
	// length that pushes a project-rooted path toward MAX_PATH.
	keys := []string{
		"a",
		cache.NewKey("plugin", "golang", "version", "1.0.0", "inputs", cache.HashBytes([]byte("x"))),
		cache.NewKey("plugin", "backend.golang", "reads", cache.HashBytes([]byte("y")),
			"routing", cache.HashBytes([]byte("z")), "scope", cache.HashBytes([]byte("w"))),
	}

	// Characters Windows rejects in a path component, plus the
	// separators that would smuggle in an extra directory level.
	const illegal = `:<>"|?*/\`

	t.Run("path components are hex and fixed-width", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		d := cache.NewDisk(root)
		for _, key := range keys {
			assertNoError(t, d.Put(key, []byte("body")))
		}

		// Inspect what actually landed on disk rather than
		// re-deriving it, so a regression in keyPath surfaces here
		// instead of being mirrored by the test's own arithmetic.
		entries := walkRelative(t, root)
		if len(entries) != len(keys) {
			t.Fatalf("wrote %d keys but found %d files: %v", len(keys), len(entries), entries)
		}
		for _, rel := range entries {
			segs := strings.Split(rel, string(filepath.Separator))
			if len(segs) != 2 {
				t.Fatalf("entry %q has %d path components, want <bucket>/<name>", rel, len(segs))
			}
			if len(segs[0]) != 2 {
				t.Fatalf("bucket in %q is %d chars, want 2", rel, len(segs[0]))
			}
			if len(segs[1]) != 64 {
				t.Fatalf("filename in %q is %d chars, want a fixed 64", rel, len(segs[1]))
			}
			for _, seg := range segs {
				if strings.ContainsAny(seg, illegal) {
					t.Fatalf("path component %q contains a character illegal on Windows", seg)
				}
				if strings.Trim(seg, "0123456789abcdef") != "" {
					t.Fatalf("path component %q is not lower-case hex", seg)
				}
			}
		}
	})

	t.Run("keys sharing a prefix still shard across buckets", func(t *testing.T) {
		t.Parallel()
		// Regression: every key NewKey builds starts with "plugin",
		// so bucketing on the key's own first two bytes produced a
		// single "pl" directory for the entire cache. Counted from
		// the directories Put actually created.
		root := t.TempDir()
		d := cache.NewDisk(root)
		const n = 64
		for i := range n {
			assertNoError(t, d.Put(cache.NewKey("plugin", "gen", "n", strconv.Itoa(i)), []byte("body")))
		}

		buckets := map[string]bool{}
		for _, rel := range walkRelative(t, root) {
			buckets[strings.Split(rel, string(filepath.Separator))[0]] = true
		}
		if len(buckets) < n/2 {
			t.Fatalf("%d common-prefix keys landed in %d buckets; sharding is not distributing", n, len(buckets))
		}
	})
}

// walkRelative returns every file under root, as a path relative to
// root. Directories are skipped; the caller asserts on the shape of
// the layout Disk actually produced.
func walkRelative(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		// WalkDir always descends from root, so trimming the prefix
		// is exact and avoids a second error path in the callback.
		out = append(out, strings.TrimPrefix(path, root+string(filepath.Separator)))
		return nil
	})
	assertNoError(t, err)
	return out
}

func TestDisk_Get(t *testing.T) {
	t.Parallel()

	t.Run("Get on missing key returns nil and false", func(t *testing.T) {
		t.Parallel()
		d := cache.NewDisk(t.TempDir())
		if got, ok := d.Get("missing"); ok || got != nil {
			t.Fatalf("Get on missing key = %q ok=%v", got, ok)
		}
	})

	t.Run("Get with empty key returns a miss", func(t *testing.T) {
		t.Parallel()
		d := cache.NewDisk(t.TempDir())
		if _, ok := d.Get(""); ok {
			t.Fatalf("Get with empty key should be a miss")
		}
	})

	t.Run("Get on an unreadable entry reports a miss", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		d := cache.NewDisk(root)
		key := "abcdef0123"
		assertNoError(t, d.Put(key, []byte("body")))
		// Make the per-key directory unreadable so the subsequent
		// Get hits the read error path rather than finding the file.
		bucket := filepath.Join(root, bucketFor(key))
		assertNoError(t, os.Chmod(bucket, 0o000))         //nolint:gosec // intentional unreadable
		t.Cleanup(func() { _ = os.Chmod(bucket, 0o750) }) //nolint:gosec // restore for TempDir cleanup
		if _, ok := d.Get(key); ok {
			t.Fatalf("Get on unreadable entry should report a miss")
		}
	})
}

func TestDisk_Put(t *testing.T) {
	t.Parallel()

	t.Run("rejects empty key with ErrInvalidKey", func(t *testing.T) {
		t.Parallel()
		d := cache.NewDisk(t.TempDir())
		err := d.Put("", []byte("body"))
		if !errors.Is(err, cache.ErrInvalidKey) {
			t.Fatalf("Put with empty key should return ErrInvalidKey; got %v", err)
		}
	})

	t.Run("returns an error when MkdirAll fails", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		// Pre-create a regular file at the path the Disk wants to
		// use as the bucket directory.
		conflictPath := filepath.Join(root, bucketFor("abcdef"))
		assertNoError(t, os.WriteFile(conflictPath, nil, 0o600))
		d := cache.NewDisk(root)
		if err := d.Put("abcdef", []byte("body")); err == nil {
			t.Fatalf("Put should fail when MkdirAll cannot create the bucket dir")
		}
	})

	t.Run("returns an error when WriteFile fails on a read-only bucket", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		bucket := filepath.Join(root, bucketFor("abcdef"))
		assertNoError(t, os.MkdirAll(bucket, 0o750))
		assertNoError(t, os.Chmod(bucket, 0o500))         //nolint:gosec // intentional read-only
		t.Cleanup(func() { _ = os.Chmod(bucket, 0o750) }) //nolint:gosec // restore for TempDir cleanup
		d := cache.NewDisk(root)
		if err := d.Put("abcdef", []byte("body")); err == nil {
			t.Fatalf("Put should fail when WriteFile cannot create a file in a read-only bucket")
		}
	})

	t.Run("returns an error when Rename targets an existing directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		// Pre-create the destination as a directory so Rename can't
		// replace it with a file.
		assertNoError(t, os.MkdirAll(filepath.Join(root, bucketFor("abcdef"), nameFor("abcdef")), 0o750))
		d := cache.NewDisk(root)
		if err := d.Put("abcdef", []byte("body")); err == nil {
			t.Fatalf("Put should fail when Rename collides with an existing directory")
		}
	})
}

func TestDisk_Has(t *testing.T) {
	t.Parallel()

	t.Run("reports true for previously-put keys", func(t *testing.T) {
		t.Parallel()
		d := cache.NewDisk(t.TempDir())
		assertNoError(t, d.Put("abcdef", []byte("body")))
		if !d.Has("abcdef") {
			t.Fatalf("Has should return true after Put")
		}
	})

	t.Run("reports false for missing keys", func(t *testing.T) {
		t.Parallel()
		d := cache.NewDisk(t.TempDir())
		if d.Has("nope") {
			t.Fatalf("Has on missing key should return false")
		}
	})

	t.Run("reports false for empty keys", func(t *testing.T) {
		t.Parallel()
		d := cache.NewDisk(t.TempDir())
		if d.Has("") {
			t.Fatalf("Has on empty key should return false")
		}
	})
}

func TestDisk_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	t.Run("Put and Get are safe under -race", func(t *testing.T) {
		t.Parallel()
		d := cache.NewDisk(t.TempDir())
		var wg sync.WaitGroup
		for i := range 16 {
			wg.Go(func() {
				key := "abc" + string(rune('a'+i%26))
				_ = d.Put(key, []byte("body"))
			})
		}
		for range 4 {
			wg.Go(func() {
				_, _ = d.Get("abca")
			})
		}
		wg.Wait()
	})
}

func TestDisk_SatisfiesCache(t *testing.T) {
	t.Parallel()

	t.Run("Disk satisfies the Cache interface", func(t *testing.T) {
		t.Parallel()
		var _ cache.Cache = cache.NewDisk("/tmp/eidos-test")
	})
}
