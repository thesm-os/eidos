// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package detectors_test

import (
	"os"
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

// TestAll_CoversEveryShippedPackage pins the aggregator against the
// directory tree.
//
// [detectors.All] is a hand-maintained list, and a detector package
// added to the tree without a line here ships and never runs. Nothing
// else catches it: the detector's own tests drive it directly and
// pass, `shape/full` composes the aggregator rather than the tree, and
// what a consumer sees is callables the detector would have matched
// carrying no shape stamp — indistinguishable from source it does not
// apply to.
//
// The twin of this check has guarded contracts and mixins since each
// aggregator existed; detectors is the axis that went without one.
//
// Relies on detector names matching their directory names, which every
// shipped detector observes; a future detector whose name legitimately
// differs extends the mapping below rather than weakening the test.
func TestAll_CoversEveryShippedPackage(t *testing.T) {
	t.Parallel()
	registered := make(map[string]struct{})
	for _, d := range detectors.All() {
		registered[d.Name] = struct{}{}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "internal" {
			continue
		}
		if _, ok := registered[e.Name()]; !ok {
			t.Errorf("package %q is shipped but not registered in detectors.All()", e.Name())
		}
	}
}
