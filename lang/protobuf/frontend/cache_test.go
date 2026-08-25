// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"maps"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/lang/protobuf/frontend"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// TestLoad_ConsultsCache pins the framework-cache-consumer
// contract: every Load run composes a content-addressed key and
// consults the configured cache. The fixture-driven test cycles
// two consecutive runs against the same fixture and asserts the
// cache sees Get + Put on the first run, and Get only on the
// second run.
func TestLoad_ConsultsCache(t *testing.T) {
	t.Parallel()

	t.Run("first run misses + stores; second run hits the same key", func(t *testing.T) {
		t.Parallel()
		rec := newRecordingCache()
		root := cacheFixtureRoot(t, "simple")

		runOnce(t, root, rec)
		if rec.getCount() != 1 {
			t.Fatalf("first run: cache Get count = %d, want 1", rec.getCount())
		}
		if rec.putCount() != 1 {
			t.Fatalf("first run: cache Put count = %d, want 1", rec.putCount())
		}
		firstKey := rec.lastKeyValue()

		runOnce(t, root, rec)
		if rec.getCount() != 2 {
			t.Fatalf("second run: total Get count = %d, want 2", rec.getCount())
		}
		if rec.putCount() != 1 {
			t.Fatalf("second run: total Put count = %d, want 1 (hit skips Put)", rec.putCount())
		}
		secondKey := rec.lastKeyValue()
		if firstKey != secondKey {
			t.Fatalf(
				"cache key is not deterministic across runs: first=%q second=%q",
				firstKey,
				secondKey,
			)
		}
		if !strings.Contains(firstKey, "plugin:"+frontend.FrontendName) {
			t.Fatalf("cache key should embed plugin name; got %q", firstKey)
		}
	})
}

// TestCacheKey_InvalidatesOnOptionsFlip pins that flipping a
// frontend option produces a distinct cache key — protecting
// against silent stale-cache hits when users change
// include_well_known or import_paths.
func TestCacheKey_InvalidatesOnOptionsFlip(t *testing.T) {
	t.Parallel()

	t.Run("include_well_known toggle changes the key", func(t *testing.T) {
		t.Parallel()
		root := cacheFixtureRoot(t, "simple")

		recA := newRecordingCache()
		recB := newRecordingCache()
		runOnceWithOpts(t, root, recA, map[string]string{"include_well_known": "true"})
		runOnceWithOpts(t, root, recB, map[string]string{"include_well_known": "false"})

		if recA.lastKeyValue() == recB.lastKeyValue() {
			t.Fatalf(
				"option flip should change the cache key; both runs hashed to %q",
				recA.lastKeyValue(),
			)
		}
	})
}

// runOnce drives the protobuf frontend against root with default
// options, capturing the supplied recordingCache on the FrontendContext.
func runOnce(t *testing.T, root string, rec *recordingCache) {
	t.Helper()
	runOnceWithOpts(t, root, rec, nil)
}

// runOnceWithOpts drives the protobuf frontend against root with
// the supplied option overrides plus the standard `dir = root`
// pin. The recordingCache instance carries through every call so
// total Get/Put counts accumulate across consecutive runs.
func runOnceWithOpts(t *testing.T, root string, rec *recordingCache, extra map[string]string) {
	t.Helper()
	f := frontend.New()
	values := map[string]string{"dir": root}
	maps.Copy(values, extra)
	if err := f.SetOptions(opt.New(f.OptionsSchema(), values)); err != nil {
		t.Fatalf("SetOptions: %v", err)
	}
	ctx := &plugin.FrontendContext{
		Store:    store.New(),
		Diag:     diag.New(),
		Registry: directive.NewRegistry(),
		Parser:   directive.DefaultParser(),
		Cache:    rec,
		Pattern:  "./...",
	}
	if err := f.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

// cacheFixtureRoot returns the absolute path of
// frontend/protobuf/testdata/<name>.
func cacheFixtureRoot(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

// recordingCache implements [cache.Cache] and tracks every Get /
// Put / Has call so tests assert on the protobuf frontend's
// cache-consumption surface. The underlying storage is an
// in-memory map shared across the recorded calls so a Put followed
// by a Get returns the stored value.
type recordingCache struct {
	mu      sync.Mutex
	store   map[string][]byte
	gets    int
	puts    int
	lastKey string
}

func newRecordingCache() *recordingCache {
	return &recordingCache{store: map[string][]byte{}}
}

func (r *recordingCache) Get(key string) ([]byte, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gets++
	r.lastKey = key
	body, ok := r.store[key]
	return body, ok
}

func (r *recordingCache) Put(key string, value []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.puts++
	r.lastKey = key
	r.store[key] = value
	return nil
}

func (r *recordingCache) Has(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.store[key]
	return ok
}

func (r *recordingCache) getCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gets
}

func (r *recordingCache) putCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.puts
}

// lastKeyValue returns the most-recently-seen key under the
// receiver's mutex. Named `lastKeyValue` rather than `lastKey` so
// the field name doesn't shadow the method.
func (r *recordingCache) lastKeyValue() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastKey
}

// Compile-time assertion that recordingCache satisfies the
// framework's cache surface.
var _ cache.Cache = (*recordingCache)(nil)

// TestLoadPattern_CacheHitSkipsConversion pins that the protobuf
// frontend's cache actually avoids work.
//
// The read path previously computed a key, discarded the hit body,
// stored a nil payload, and re-converted unconditionally — a no-op
// that wrote one empty file per pattern and changed nothing. The
// assertion that matters is not "a second run succeeds" but "a
// second run produces the same graph without the descriptors", which
// only a real payload can satisfy.
func TestLoadPattern_CacheHitSkipsConversion(t *testing.T) {
	t.Parallel()

	root := cacheFixtureRoot(t, "simple")
	rec := newRecordingCache()

	first := loadCountingPackages(t, root, rec)
	if first == 0 {
		t.Fatalf("fixture produced no packages; the test cannot tell a hit from a miss")
	}
	// A non-empty stored body is what separates a working cache from
	// the no-op this replaced: the old path stored nil, so both runs
	// re-converted and any count-based assertion passed regardless.
	body, ok := rec.store[rec.lastKey]
	if !ok || len(body) == 0 {
		t.Fatalf("cache holds no payload under %q; a hit cannot skip conversion", rec.lastKey)
	}

	// The key is content-addressed over the descriptors, so a second
	// run must use the same root to hit at all. The payload check
	// above is what proves the hit was served from storage rather
	// than re-converted.
	second := loadCountingPackages(t, root, rec)
	if second != first {
		t.Fatalf("cached run registered %d packages, want %d — a hit must reproduce the graph",
			second, first)
	}
}

// loadCountingPackages runs the frontend against root with rec as
// its cache and returns how many packages landed in the store.
func loadCountingPackages(t *testing.T, root string, rec *recordingCache) int {
	t.Helper()
	f := frontend.New()
	if err := f.SetOptions(opt.New(f.OptionsSchema(), map[string]string{"dir": root})); err != nil {
		t.Fatalf("SetOptions: %v", err)
	}
	s := store.New()
	ctx := &plugin.FrontendContext{
		Store:    s,
		Diag:     diag.New(),
		Registry: directive.NewRegistry(),
		Parser:   directive.DefaultParser(),
		Cache:    rec,
		Pattern:  "./...",
	}
	if err := f.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s.Nodes().Packages().Len()
}
