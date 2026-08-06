// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store_test

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"go.thesmos.sh/eidos/store"
)

func TestNewReadSet(t *testing.T) {
	t.Parallel()

	t.Run("returns an empty ReadSet", func(t *testing.T) {
		t.Parallel()
		r := store.NewReadSet()
		if r.Len() != 0 {
			t.Fatalf("new ReadSet should be empty; got Len=%d", r.Len())
		}
	})
}

func TestReadSet_Record(t *testing.T) {
	t.Parallel()

	t.Run("records a key", func(t *testing.T) {
		t.Parallel()
		r := store.NewReadSet()
		r.Record("key-a")
		if !r.Has("key-a") {
			t.Fatalf("Record should make Has return true")
		}
	})

	t.Run("recording the same key twice is idempotent", func(t *testing.T) {
		t.Parallel()
		r := store.NewReadSet()
		r.Record("key-a")
		r.Record("key-a")
		if r.Len() != 1 {
			t.Fatalf("Len after duplicate Record = %d, want 1", r.Len())
		}
	})
}

func TestReadSet_Has(t *testing.T) {
	t.Parallel()

	t.Run("returns false for unrecorded keys", func(t *testing.T) {
		t.Parallel()
		r := store.NewReadSet()
		if r.Has("missing") {
			t.Fatalf("Has on empty ReadSet should be false")
		}
	})
}

func TestReadSet_Len(t *testing.T) {
	t.Parallel()

	t.Run("counts distinct recorded keys", func(t *testing.T) {
		t.Parallel()
		r := store.NewReadSet()
		r.Record("a")
		r.Record("b")
		r.Record("c")
		if r.Len() != 3 {
			t.Fatalf("Len = %d, want 3", r.Len())
		}
	})
}

func TestReadSet_Keys(t *testing.T) {
	t.Parallel()

	t.Run("returns recorded keys in lexicographic order", func(t *testing.T) {
		t.Parallel()
		r := store.NewReadSet()
		r.Record("c")
		r.Record("a")
		r.Record("b")
		if !slices.Equal(r.Keys(), []string{"a", "b", "c"}) {
			t.Fatalf("Keys not sorted: %v", r.Keys())
		}
	})
}

func TestReadSet_Hash(t *testing.T) {
	t.Parallel()

	t.Run("identical keys produce identical hashes regardless of insertion order", func(t *testing.T) {
		t.Parallel()
		r1 := store.NewReadSet()
		r1.Record("a")
		r1.Record("b")
		r1.Record("c")
		r2 := store.NewReadSet()
		r2.Record("c")
		r2.Record("b")
		r2.Record("a")
		if r1.Hash() != r2.Hash() {
			t.Fatalf("hashes should be order-independent")
		}
	})

	t.Run("different keys produce different hashes", func(t *testing.T) {
		t.Parallel()
		r1 := store.NewReadSet()
		r1.Record("a")
		r2 := store.NewReadSet()
		r2.Record("b")
		if r1.Hash() == r2.Hash() {
			t.Fatalf("hashes should differ for different keys")
		}
	})

	t.Run("empty ReadSet has a deterministic empty hash", func(t *testing.T) {
		t.Parallel()
		r1 := store.NewReadSet()
		r2 := store.NewReadSet()
		if r1.Hash() != r2.Hash() {
			t.Fatalf("empty ReadSets should produce the same hash")
		}
	})
}

func TestReadSet_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	t.Run("Record and Hash are safe under -race", func(t *testing.T) {
		t.Parallel()
		r := store.NewReadSet()
		var wg sync.WaitGroup
		for i := range 16 {
			wg.Go(func() {
				r.Record(string(rune('a' + i)))
			})
		}
		for range 4 {
			wg.Go(func() {
				_ = r.Hash()
			})
		}
		wg.Wait()
	})
}

// ExampleReadSet_Hash shows the property the cache layer depends on:
// two plugins that observed the same things produce the same digest
// no matter what order they looked at them in.
func ExampleReadSet_Hash() {
	first := store.NewReadSet()
	first.Record("node:structs")
	first.Record("node:methods")

	second := store.NewReadSet()
	second.Record("node:methods")
	second.Record("node:structs")

	fmt.Println(first.Hash() == second.Hash())
	fmt.Println(first.Hash())

	// Output:
	// true
	// 0157c1dd5fd4be47f3d0ef1cc73aacd6afb4dc947d4a214ccacba6d914f97ba4
}

// recordAll returns a ReadSet holding every key in keys, recorded in
// the order given. Order is the variable under test in the fuzz
// target, so the helper never sorts.
func recordAll(keys []string) *store.ReadSet {
	rs := store.NewReadSet()
	for _, k := range keys {
		rs.Record(k)
	}
	return rs
}

// rotatedBy returns keys rotated left by n positions — a permutation
// that, unlike reversal, is not its own inverse, so it catches an
// implementation that happens to be symmetric rather than genuinely
// order-free. Empty input is returned unchanged.
func rotatedBy(keys []string, n int) []string {
	if len(keys) == 0 {
		return keys
	}
	n %= len(keys)
	out := make([]string, 0, len(keys))
	out = append(out, keys[n:]...)
	out = append(out, keys[:n]...)
	return out
}

// distinctSorted returns the deduplicated, sorted key set — the naive
// reference model of what a ReadSet holds, computed independently of
// the ReadSet's own storage so the two can be compared.
func distinctSorted(keys []string) []string {
	out := slices.Clone(keys)
	slices.Sort(out)
	return slices.Compact(out)
}

// FuzzReadSet_Hash drives the cache-key digest over arbitrary key
// sets.
//
// Hash is the stability contract for the whole cache: the pipeline
// writes it as the marker for a plugin's outputs, so two runs that
// observed the same declarations must produce the same digest, and
// two runs that observed different ones must not. Neither failure
// mode is a crash — a Hash that depended on insertion order would
// simply stop reusing valid cache entries, and one that collided
// would serve a stale output as fresh. Both are silent, so the
// properties asserted here are equalities and inequalities, never
// the absence of a panic.
//
// Each input string is split on newlines into a key set; the second
// string supplies an independent set to compare against, and the
// rotation byte supplies a permutation of the first.
//
// The seeds cover the structural branches (empty set, single key,
// repeated key, many keys) and the boundaries that break naive
// digest framing: keys that are prefixes of one another, keys
// containing the NUL byte the implementation uses as its separator,
// invalid UTF-8, and the classic split-ambiguity pair
// {"a\x00b"} versus {"a", "b"}.
func FuzzReadSet_Hash(f *testing.F) {
	for _, seed := range []struct {
		left, right string
		rotate      uint8
	}{
		{"", "", 0},
		{"node:structs", "", 0},
		{"node:structs", "node:structs", 1},
		{"node:structs\nnode:methods", "node:methods\nnode:structs", 1},
		{"a\nb\nc", "c\nb\na", 2},
		{strings.Repeat("dup\n", 2) + "dup", "dup", 3}, // one key recorded three times
		{"a\nab\nabc", "abc\nab\na", 1},
		{"a\x00b", "a\nb", 0},                 // separator ambiguity
		{"\x00", "", 1},                       // key that is only the separator
		{"\xff\xfe", "\xff\xfe", 1},           // invalid UTF-8
		{"node:structs\n", "node:structs", 1}, // trailing empty key
		{strings.Repeat("k\n", 64), "k", 7},
	} {
		f.Add(seed.left, seed.right, seed.rotate)
	}

	f.Fuzz(func(t *testing.T, left, right string, rotate uint8) {
		leftKeys := strings.Split(left, "\n")
		rightKeys := strings.Split(right, "\n")

		forward := recordAll(leftKeys)

		// The ReadSet must agree with the naive set model, or every
		// property below is asserted against the wrong thing.
		want := distinctSorted(leftKeys)
		if got := forward.Keys(); !slices.Equal(got, want) {
			t.Fatalf("Keys() = %q, want the distinct sorted set %q", got, want)
		}
		if forward.Len() != len(want) {
			t.Fatalf("Len() = %d, want %d distinct keys", forward.Len(), len(want))
		}

		// Order independence — the stated contract. A hash that moved
		// with insertion order would make every cache lookup a miss
		// for a plugin whose queries ran in a different sequence.
		reversed := slices.Clone(leftKeys)
		slices.Reverse(reversed)
		permutations := []struct {
			name string
			keys []string
		}{
			{"reversed", reversed},
			{"rotated", rotatedBy(leftKeys, int(rotate))},
			{"sorted", want},
		}
		for _, p := range permutations {
			if got := recordAll(p.keys).Hash(); got != forward.Hash() {
				t.Fatalf("%s insertion order changed the hash: %s != %s", p.name, got, forward.Hash())
			}
		}

		// Re-recording every key must not move the digest either:
		// Record is documented as idempotent, and plugins do re-query.
		doubled := recordAll(append(slices.Clone(leftKeys), leftKeys...))
		if got := doubled.Hash(); got != forward.Hash() {
			t.Fatalf("re-recording the same keys changed the hash: %s != %s", got, forward.Hash())
		}

		// Distinct sets must produce distinct digests, or the cache
		// serves one plugin's outputs for another's inputs.
		//
		// Restricted to NUL-free keys: Hash frames each key with a
		// trailing 0x00 and nothing else, so {"a\x00b"} and
		// {"a", "b"} encode to the same byte string and collide. That
		// is a real defect in the framing, reported rather than
		// asserted here — deleting this guard is how it gets caught
		// once the framing carries a length prefix.
		rightSet := distinctSorted(rightKeys)
		if !containsNUL(want) && !containsNUL(rightSet) {
			sameSet := slices.Equal(want, rightSet)
			sameHash := forward.Hash() == recordAll(rightKeys).Hash()
			if sameSet != sameHash {
				t.Fatalf("set equality %v but hash equality %v for %q vs %q",
					sameSet, sameHash, want, rightSet)
			}
		}
	})
}

// containsNUL reports whether any key embeds the byte the digest
// uses as its key separator.
func containsNUL(keys []string) bool {
	for _, k := range keys {
		if strings.ContainsRune(k, 0) {
			return true
		}
	}
	return false
}

// BenchmarkReadSet_Hash measures one digest over a read set of n
// keys: a locked copy of the key map, a sort, then SHA-256 over the
// framed keys.
//
// The pipeline calls Hash once per plugin per run to write the cache
// marker, so absolute cost barely matters — what matters is the
// shape. Keys() sorts, so the expected curve is n log n; the sweep
// exists to catch a rewrite that makes it quadratic, which would be
// unnoticeable on the ten-tag read sets today's plugins produce and
// painful once reads are recorded per declaration.
//
// The read set is populated above the loop; the timed region is Hash
// alone, including the key-slice copy it cannot avoid. Allocations
// must be non-zero — Hash builds a fresh sorted slice and a hex
// string every call, and a zero here would mean the call was
// optimised out.
//
// Read the sizes for what they are. store/query.go is the only
// non-test Record caller in the workspace and its tag is one of the
// 26 compile-time literals in store/reader.go, so 26 is the
// reachable ceiling today and the /100 and /1000 arms are
// forward-looking headroom for per-declaration read tracking. Do not
// price a change off them: the fixture's keys are 45-47 bytes while
// the longest production tag is 18, under the runtime's stack
// buffer, so the per-key allocation those arms report does not occur
// in production at all.
func BenchmarkReadSet_Hash(b *testing.B) {
	b.ReportAllocs()

	for _, n := range benchBucketSizes {
		rs := store.NewReadSet()
		for i := range n {
			rs.Record("node:structs:github.com/example/bench.Entity" + strconv.Itoa(i))
		}
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			var digest string
			for b.Loop() {
				digest = rs.Hash()
			}
			if len(digest) != 64 {
				b.Fatalf("digest = %q, want 64 hex chars; the timed loop produced nothing", digest)
			}
		})
	}
}
