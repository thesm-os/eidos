// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package contracts_test

import (
	"sort"
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
