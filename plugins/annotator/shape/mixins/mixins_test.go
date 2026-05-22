// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package mixins_test

import (
	"sort"
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins"
)

// TestAll_NonEmpty pins that the aggregator surfaces at least one
// mixin.
func TestAll_NonEmpty(t *testing.T) {
	t.Parallel()
	if got := mixins.All(); len(got) == 0 {
		t.Fatal("mixins.All() returned empty slice; expected the shipped catalog")
	}
}

// TestAll_Alphabetical pins the documented ordering.
func TestAll_Alphabetical(t *testing.T) {
	t.Parallel()
	all := mixins.All()
	names := make([]string, len(all))
	for i, m := range all {
		names[i] = m.Name
	}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("mixins.All() is not alphabetical by Name; got %v", names)
	}
}

// TestAll_UniqueNames pins that no two entries collide on Name.
func TestAll_UniqueNames(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{}, len(mixins.All()))
	for _, m := range mixins.All() {
		if _, dup := seen[m.Name]; dup {
			t.Fatalf("mixins.All() contains duplicate Name %q", m.Name)
		}
		seen[m.Name] = struct{}{}
	}
}

// TestAll_FreshSlice pins the documented allocation behaviour.
func TestAll_FreshSlice(t *testing.T) {
	t.Parallel()
	first := mixins.All()
	if len(first) == 0 {
		t.Fatal("nothing to mutate; All() returned empty")
	}
	first[0].Name = "_mutated_"
	if second := mixins.All(); second[0].Name == "_mutated_" {
		t.Fatalf("All() returned a shared slice; mutating first[0].Name leaked into the next call")
	}
}
