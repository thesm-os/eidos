// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ids_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors"
	"go.thesmos.sh/eidos/plugins/annotator/shape/ids"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins"
)

// names lifts a registered catalog's names into the id type for
// comparison against the exported list.
func names[T any](items []T, name func(T) string) []ids.Name {
	out := make([]ids.Name, 0, len(items))
	for _, item := range items {
		out = append(out, ids.Name(name(item)))
	}
	slices.Sort(out)
	return out
}

// TestIDs_MatchTheRegisteredCatalog is the guard that makes this
// package safe to rely on.
//
// A re-export that can fall behind is worse than the string literals
// it replaces: the literal is at least obviously unverified, while a
// constant carries the authority of the compiler having checked
// something — and what the compiler checks is that the referenced
// package exists, not that every registered shape has a constant. A
// shape added to the catalog and forgotten here is the silence.
func TestIDs_MatchTheRegisteredCatalog(t *testing.T) {
	t.Parallel()

	t.Run("every registered detector has an id", func(t *testing.T) {
		t.Parallel()
		want := names(detectors.All(), func(d shape.Detector) string { return d.Name })
		if got := ids.Detectors(); !slices.Equal(got, want) {
			t.Errorf("Detectors() has drifted from detectors.All();\n  missing: %v\n  extra: %v",
				missing(want, got), missing(got, want))
		}
	})

	t.Run("every registered contract has an id", func(t *testing.T) {
		t.Parallel()
		want := names(contracts.All(), func(c shape.Contract) string { return c.Name })
		if got := ids.Contracts(); !slices.Equal(got, want) {
			t.Errorf("Contracts() has drifted from contracts.All();\n  missing: %v\n  extra: %v",
				missing(want, got), missing(got, want))
		}
	})

	t.Run("every registered mixin has an id", func(t *testing.T) {
		t.Parallel()
		want := names(mixins.All(), func(m shape.Mixin) string { return m.Name })
		if got := ids.Mixins(); !slices.Equal(got, want) {
			t.Errorf("Mixins() has drifted from mixins.All();\n  missing: %v\n  extra: %v",
				missing(want, got), missing(got, want))
		}
	})

	t.Run("the lists are non-empty", func(t *testing.T) {
		t.Parallel()
		// Guard the guard: two empty slices compare equal, so a
		// refactor emptying both sides would leave the drift checks
		// reporting green over nothing.
		for label, got := range map[string][]ids.Name{
			"Detectors": ids.Detectors(),
			"Contracts": ids.Contracts(),
			"Mixins":    ids.Mixins(),
		} {
			if len(got) == 0 {
				t.Errorf("%s() is empty; the drift check above would be vacuous", label)
			}
		}
	})
}

// TestIDs_Untyped pins that a constant still passes where the shape
// API takes a string, which is why the constants carry no type.
func TestIDs_Untyped(t *testing.T) {
	t.Parallel()
	// Compiles only while the constants stay untyped: a typed Name
	// would need a conversion at every call site in every consumer.
	if key := shape.MixinParamKey(ids.MixinTTL, "notfound"); key.Name() == "" {
		t.Fatal("MixinParamKey returned an unnamed key")
	}
	if key := shape.ContractRoleKey(ids.ContractCursor); key.Name() == "" {
		t.Fatal("ContractRoleKey returned an unnamed key")
	}
}

// TestIDs_NamesAreNotPackageNames pins the two places where reading
// the constant is the only way to learn the registered spelling.
func TestIDs_NamesAreNotPackageNames(t *testing.T) {
	t.Parallel()
	if ids.ContractCache != "cache" {
		t.Errorf("ContractCache = %q, want cache — the writethroughcache package registers the short name",
			ids.ContractCache)
	}
	if ids.ContractBatchWriter != "batch-writer" {
		t.Errorf("ContractBatchWriter = %q, want the hyphenated form", ids.ContractBatchWriter)
	}
}

// missing returns the elements of want that got does not contain.
func missing(want, got []ids.Name) []ids.Name {
	var out []ids.Name
	for _, w := range want {
		if !slices.Contains(got, w) {
			out = append(out, w)
		}
	}
	return out
}
