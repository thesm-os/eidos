// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package contracts_test

import (
	"os"
	"sort"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts"
)

// TestAll_NonEmpty pins that the convenience aggregator surfaces
// at least one contract — guards against an accidental empty
// import block masking a broken aggregator.
func TestAll_NonEmpty(t *testing.T) {
	t.Parallel()
	if got := contracts.All(); len(got) == 0 {
		t.Fatal("contracts.All() returned empty slice; expected the shipped catalog")
	}
}

// TestAll_Alphabetical pins the documented ordering of [All]'s
// return value so consumers (test goldens, diagnostic dumps,
// catalog walkers) can rely on stable iteration.
func TestAll_Alphabetical(t *testing.T) {
	t.Parallel()
	all := contracts.All()
	names := make([]string, len(all))
	for i, c := range all {
		names[i] = c.Name
	}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("contracts.All() is not alphabetical by Name; got %v", names)
	}
}

// TestAll_UniqueNames pins that no two entries collide on Name —
// a duplicate would silently shadow the second entry's
// registration on the umbrella plugin.
func TestAll_UniqueNames(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{}, len(contracts.All()))
	for _, c := range contracts.All() {
		if _, dup := seen[c.Name]; dup {
			t.Fatalf("contracts.All() contains duplicate Name %q", c.Name)
		}
		seen[c.Name] = struct{}{}
	}
}

// TestAll_FreshSlice pins the documented allocation behaviour —
// callers mutating the returned slice must not affect future
// invocations.
func TestAll_FreshSlice(t *testing.T) {
	t.Parallel()
	first := contracts.All()
	if len(first) == 0 {
		t.Fatal("nothing to mutate; All() returned empty")
	}
	original := first[0].Name
	first[0].Name = "_mutated_"
	if second := contracts.All(); second[0].Name == "_mutated_" {
		t.Fatalf("All() returned a shared slice; mutating first[0].Name leaked into the next call")
	}
	_ = original
}

// TestAll_CoversEveryShippedPackage pins the catalog against the
// directory: every contract sub-package must appear in
// [contracts.All].
//
// Nothing else can catch the omission — a package left out of the
// aggregator compiles, passes its own tests, and is absent from every
// pipeline built on the full catalog, where its directive is then
// reported as an unregistered name. The mixins aggregator carries the
// same guard.
//
// Contract names may hyphenate where directories cannot
// (`batch-writer` in `batchwriter/`), so names are compared with
// hyphens stripped. One package deliberately diverges further:
// `writethroughcache` registers the contract `cache` — the package
// names the pairing, the directive names the thing declared — and
// the consumer's corpus stamps it under that name. A future
// divergence extends the map below, so it is a recorded decision
// rather than drift.
func TestAll_CoversEveryShippedPackage(t *testing.T) {
	t.Parallel()
	aliases := map[string]string{"writethroughcache": "cache"}
	registered := make(map[string]struct{})
	for _, c := range contracts.All() {
		registered[strings.ReplaceAll(c.Name, "-", "")] = struct{}{}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "internal" {
			continue
		}
		want := e.Name()
		if alias, ok := aliases[want]; ok {
			want = alias
		}
		if _, ok := registered[want]; !ok {
			t.Errorf("package %q is shipped but not registered in contracts.All()", e.Name())
		}
	}
}
