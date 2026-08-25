// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugin_test

import (
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// These moved here with the dance they cover.
//
// Every frontend performed it — hash, key, look up, re-wire, or
// convert and write back — and only the hash is language-specific. The
// contract's one capitalised MUST is that the composition fingerprint
// reaches the key, and while each frontend composed its own key that
// MUST could only be policed by a conformance suite. Composed here, it
// is structural: the tests below drive the framework's own path, so a
// frontend cannot pass them by accident or fail them by omission.

// loadCtx returns a context over c, with the given fingerprint.
func loadCtx(t *testing.T, c cache.Cache, fingerprint string) *plugin.FrontendContext {
	t.Helper()
	return &plugin.FrontendContext{
		Store:       store.New(),
		Diag:        diag.New(),
		Cache:       c,
		Fingerprint: fingerprint,
	}
}

// pkg returns one loadable package.
func pkg(name string) []*node.Package {
	return []*node.Package{{Name: name, Path: "example.com/" + name}}
}

// A cache that can retain nothing is not asked for a hash.
//
// The largest item in a frontend's load cost: hashing every source
// file and marshalling the whole node graph for a store that discards
// both. The predicate behind it used to live in the Go frontend, where
// it guarded only that frontend's copy.
func TestCacheLoad_SkipsTheHashWhenNothingCanBeRetained(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		cache cache.Cache
	}{
		{"a nil cache", nil},
		// The case a nil guard misses: None.Get always misses and
		// None.Put always discards, and a *cache.None is not nil.
		{"a None cache", cache.NewNone()},
	} {
		t.Run(tc.name+" is never asked for one", func(t *testing.T) {
			t.Parallel()
			asked := false
			err := plugin.CacheLoad(loadCtx(t, tc.cache, "fp"), "fe", "1", "u",
				func() (string, error) { asked = true; return "h", nil },
				func() ([]*node.Package, error) { return pkg("a"), nil },
			)
			if err != nil {
				t.Fatalf("CacheLoad: %v", err)
			}
			if asked {
				t.Error("the hash was computed for a cache that cannot retain it")
			}
		})
	}

	t.Run("an unrecognised implementation is assumed to retain", func(t *testing.T) {
		t.Parallel()
		// Conservative on purpose: guessing wrong here costs the status
		// quo, where guessing wrong the other way silently disables
		// caching for a third-party store.
		asked := false
		c := cache.NewDisk(t.TempDir())
		if err := plugin.CacheLoad(loadCtx(t, c, "fp"), "fe", "1", "u",
			func() (string, error) { asked = true; return "h", nil },
			func() ([]*node.Package, error) { return pkg("a"), nil },
		); err != nil {
			t.Fatalf("CacheLoad: %v", err)
		}
		if !asked {
			t.Error("a retaining cache should have been keyed")
		}
	})
}

// A second load over unchanged inputs converts nothing.
func TestCacheLoad_HitSkipsConversion(t *testing.T) {
	t.Parallel()

	c := cache.NewDisk(t.TempDir())
	load := func(convert plugin.FrontendConvert) error {
		return plugin.CacheLoad(loadCtx(t, c, "fp"), "fe", "1", "u",
			func() (string, error) { return "same", nil }, convert)
	}
	if err := load(func() ([]*node.Package, error) { return pkg("a"), nil }); err != nil {
		t.Fatalf("first load: %v", err)
	}

	converted := false
	if err := load(func() ([]*node.Package, error) {
		converted = true
		return pkg("a"), nil
	}); err != nil {
		t.Fatalf("second load: %v", err)
	}
	if converted {
		t.Error("the second load converted; a hit exists to avoid exactly that")
	}
}

// The composition fingerprint reaches the key.
//
// The frontend contract's one capitalised MUST. A plugin set that
// changed is a graph that may need to differ, and a key that ignored
// the fingerprint would serve the old one — a hit indistinguishable
// from a correct one.
func TestCacheLoad_FingerprintInvalidates(t *testing.T) {
	t.Parallel()

	c := cache.NewDisk(t.TempDir())
	load := func(fingerprint string, convert plugin.FrontendConvert) error {
		return plugin.CacheLoad(loadCtx(t, c, fingerprint), "fe", "1", "u",
			func() (string, error) { return "same", nil }, convert)
	}
	if err := load("before", func() ([]*node.Package, error) { return pkg("a"), nil }); err != nil {
		t.Fatalf("first load: %v", err)
	}

	converted := false
	if err := load("after", func() ([]*node.Package, error) {
		converted = true
		return pkg("a"), nil
	}); err != nil {
		t.Fatalf("second load: %v", err)
	}
	if !converted {
		t.Error("a recomposed plugin set was served a graph parsed before it existed")
	}
}

// Two units in one run do not collide.
func TestCacheLoad_UnitsAreKeyedApart(t *testing.T) {
	t.Parallel()

	c := cache.NewDisk(t.TempDir())
	load := func(unit string, convert plugin.FrontendConvert) error {
		return plugin.CacheLoad(loadCtx(t, c, "fp"), "fe", "1", unit,
			func() (string, error) { return "same", nil }, convert)
	}
	if err := load("one", func() ([]*node.Package, error) { return pkg("a"), nil }); err != nil {
		t.Fatalf("first load: %v", err)
	}

	converted := false
	if err := load("two", func() ([]*node.Package, error) {
		converted = true
		return pkg("b"), nil
	}); err != nil {
		t.Fatalf("second load: %v", err)
	}
	if !converted {
		// Identical inputs are ordinary between units — two empty
		// packages hash alike — so the unit has to be in the key.
		t.Error("a second unit read back the first one's graph")
	}
}

// A hash that cannot be computed converts rather than failing.
//
// An input the frontend cannot read is a reason not to key the unit,
// not a reason to abandon it: the conversion may well succeed, and a
// run that stopped here would fail over an optimisation.
func TestCacheLoad_UnkeyableUnitStillConverts(t *testing.T) {
	t.Parallel()

	converted := false
	err := plugin.CacheLoad(loadCtx(t, cache.NewDisk(t.TempDir()), "fp"), "fe", "1", "u",
		func() (string, error) { return "", errUnkeyable },
		func() ([]*node.Package, error) { converted = true; return pkg("a"), nil },
	)
	if err != nil {
		t.Fatalf("CacheLoad: %v", err)
	}
	if !converted {
		t.Error("a unit that could not be keyed was not converted either")
	}
}

// errUnkeyable stands in for whatever stops a frontend hashing its
// inputs — a file deleted between the load and the hash, typically.
var errUnkeyable = stubError("cannot read inputs")

type stubError string

func (e stubError) Error() string { return string(e) }

// A hit comes back with the owner pointers encoding strips.
//
// JSON breaks the host-to-child cycles deliberately, so a graph read
// straight back has fields, methods and variants whose Owner is nil.
// Nothing fails at that point — it fails later, in whatever walks
// upward from a member, which is a long way from the cache.
func TestCacheLoad_HitRewiresOwners(t *testing.T) {
	t.Parallel()

	c := cache.NewDisk(t.TempDir())
	withField := func() []*node.Package {
		s := &node.Struct{Name: "User", Package: "example.com/a"}
		s.Fields = []*node.Field{{Name: "ID", Owner: s}}
		return []*node.Package{{Name: "a", Path: "example.com/a", Structs: []*node.Struct{s}}}
	}
	load := func(ctx *plugin.FrontendContext) error {
		return plugin.CacheLoad(ctx, "fe", "1", "u",
			func() (string, error) { return "same", nil },
			func() ([]*node.Package, error) { return withField(), nil })
	}
	if err := load(loadCtx(t, c, "fp")); err != nil {
		t.Fatalf("first load: %v", err)
	}

	ctx := loadCtx(t, c, "fp")
	if err := load(ctx); err != nil {
		t.Fatalf("second load: %v", err)
	}
	got, ok := store.NewReader(ctx.Store).Structs().
		Where(func(s *node.Struct) bool { return s.Name == "User" }).First()
	if !ok {
		t.Fatal("the cached package did not reach the store")
	}
	if got.Fields[0].Owner != got {
		t.Error("a member came back with no owner; the walk upward from it " +
			"fails somewhere that names neither the cache nor this run")
	}
}

// A corrupt entry converts rather than failing.
//
// The cache memoises work that can always be redone, so an unreadable
// payload costs one conversion — and failing the run over it would
// stop a build the next write repairs.
func TestCacheLoad_CorruptEntryIsAMiss(t *testing.T) {
	t.Parallel()

	converted := false
	err := plugin.CacheLoad(loadCtx(t, corruptCache{}, "fp"), "fe", "1", "u",
		func() (string, error) { return "same", nil },
		func() ([]*node.Package, error) { converted = true; return pkg("a"), nil },
	)
	if err != nil {
		t.Fatalf("CacheLoad: %v", err)
	}
	if !converted {
		t.Error("a corrupt entry was read back as a hit")
	}
}

// corruptCache answers every lookup with a payload that is not a
// graph.
//
// A wrapper rather than a poked-at disk entry: the key is the
// framework's to compose, and a test that had to find the file on disk
// would be asserting the key's shape rather than what happens to a
// payload that cannot be read.
type corruptCache struct{}

func (corruptCache) Get(string) ([]byte, bool) { return []byte("{not json"), true }
func (corruptCache) Put(string, []byte) error  { return nil }
