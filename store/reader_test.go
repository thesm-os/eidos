// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

func TestNewReader(t *testing.T) {
	t.Parallel()

	t.Run("returns a Reader with empty ReadSet", func(t *testing.T) {
		t.Parallel()
		r := store.NewReader(store.New())
		if r.ReadSet() == nil {
			t.Fatalf("ReadSet should be non-nil")
		}
		if r.ReadSet().Len() != 0 {
			t.Fatalf("new Reader's ReadSet should be empty")
		}
	})
}

func TestReader_Store(t *testing.T) {
	t.Parallel()

	t.Run("returns the underlying Store", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		r := store.NewReader(s)
		if r.Store() != s {
			t.Fatalf("Store() should return the wrapped instance")
		}
	})
}

func TestReader_ReadSet(t *testing.T) {
	t.Parallel()

	t.Run("returns the same instance across calls", func(t *testing.T) {
		t.Parallel()
		r := store.NewReader(store.New())
		if a, b := r.ReadSet(), r.ReadSet(); a != b {
			t.Fatalf("ReadSet should be the same instance across calls")
		}
	})
}

func TestReader_NodeQueries(t *testing.T) {
	t.Parallel()

	t.Run("each node-side accessor returns a query that records its own tag", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		r := store.NewReader(s)
		_ = r.Packages().Slice()
		_ = r.Files().Slice()
		_ = r.Imports().Slice()
		_ = r.Structs().Slice()
		_ = r.Interfaces().Slice()
		_ = r.Methods().Slice()
		_ = r.Fields().Slice()
		_ = r.Functions().Slice()
		_ = r.Variables().Slice()
		_ = r.Constants().Slice()
		_ = r.Enums().Slice()
		_ = r.EnumVariants().Slice()
		_ = r.Aliases().Slice()
		want := []string{
			"node:packages", "node:files", "node:imports", "node:structs",
			"node:interfaces", "node:methods", "node:fields", "node:functions",
			"node:variables", "node:constants", "node:enums", "node:enum_variants",
			"node:aliases",
		}
		for _, k := range want {
			if !r.ReadSet().Has(k) {
				t.Fatalf("ReadSet missing key %q", k)
			}
		}
	})
}

func TestReader_EmitQueries(t *testing.T) {
	t.Parallel()

	t.Run("each emit-side accessor returns a query that records its own tag", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Emit().AddPackage(makeUserEmitPackage()))
		r := store.NewReader(s)
		_ = r.EmitPackages().Slice()
		_ = r.EmitFiles().Slice()
		_ = r.EmitImports().Slice()
		_ = r.EmitStructs().Slice()
		_ = r.EmitInterfaces().Slice()
		_ = r.EmitMethods().Slice()
		_ = r.EmitFields().Slice()
		_ = r.EmitFunctions().Slice()
		_ = r.EmitVariables().Slice()
		_ = r.EmitConstants().Slice()
		_ = r.EmitEnums().Slice()
		_ = r.EmitEnumVariants().Slice()
		_ = r.EmitAliases().Slice()
		want := []string{
			"emit:packages", "emit:files", "emit:imports", "emit:structs",
			"emit:interfaces", "emit:methods", "emit:fields", "emit:functions",
			"emit:variables", "emit:constants", "emit:enums", "emit:enum_variants",
			"emit:aliases",
		}
		for _, k := range want {
			if !r.ReadSet().Has(k) {
				t.Fatalf("ReadSet missing key %q", k)
			}
		}
	})
}

// TestNewScopedReader covers the scope-filter contract: node-side
// range queries pre-filter to nodes matching the predicate; direct
// bucket access bypasses the predicate; emit-side queries are
// unfiltered.
func TestNewScopedReader(t *testing.T) {
	t.Parallel()

	t.Run("nil scope is equivalent to NewReader (no filter)", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		r := store.NewScopedReader(s, nil)
		if got := r.Structs().Slice(); len(got) != 2 {
			t.Fatalf("nil scope should yield every struct; got %d", len(got))
		}
	})

	t.Run("scope predicate filters node-side range queries", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		// Match only the User struct.
		r := store.NewScopedReader(s, func(n node.Node) bool {
			st, ok := n.(*node.Struct)
			return ok && st.Name == "User"
		})
		got := r.Structs().Slice()
		if len(got) != 1 || got[0].Name != "User" {
			t.Fatalf("scope should yield only User; got %+v", got)
		}
	})

	t.Run("scope applies to every node-side accessor uniformly", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		// Reject every node — every range query should be empty.
		r := store.NewScopedReader(s, func(node.Node) bool { return false })
		queries := map[string]int{
			"packages":      len(r.Packages().Slice()),
			"files":         len(r.Files().Slice()),
			"imports":       len(r.Imports().Slice()),
			"structs":       len(r.Structs().Slice()),
			"interfaces":    len(r.Interfaces().Slice()),
			"methods":       len(r.Methods().Slice()),
			"fields":        len(r.Fields().Slice()),
			"functions":     len(r.Functions().Slice()),
			"variables":     len(r.Variables().Slice()),
			"constants":     len(r.Constants().Slice()),
			"enums":         len(r.Enums().Slice()),
			"enum_variants": len(r.EnumVariants().Slice()),
			"aliases":       len(r.Aliases().Slice()),
		}
		for kind, count := range queries {
			if count != 0 {
				t.Fatalf("reject-all scope should empty %q query; got %d entries", kind, count)
			}
		}
	})

	t.Run("direct bucket access bypasses the scope filter", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		// Reader rejects everything, but the underlying bucket is
		// untouched — callers needing exact-name lookup must still
		// reach them.
		r := store.NewScopedReader(s, func(node.Node) bool { return false })
		if got := r.Structs().Slice(); len(got) != 0 {
			t.Fatalf("scoped reader should hide structs; got %d", len(got))
		}
		if got := r.Store().Nodes().Structs().Items(); len(got) == 0 {
			t.Fatalf("direct bucket access should bypass scope and yield the underlying entities")
		}
	})

	t.Run("emit-side queries are unfiltered regardless of scope", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Emit().AddPackage(makeUserEmitPackage()))
		r := store.NewScopedReader(s, func(node.Node) bool { return false })
		if got := r.EmitStructs().Slice(); len(got) == 0 {
			t.Fatalf("emit-side range queries should ignore scope; got 0 entries")
		}
	})
}

// BenchmarkReader_Structs prices the read-tracking path against the
// bucket it wraps, over a 500-struct store.
//
// Every plugin query in the framework goes through the Reader, and
// the Reader is not free: it copies the bucket's item slice
// (Bucket.Items), allocates a Query, and on the terminal call takes
// the ReadSet mutex to record one tag. The three variants split that
// cost so it can be attributed rather than guessed:
//
//   - "bucket" — Bucket.Items alone. The floor: one slice copy.
//   - "reader" — the same items through Reader.Structs().Slice(),
//     so the difference against "bucket" is exactly the Query
//     allocation, the second copy Slice makes, and the ReadSet
//     record. That delta is the tax cache-key tracking levies on
//     every query in the system.
//   - "scoped" — the same call on a reader carrying a scope
//     predicate, which inserts a third pass and a third slice. This
//     is what a `-target`-narrowed run pays per query.
//
// The ReadSet does not grow across iterations (Record is idempotent
// on the tag) so the reader variant measures steady-state cost, not
// a set filling up.
func BenchmarkReader_Structs(b *testing.B) {
	b.ReportAllocs()

	s := store.New()
	if err := s.Nodes().AddPackage(makeBenchPackage(500)); err != nil {
		b.Fatalf("AddPackage: %v", err)
	}
	bucket := s.Nodes().Structs()
	reader := store.NewReader(s)
	scoped := store.NewScopedReader(s, func(node.Node) bool { return true })

	b.Run("bucket", func(b *testing.B) {
		b.ReportAllocs()
		total := 0
		for b.Loop() {
			total += len(bucket.Items())
		}
		if total == 0 {
			b.Fatalf("bucket yielded no structs: the benchmark measures nothing")
		}
	})

	b.Run("reader", func(b *testing.B) {
		b.ReportAllocs()
		total := 0
		for b.Loop() {
			total += len(reader.Structs().Slice())
		}
		if total == 0 {
			b.Fatalf("reader yielded no structs: the benchmark measures nothing")
		}
	})

	b.Run("scoped", func(b *testing.B) {
		b.ReportAllocs()
		total := 0
		for b.Loop() {
			total += len(scoped.Structs().Slice())
		}
		if total == 0 {
			b.Fatalf("scoped reader yielded no structs: the benchmark measures nothing")
		}
	})
}
