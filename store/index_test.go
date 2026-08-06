// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store_test

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"go.thesmos.sh/eidos/store"
)

// benchBucketSizes are the bucket populations the scaling benchmarks
// sweep. 1 exposes fixed overhead, 1000 is above the size at which a
// hidden linear scan inside a per-item operation stops hiding — a
// real frontend run indexes low thousands of declarations.
var benchBucketSizes = []int{1, 10, 100, 1000}

// benchQNames builds n distinct qualified names shaped like the ones
// the store really holds — a package path plus a type name — so the
// map's hashing cost reflects realistic key lengths rather than the
// single-byte keys a synthetic fixture would use.
func benchQNames(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "github.com/example/bench.Entity" + strconv.Itoa(i)
	}
	return out
}

func TestNewBucket(t *testing.T) {
	t.Parallel()

	t.Run("returns an empty bucket ready for use", func(t *testing.T) {
		t.Parallel()
		b := store.NewBucket[int]()
		if b.Len() != 0 {
			t.Fatalf("new bucket should be empty; got Len=%d", b.Len())
		}
	})
}

func TestBucket_Add(t *testing.T) {
	t.Parallel()

	t.Run("appends item under qname", func(t *testing.T) {
		t.Parallel()
		b := store.NewBucket[string]()
		assertNoError(t, b.Add("a", "first"))
		assertNoError(t, b.Add("b", "second"))
		if b.Len() != 2 {
			t.Fatalf("expected Len=2; got %d", b.Len())
		}
	})

	t.Run("rejects duplicate qnames with ErrDuplicateQName", func(t *testing.T) {
		t.Parallel()
		b := store.NewBucket[int]()
		assertNoError(t, b.Add("k", 1))
		err := b.Add("k", 2)
		if !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("expected ErrDuplicateQName; got %v", err)
		}
		if !strings.Contains(err.Error(), "k") {
			t.Fatalf("error should mention the offending qname; got %q", err.Error())
		}
	})
}

func TestBucket_ByQName(t *testing.T) {
	t.Parallel()

	t.Run("returns the recorded item", func(t *testing.T) {
		t.Parallel()
		b := store.NewBucket[string]()
		assertNoError(t, b.Add("k", "value"))
		v, ok := b.ByQName("k")
		if !ok || v != "value" {
			t.Fatalf("ByQName mismatch: got %q ok=%v", v, ok)
		}
	})

	t.Run("returns zero value and false for unknown qname", func(t *testing.T) {
		t.Parallel()
		b := store.NewBucket[string]()
		v, ok := b.ByQName("missing")
		if ok || v != "" {
			t.Fatalf("ByQName(unknown) should return zero value and false; got %q ok=%v", v, ok)
		}
	})
}

func TestBucket_Items(t *testing.T) {
	t.Parallel()

	t.Run("returns items in insertion order", func(t *testing.T) {
		t.Parallel()
		b := store.NewBucket[string]()
		assertNoError(t, b.Add("a", "first"))
		assertNoError(t, b.Add("b", "second"))
		assertNoError(t, b.Add("c", "third"))
		got := b.Items()
		if !slices.Equal(got, []string{"first", "second", "third"}) {
			t.Fatalf("Items order mismatch: %v", got)
		}
	})

	t.Run("returns a copy that can be mutated independently", func(t *testing.T) {
		t.Parallel()
		b := store.NewBucket[int]()
		assertNoError(t, b.Add("a", 1))
		first := b.Items()
		first[0] = 99
		second := b.Items()
		if second[0] != 1 {
			t.Fatalf("mutation of returned slice should not affect bucket; got %d", second[0])
		}
	})
}

func TestBucket_Range(t *testing.T) {
	t.Parallel()

	t.Run("invokes fn for each item in insertion order", func(t *testing.T) {
		t.Parallel()
		b := store.NewBucket[int]()
		for i := 1; i <= 3; i++ {
			assertNoError(t, b.Add(string(rune('a'+i-1)), i))
		}
		var got []int
		b.Range(func(v int) bool {
			got = append(got, v)
			return true
		})
		if !slices.Equal(got, []int{1, 2, 3}) {
			t.Fatalf("Range order mismatch: %v", got)
		}
	})

	t.Run("returning false stops iteration", func(t *testing.T) {
		t.Parallel()
		b := store.NewBucket[int]()
		assertNoError(t, b.Add("a", 1))
		assertNoError(t, b.Add("b", 2))
		assertNoError(t, b.Add("c", 3))
		var visited int
		b.Range(func(int) bool {
			visited++
			return visited < 2
		})
		if visited != 2 {
			t.Fatalf("expected 2 visits before stop; got %d", visited)
		}
	})
}

func TestBucket_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	t.Run("Add and Items are safe under -race", func(t *testing.T) {
		t.Parallel()
		b := store.NewBucket[int]()
		var wg sync.WaitGroup
		const writers = 8
		const writes = 32
		for i := range writers {
			wg.Go(func() {
				for j := range writes {
					key := string(rune('a' + i*writes + j))
					_ = b.Add(key, i*writes+j) //nolint:errcheck // dup tolerated under contention
				}
			})
		}
		for range 4 {
			wg.Go(func() {
				_ = b.Items()
			})
		}
		wg.Wait()
	})
}

func TestNewMultiIndex(t *testing.T) {
	t.Parallel()

	t.Run("returns an empty index ready for use", func(t *testing.T) {
		t.Parallel()
		m := store.NewMultiIndex[string, int]()
		if m.Len() != 0 {
			t.Fatalf("new index should be empty; got Len=%d", m.Len())
		}
	})
}

func TestMultiIndex_Add(t *testing.T) {
	t.Parallel()

	t.Run("appends value under key, preserving insertion order", func(t *testing.T) {
		t.Parallel()
		m := store.NewMultiIndex[string, int]()
		m.Add("k", 1)
		m.Add("k", 2)
		m.Add("k", 3)
		if !slices.Equal(m.Get("k"), []int{1, 2, 3}) {
			t.Fatalf("Add order mismatch: %v", m.Get("k"))
		}
	})
}

func TestMultiIndex_Get(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for unknown keys", func(t *testing.T) {
		t.Parallel()
		m := store.NewMultiIndex[string, int]()
		if m.Get("missing") != nil {
			t.Fatalf("Get(unknown) should be nil")
		}
	})

	t.Run("returns a copy that the caller may mutate", func(t *testing.T) {
		t.Parallel()
		m := store.NewMultiIndex[string, int]()
		m.Add("k", 1)
		first := m.Get("k")
		first[0] = 99
		if m.Get("k")[0] != 1 {
			t.Fatalf("Get should return a defensive copy")
		}
	})
}

func TestMultiIndex_Has(t *testing.T) {
	t.Parallel()

	t.Run("reports true for keys with at least one entry", func(t *testing.T) {
		t.Parallel()
		m := store.NewMultiIndex[string, int]()
		m.Add("k", 1)
		if !m.Has("k") {
			t.Fatalf("Has should report true after Add")
		}
	})

	t.Run("reports false for unknown keys", func(t *testing.T) {
		t.Parallel()
		m := store.NewMultiIndex[string, int]()
		if m.Has("missing") {
			t.Fatalf("Has should report false for unknown key")
		}
	})
}

func TestMultiIndex_Len(t *testing.T) {
	t.Parallel()

	t.Run("counts distinct keys", func(t *testing.T) {
		t.Parallel()
		m := store.NewMultiIndex[string, int]()
		m.Add("a", 1)
		m.Add("a", 2)
		m.Add("b", 3)
		if got := m.Len(); got != 2 {
			t.Fatalf("Len = %d, want 2", got)
		}
	})
}

func TestMultiIndex_Keys(t *testing.T) {
	t.Parallel()

	t.Run("returns distinct keys in first-insertion order", func(t *testing.T) {
		t.Parallel()
		m := store.NewMultiIndex[string, int]()
		m.Add("b", 1)
		m.Add("a", 2)
		m.Add("b", 3) // duplicate key — must not re-order
		m.Add("c", 4)
		got := m.Keys()
		want := []string{"b", "a", "c"}
		if len(got) != len(want) {
			t.Fatalf("Keys = %v, want %v", got, want)
		}
		for i, k := range want {
			if got[i] != k {
				t.Fatalf("Keys[%d] = %q, want %q (full %v)", i, got[i], k, got)
			}
		}
	})

	t.Run("empty index returns non-nil zero-length slice", func(t *testing.T) {
		t.Parallel()
		m := store.NewMultiIndex[string, int]()
		got := m.Keys()
		if got == nil {
			t.Fatalf("Keys on empty index should be non-nil")
		}
		if len(got) != 0 {
			t.Fatalf("Keys on empty index = %v, want empty", got)
		}
	})

	t.Run("mutating the returned slice does not affect the index", func(t *testing.T) {
		t.Parallel()
		m := store.NewMultiIndex[string, int]()
		m.Add("x", 1)
		got := m.Keys()
		got[0] = "tampered"
		fresh := m.Keys()
		if fresh[0] != "x" {
			t.Fatalf("mutating returned slice must not affect index; fresh Keys = %v", fresh)
		}
	})
}

func TestMultiIndex_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	t.Run("Add and Get are safe under -race", func(t *testing.T) {
		t.Parallel()
		m := store.NewMultiIndex[string, int]()
		var wg sync.WaitGroup
		for i := range 8 {
			wg.Go(func() {
				for j := range 32 {
					m.Add("k", i*32+j)
				}
			})
		}
		for range 4 {
			wg.Go(func() {
				_ = m.Get("k")
			})
		}
		wg.Wait()
	})
}

// BenchmarkBucket_Add measures populating a bucket from empty to n
// entries: per item one exclusive lock acquisition, one map-presence
// check, one slice append, and one map insert.
//
// Add is the write half of every frontend and generator pass, so the
// question the scaling sweep answers is whether insertion stays
// linear. A plausible future rewrite — scanning items to detect
// duplicates instead of consulting the map — is invisible at n=1 and
// quadratic at n=1000; only the sweep catches it. Read per-op time
// as "cost of building an n-entry bucket": it must grow by roughly
// the same factor as n.
//
// The keys are generated above the loop. The timed region covers
// NewBucket plus the n Adds, so the reported allocation deliberately
// includes the map's growth — that growth is the cost being
// questioned.
func BenchmarkBucket_Add(b *testing.B) {
	b.ReportAllocs()

	for _, n := range benchBucketSizes {
		keys := benchQNames(n)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				bucket := store.NewBucket[int]()
				for i, k := range keys {
					if err := bucket.Add(k, i); err != nil {
						b.Fatalf("Add(%q): %v", k, err)
					}
				}
			}
		})
	}
}

// BenchmarkBucket_ByQName measures a single successful qualified-name
// lookup against a bucket already holding n entries.
//
// ByQName is the cross-reference primitive — every plugin resolving
// a type it did not declare lands here — and its contract is O(1).
// The sweep is the assertion: per-op time must stay flat as n grows
// by three orders of magnitude. A slope means the map lookup has
// been replaced by a scan.
//
// Zero allocations is the correct result here and not a sign of an
// eliminated loop body: ByQName takes a read lock and returns a
// value already in the map. The accumulator below is what keeps the
// call from being optimised away, and the final check makes the
// accumulator load-bearing.
func BenchmarkBucket_ByQName(b *testing.B) {
	b.ReportAllocs()

	for _, n := range benchBucketSizes {
		keys := benchQNames(n)
		bucket := store.NewBucket[int]()
		for i, k := range keys {
			if err := bucket.Add(k, i+1); err != nil {
				b.Fatalf("Add(%q): %v", k, err)
			}
		}
		// Probe the middle of the insertion order so the result is not
		// biased by whichever entry the map happened to place first.
		probe := keys[n/2]

		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			var found int
			for b.Loop() {
				v, ok := bucket.ByQName(probe)
				if !ok {
					b.Fatalf("ByQName(%q) missing from a bucket of %d", probe, n)
				}
				found += v
			}
			if found == 0 {
				b.Fatalf("accumulator stayed zero: the timed loop did not run")
			}
		})
	}
}
