// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"errors"
	"sync"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/lang/golang/frontend"
	"go.thesmos.sh/eidos/node"
)

// recordingCache wraps an in-memory map cache so tests can count
// hits and misses. Tracks both Get calls and the values stored, but
// stays content-addressed: a put for the same key replaces the
// prior value (which is fine — same content produces same bytes).
type recordingCache struct {
	mu     sync.Mutex
	values map[string][]byte
	gets   int
	hits   int
	puts   int
}

func newRecordingCache() *recordingCache {
	return &recordingCache{values: map[string][]byte{}}
}

func (c *recordingCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gets++
	v, ok := c.values[key]
	if ok {
		c.hits++
	}
	return v, ok
}

func (c *recordingCache) Put(key string, value []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.puts++
	c.values[key] = value
	return nil
}

func (c *recordingCache) Stats() (gets, hits, puts int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gets, c.hits, c.puts
}

// TestCache_HitOnUnchangedInputs verifies re-running with unchanged
// source files skips parsing for those files. We materialise one
// source directory and run the frontend twice against it through
// the same recording cache; the second pass must register a hit.
func TestCache_HitOnUnchangedInputs(t *testing.T) {
	t.Parallel()
	t.Run("second load against the same cache produces a hit", func(t *testing.T) {
		t.Parallel()
		dir := materialiseGoSource(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ N int }\n",
		})
		c := newRecordingCache()

		loadDir(t, dir, c)
		_, _, puts := c.Stats()
		if puts != 1 {
			t.Fatalf("expected 1 cache write after miss, got %d", puts)
		}

		loadDir(t, dir, c)
		_, hits, _ := c.Stats()
		if hits == 0 {
			t.Fatalf("expected at least one cache hit on second load")
		}
	})

	t.Run("differing source bytes produce a fresh cache key", func(t *testing.T) {
		t.Parallel()
		c := newRecordingCache()
		dirA := materialiseGoSource(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ N int }\n",
		})
		dirB := materialiseGoSource(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ M int }\n",
		})
		loadDir(t, dirA, c)
		loadDir(t, dirB, c)
		_, hits, puts := c.Stats()
		if puts < 2 {
			t.Fatalf("expected at least 2 cache writes for distinct sources, got %d", puts)
		}
		if hits != 0 {
			t.Fatalf("differing bytes must not produce cache hits, got %d", hits)
		}
	})
}

// TestCache_NoneIsTransparent covers the documented "Cache may be
// no-op without affecting correctness" contract: a None cache
// returns nothing and produces an identical store.
func TestCache_NoneIsTransparent(t *testing.T) {
	t.Parallel()
	t.Run("None cache returns no hits but Load still succeeds", func(t *testing.T) {
		t.Parallel()
		dir := materialiseGoSource(t, map[string]string{
			"a.go": "package a\n\ntype S struct{}\n",
		})
		_, d := loadDir(t, dir, cache.NewNone())
		if d.HasErrors() {
			t.Fatalf("unexpected diagnostics: %+v", d.Diagnostics())
		}
	})

	t.Run("nil cache is treated like None", func(t *testing.T) {
		t.Parallel()
		dir := materialiseGoSource(t, map[string]string{"a.go": "package a\n"})
		_, d := loadDir(t, dir, nil)
		if d.HasErrors() {
			t.Fatalf("nil cache Load: %+v", d.Diagnostics())
		}
	})
}

// brokenCache returns hits with malformed bytes so we can verify
// the loader degrades gracefully instead of crashing.
type brokenCache struct{}

func (*brokenCache) Get(string) ([]byte, bool) { return []byte("not-json"), true }
func (*brokenCache) Put(string, []byte) error  { return nil }

// putFailingCache returns no hits but returns an error from every
// Put so the loader's put-error diagnostic branch is exercised.
type putFailingCache struct{}

func (*putFailingCache) Get(string) ([]byte, bool) { return nil, false }
func (*putFailingCache) Put(string, []byte) error {
	return errPutFailed
}

var errPutFailed = errors.New("cache.Put: simulated failure")

// TestCache_CorruptEntryFallsThroughToConversion verifies a corrupt
// cache entry behaves like a miss — the converter runs fresh and
// the resulting package is still populated.
func TestCache_CorruptEntryFallsThroughToConversion(t *testing.T) {
	t.Parallel()
	t.Run("corrupt cache entry behaves as a miss", func(t *testing.T) {
		t.Parallel()
		dir := materialiseGoSource(t, map[string]string{
			"a.go": "package a\n\ntype S struct{}\n",
		})
		s, _ := loadDir(t, dir, &brokenCache{})
		got := false
		s.Nodes().Packages().Range(func(*node.Package) bool {
			got = true
			return false
		})
		if !got {
			t.Fatalf("converter should have run on cache miss")
		}
	})
}

// TestCache_PutFailureEmitsWarnDiagnostic verifies the loader
// degrades gracefully when the cache backend rejects a write — the
// diagnostic surfaces as a Warn so the run continues, and the
// store still receives the freshly-converted package.
func TestCache_PutFailureEmitsWarnDiagnostic(t *testing.T) {
	t.Parallel()
	t.Run("Put error surfaces as a Warn", func(t *testing.T) {
		t.Parallel()
		dir := materialiseGoSource(t, map[string]string{
			"a.go": "package a\n\ntype S struct{}\n",
		})
		s, d := loadDir(t, dir, &putFailingCache{})
		// Package was still added to the store despite the cache
		// failure.
		got := false
		s.Nodes().Packages().Range(func(*node.Package) bool {
			got = true
			return false
		})
		if !got {
			t.Fatalf("converter must run even when cache Put fails")
		}
		// Warn diagnostic surfaced.
		warns := 0
		for _, dg := range d.Diagnostics() {
			if dg.Message != "" && dg.Severity.String() == "warn" {
				warns++
			}
		}
		if warns == 0 {
			t.Fatalf("expected at least one Warn diagnostic for the Put failure, got %+v", d.Diagnostics())
		}
	})
}

// TestCacheUsable is the whole regression barrier for the largest
// item in the frontend's load cost: a run with the cache switched off
// paying to hash every source file and marshal the entire node graph
// for a store that discards both.
//
// A pure predicate, so the barrier is cheap. It is also the only
// place the None case is stated — downstream both guards test for
// nil, and a *cache.None is not nil.
func TestCacheUsable(t *testing.T) {
	t.Parallel()

	t.Run("a nil cache cannot retain a write", func(t *testing.T) {
		t.Parallel()
		if frontend.CacheUsableForTest(nil) {
			t.Fatalf("nil cache reported usable")
		}
	})

	t.Run("a None cache cannot retain a write", func(t *testing.T) {
		t.Parallel()
		// The case the nil guards miss. None.Get always misses and
		// None.Put always discards, so every byte spent producing a
		// key or a body for it is wasted.
		if frontend.CacheUsableForTest(cache.NewNone()) {
			t.Fatalf("None cache reported usable")
		}
	})

	t.Run("a disk cache is usable", func(t *testing.T) {
		t.Parallel()
		if !frontend.CacheUsableForTest(cache.NewDisk(t.TempDir())) {
			t.Fatalf("disk cache reported unusable")
		}
	})

	t.Run("an unrecognised implementation is assumed usable", func(t *testing.T) {
		t.Parallel()
		// Conservative on purpose: guessing wrong here costs only
		// the status quo, where guessing wrong the other way would
		// silently disable caching for a third-party store.
		if !frontend.CacheUsableForTest(stubCache{}) {
			t.Fatalf("unknown cache implementation reported unusable")
		}
	})
}

// stubCache is a third-party-shaped [cache.Cache] the predicate has
// never seen.
type stubCache struct{}

func (stubCache) Get(string) ([]byte, bool) { return nil, false }
func (stubCache) Put(string, []byte) error  { return nil }
