// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package mixins_test

import (
	"os"
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

// TestAll_CoversEveryShippedPackage pins the catalog against the
// directory: every mixin sub-package must appear in [mixins.All].
//
// Nothing else can catch the omission. A package left out of the
// aggregator still compiles, passes its own tests, and is absent from
// every pipeline built on the full catalog — where its directive is
// then reported as an unregistered name. The registration is a line
// in a hand-maintained list, and this is the check that the list and
// the tree agree.
//
// Relies on mixin names matching their directory names, which every
// shipped mixin observes; a future mixin whose name legitimately
// differs extends the mapping below rather than weakening the test.
func TestAll_CoversEveryShippedPackage(t *testing.T) {
	t.Parallel()
	registered := make(map[string]struct{})
	for _, m := range mixins.All() {
		registered[m.Name] = struct{}{}
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
			t.Errorf("package %q is shipped but not registered in mixins.All()", e.Name())
		}
	}
}
