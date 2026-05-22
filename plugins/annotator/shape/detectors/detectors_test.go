// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package detectors_test

import (
	"sort"
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors"
)

// TestAll_NonEmpty pins that the aggregator surfaces at least one
// detector.
func TestAll_NonEmpty(t *testing.T) {
	t.Parallel()
	if got := detectors.All(); len(got) == 0 {
		t.Fatal("detectors.All() returned empty slice; expected the shipped catalog")
	}
}

// TestAll_Alphabetical pins the documented ordering. Note: the
// umbrella shape plugin dispatches by [shape.Detector.Priority],
// not slice position; the alphabetical order here is for stable
// diagnostics and test-output readability.
func TestAll_Alphabetical(t *testing.T) {
	t.Parallel()
	all := detectors.All()
	names := make([]string, len(all))
	for i, d := range all {
		names[i] = d.Name
	}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("detectors.All() is not alphabetical by Name; got %v", names)
	}
}

// TestAll_UniqueNames pins that no two entries collide on Name.
func TestAll_UniqueNames(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{}, len(detectors.All()))
	for _, d := range detectors.All() {
		if _, dup := seen[d.Name]; dup {
			t.Fatalf("detectors.All() contains duplicate Name %q", d.Name)
		}
		seen[d.Name] = struct{}{}
	}
}

// TestAll_FreshSlice pins the documented allocation behaviour.
func TestAll_FreshSlice(t *testing.T) {
	t.Parallel()
	first := detectors.All()
	if len(first) == 0 {
		t.Fatal("nothing to mutate; All() returned empty")
	}
	first[0].Name = "_mutated_"
	if second := detectors.All(); second[0].Name == "_mutated_" {
		t.Fatalf("All() returned a shared slice; mutating first[0].Name leaked into the next call")
	}
}
