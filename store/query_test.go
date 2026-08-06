// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store_test

import (
	"fmt"
	"slices"
	"strconv"
	"testing"

	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

func makeStructPopulatedReader(t *testing.T) *store.Reader {
	t.Helper()
	s := store.New()
	assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
	return store.NewReader(s)
}

func TestQuery_Where(t *testing.T) {
	t.Parallel()

	t.Run("filters by the supplied predicate", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		got := r.Structs().
			Where(func(s *node.Struct) bool { return s.Name == "User" }).
			Slice()
		if len(got) != 1 || got[0].Name != "User" {
			t.Fatalf("Where filter mismatch: %+v", got)
		}
	})

	t.Run("multiple Where calls compose as logical AND", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		got := r.Structs().
			Where(func(s *node.Struct) bool { return s.Package != "" }).
			Where(func(s *node.Struct) bool { return s.Name == "User" }).
			Slice()
		if len(got) != 1 || got[0].Name != "User" {
			t.Fatalf("composed Where mismatch: %+v", got)
		}
	})

	// The AND case above cannot tell composition from replacement:
	// its last predicate is the narrowest, so a Where that discarded
	// everything before it would return the same one struct. Ordering
	// the narrow predicate first makes the two behaviours disagree —
	// replacement would widen the result back to both structs.
	t.Run("an earlier predicate still constrains a later, wider one", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		got := r.Structs().
			Where(func(s *node.Struct) bool { return s.Name == "User" }).
			Where(func(s *node.Struct) bool { return s.Package != "" }).
			Slice()
		if len(got) != 1 || got[0].Name != "User" {
			t.Fatalf("later predicate should intersect, not replace, the earlier one: %+v", got)
		}
	})

	t.Run("nil predicate is a no-op", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		got := r.Structs().Where(nil).Slice()
		if len(got) != 2 {
			t.Fatalf("nil Where should not filter; got %d", len(got))
		}
	})

	t.Run("returns a new Query, leaving the original usable", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		original := r.Structs()
		filtered := original.Where(func(s *node.Struct) bool { return s.Name == "User" })
		if filtered == original {
			t.Fatalf("Where should return a new Query")
		}
		if got := original.Slice(); len(got) != 2 {
			t.Fatalf("original Query should remain unfiltered; got %d", len(got))
		}
	})
}

func TestQuery_Each(t *testing.T) {
	t.Parallel()

	t.Run("invokes fn for every match in insertion order", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		var seen []string
		r.Structs().Each(func(s *node.Struct) { seen = append(seen, s.Name) })
		if !slices.Equal(seen, []string{"User", "Address"}) {
			t.Fatalf("Each order = %v, want [User Address]", seen)
		}
	})

	t.Run("respects the accumulated predicate", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		var seen []string
		r.Structs().
			Where(func(s *node.Struct) bool { return s.Name == "Address" }).
			Each(func(s *node.Struct) { seen = append(seen, s.Name) })
		if !slices.Equal(seen, []string{"Address"}) {
			t.Fatalf("filtered Each mismatch: %v", seen)
		}
	})

	t.Run("records the source tag in the ReadSet", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		r.Structs().Each(func(*node.Struct) {})
		if !r.ReadSet().Has("node:structs") {
			t.Fatalf("Each should record the source tag")
		}
	})
}

func TestQuery_Slice(t *testing.T) {
	t.Parallel()

	t.Run("returns matched items in source order", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		got := r.Structs().Slice()
		if len(got) != 2 || got[0].Name != "User" || got[1].Name != "Address" {
			t.Fatalf("Slice order mismatch: %+v", got)
		}
	})

	t.Run("returns an empty slice when nothing matches", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		got := r.Structs().Where(func(*node.Struct) bool { return false }).Slice()
		if len(got) != 0 {
			t.Fatalf("expected empty slice; got %d items", len(got))
		}
	})
}

func TestQuery_First(t *testing.T) {
	t.Parallel()

	t.Run("returns the first match and true", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		got, ok := r.Structs().First()
		if !ok || got.Name != "User" {
			t.Fatalf("First mismatch: %+v ok=%v", got, ok)
		}
	})

	t.Run("returns zero value and false when nothing matches", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		got, ok := r.Structs().Where(func(*node.Struct) bool { return false }).First()
		if ok || got != nil {
			t.Fatalf("First with no match should return (nil, false); got %+v ok=%v", got, ok)
		}
	})
}

func TestQuery_Count(t *testing.T) {
	t.Parallel()

	t.Run("returns the total number of source items when no predicate", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		if got := r.Structs().Count(); got != 2 {
			t.Fatalf("Count = %d, want 2", got)
		}
	})

	t.Run("returns the count of matched items under a predicate", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		got := r.Structs().
			Where(func(s *node.Struct) bool { return s.Name == "User" }).
			Count()
		if got != 1 {
			t.Fatalf("Count(filter=User) = %d, want 1", got)
		}
	})
}

func TestQuery_RecordsReadOnTerminalsOnly(t *testing.T) {
	t.Parallel()

	t.Run("Where alone does not record", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		_ = r.Structs().Where(func(*node.Struct) bool { return true })
		if r.ReadSet().Len() != 0 {
			t.Fatalf("Where should not record reads; got Len=%d", r.ReadSet().Len())
		}
	})

	t.Run("each terminal records exactly once per call", func(t *testing.T) {
		t.Parallel()
		r := makeStructPopulatedReader(t)
		_ = r.Structs().Slice()
		_ = r.Structs().Slice()
		// Same tag, idempotent in the read-set.
		if r.ReadSet().Len() != 1 {
			t.Fatalf("ReadSet should dedupe by tag; got Len=%d", r.ReadSet().Len())
		}
	})
}

// ExampleQuery_Where shows the predicate chain a plugin writes to
// narrow a query to the declarations it was asked to handle: the
// directive predicate selects the annotated structs, the second
// Where ANDs an ordinary closure on top.
func ExampleQuery_Where() {
	s := store.New()
	if err := s.Nodes().AddPackage(makeUserPackage()); err != nil {
		fmt.Println("add package:", err)
		return
	}

	store.NewReader(s).Structs().
		Where(store.WithDirective[*node.Struct]("repo")).
		Where(func(st *node.Struct) bool { return len(st.Fields) > 0 }).
		Each(func(st *node.Struct) { fmt.Println(st.QName()) })

	// Output:
	// github.com/example/users.User
}

// BenchmarkQuery_Where measures the composed predicate chain at 1, 2
// and 3 predicates over a 500-struct store — the size of a mid-sized
// real package set, and enough that per-item work dominates fixed
// overhead.
//
// Where composes by wrapping: each call allocates a closure that
// calls the previous one, so evaluating three predicates costs three
// closure calls per item and not one fused test. The number that
// matters is therefore the delta between 1, 2 and 3, which is the
// price a plugin pays per additional filter clause.
//
// Every case carries the same constant baseline, deliberately inside
// the timed region because no plugin can avoid it: Reader.Structs
// materialises a defensive copy of the whole struct bucket before
// the first predicate ever runs. If the deltas look small next to
// the total, that copy is the reason — and that is the finding, not
// a flaw in the measurement.
//
// Count is the terminal so the result slice does not dominate:
// what is being measured is predicate evaluation, not
// materialisation.
func BenchmarkQuery_Where(b *testing.B) {
	b.ReportAllocs()

	s := store.New()
	if err := s.Nodes().AddPackage(makeBenchPackage(500)); err != nil {
		b.Fatalf("AddPackage: %v", err)
	}
	r := store.NewReader(s)

	// Ordered cheapest-first so each added predicate is genuinely
	// extra work rather than an early-out that skips the others.
	preds := []func(*node.Struct) bool{
		func(st *node.Struct) bool { return st.Package != "" },
		store.WithDirective[*node.Struct]("repo"),
		func(st *node.Struct) bool { return len(st.Fields) == 3 },
	}

	for _, n := range []int{1, 2, 3} {
		chain := preds[:n]
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			matched := 0
			for b.Loop() {
				q := r.Structs()
				for _, p := range chain {
					q = q.Where(p)
				}
				matched += q.Count()
			}
			if matched == 0 {
				b.Fatalf("no struct matched %d predicates: the benchmark measures an empty scan", n)
			}
		})
	}
}
