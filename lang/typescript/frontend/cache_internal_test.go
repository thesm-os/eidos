// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"os"
	"path/filepath"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/node"
)

func TestPackageCacheKey(t *testing.T) {
	t.Parallel()

	write := func(t *testing.T, body string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "a.ts")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		return path
	}

	t.Run("the same inputs produce the same key", func(t *testing.T) {
		t.Parallel()
		path := write(t, "export interface A {}")
		opts := defaultOptions()

		first, err := packageCacheKey("dir", []string{path}, opts, "fp")
		if err != nil {
			t.Fatalf("packageCacheKey: %v", err)
		}
		second, _ := packageCacheKey("dir", []string{path}, opts, "fp")
		if first != second {
			t.Fatal("the same inputs produced different keys")
		}
	})

	t.Run("changed content changes the key", func(t *testing.T) {
		t.Parallel()
		a := write(t, "export interface A {}")
		b := write(t, "export interface B {}")

		ka, _ := packageCacheKey("dir", []string{a}, defaultOptions(), "fp")
		kb, _ := packageCacheKey("dir", []string{b}, defaultOptions(), "fp")
		if ka == kb {
			t.Fatal("different content produced the same key")
		}
	})

	t.Run("a changed fingerprint changes the key", func(t *testing.T) {
		t.Parallel()
		// Not optional: the cached graph carries the metadata
		// downstream plugins read, and an upgraded plugin expecting a
		// stamp an older frontend never wrote is the stale-cache
		// failure this closes.
		path := write(t, "export interface A {}")
		ka, _ := packageCacheKey("dir", []string{path}, defaultOptions(), "one")
		kb, _ := packageCacheKey("dir", []string{path}, defaultOptions(), "two")
		if ka == kb {
			t.Fatal("the fingerprint does not reach the key")
		}
	})

	t.Run("changed options change the key", func(t *testing.T) {
		t.Parallel()
		path := write(t, "export interface A {}")
		other := defaultOptions()
		other.IncludeTests = true

		ka, _ := packageCacheKey("dir", []string{path}, defaultOptions(), "fp")
		kb, _ := packageCacheKey("dir", []string{path}, other, "fp")
		if ka == kb {
			t.Fatal("options do not reach the key")
		}
	})

	t.Run("a file that vanished is reported rather than hashed as empty", func(t *testing.T) {
		t.Parallel()
		// Hashing a missing file as nothing would key a real package
		// on an input it does not have.
		_, err := packageCacheKey("dir", []string{"/nonexistent/a.ts"}, defaultOptions(), "fp")
		if err == nil {
			t.Fatal("a missing input produced a key")
		}
	})

	t.Run("the key does not depend on the order files were walked", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		names := []string{"a.ts", "b.ts"}
		paths := make([]string, 0, len(names))
		for _, n := range names {
			p := filepath.Join(dir, n)
			if err := os.WriteFile(p, []byte("export interface X {}"), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			paths = append(paths, p)
		}
		forward, _ := packageCacheKey("d", paths, defaultOptions(), "fp")
		reverse, _ := packageCacheKey("d", []string{paths[1], paths[0]}, defaultOptions(), "fp")
		if forward != reverse {
			t.Fatal("walk order reached the key")
		}
	})
}

func TestCacheRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("a stored package comes back with owners rebuilt", func(t *testing.T) {
		t.Parallel()
		c := cache.NewDisk(t.TempDir())
		pkg := &node.Package{
			Name: "p", Path: "./p",
			Interfaces: []*node.Interface{{
				Name:   "A",
				Fields: []*node.Field{{Name: "id"}},
			}},
		}
		storePackageInCache(c, "k", pkg)

		got, ok := loadPackageFromCache(c, "k")
		if !ok {
			t.Fatal("a stored package did not come back")
		}
		if got.Interfaces[0].Fields[0].Owner == nil {
			t.Error("field Owner not rebuilt on read")
		}
	})

	t.Run("a miss reports absence", func(t *testing.T) {
		t.Parallel()
		if _, ok := loadPackageFromCache(cache.NewDisk(t.TempDir()), "absent"); ok {
			t.Fatal("a missing key reported a hit")
		}
	})

	t.Run("a nil cache or empty key is a miss, not a panic", func(t *testing.T) {
		t.Parallel()
		if _, ok := loadPackageFromCache(nil, "k"); ok {
			t.Error("a nil cache reported a hit")
		}
		if _, ok := loadPackageFromCache(cache.NewDisk(t.TempDir()), ""); ok {
			t.Error("an empty key reported a hit")
		}
		// Writes through the same guards must not panic either.
		storePackageInCache(nil, "k", &node.Package{})
		storePackageInCache(cache.NewDisk(t.TempDir()), "", &node.Package{})
	})

	t.Run("a corrupt entry is a miss rather than a failure", func(t *testing.T) {
		t.Parallel()
		// The fallback is to convert the source again, which is always
		// correct. A cache that could block a run would be worse than
		// no cache.
		c := cache.NewDisk(t.TempDir())
		if err := c.Put("k", []byte("{not json")); err != nil {
			t.Fatalf("Put: %v", err)
		}
		if _, ok := loadPackageFromCache(c, "k"); ok {
			t.Fatal("a corrupt entry reported a hit")
		}
	})
}
