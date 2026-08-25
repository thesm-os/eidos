// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ids_test

import (
	"slices"
	"strings"
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

// params lifts a registered catalog's declared parameter keys into
// the exported pair form, sorted the way the accessors return them.
func params[T any](items []T, owner func(T) string, keys func(T) []shape.Param) []ids.Param {
	var out []ids.Param
	for _, item := range items {
		for _, p := range keys(item) {
			out = append(out, ids.Param{Owner: ids.Name(owner(item)), Key: p.Key})
		}
	}
	slices.SortFunc(out, func(a, b ids.Param) int {
		if a.Owner != b.Owner {
			return strings.Compare(string(a.Owner), string(b.Owner))
		}
		return strings.Compare(a.Key, b.Key)
	})
	return out
}

// The parameter constants cover exactly what the catalog accepts.
//
// The guard the name constants already have, for the other half of a
// directive. A param key reaches a stamp through
// [shape.ContractParamKey] or [shape.MixinParamKey], neither of which
// can tell a key the owner declares from one it has never heard of —
// so a constant missing here sends a caller back to a literal, and a
// constant left behind after a key is renamed compiles forever while
// matching nothing.
func TestIDs_ParamsMatchTheRegisteredCatalog(t *testing.T) {
	t.Parallel()

	t.Run("every contract parameter has an id", func(t *testing.T) {
		t.Parallel()
		want := params(contracts.All(),
			func(c shape.Contract) string { return c.Name },
			func(c shape.Contract) []shape.Param { return c.Params })
		if got := ids.ContractParams(); !slices.Equal(got, want) {
			t.Errorf("ContractParams() has drifted from contracts.All();\n  missing: %v\n  extra: %v",
				missingParams(want, got), missingParams(got, want))
		}
	})

	t.Run("every mixin parameter has an id", func(t *testing.T) {
		t.Parallel()
		want := params(mixins.All(),
			func(m shape.Mixin) string { return m.Name },
			func(m shape.Mixin) []shape.Param { return m.Params })
		if got := ids.MixinParams(); !slices.Equal(got, want) {
			t.Errorf("MixinParams() has drifted from mixins.All();\n  missing: %v\n  extra: %v",
				missingParams(want, got), missingParams(got, want))
		}
	})
}

// missingParams returns the entries of want that are absent from got.
func missingParams(want, got []ids.Param) []ids.Param {
	var out []ids.Param
	for _, w := range want {
		if !slices.Contains(got, w) {
			out = append(out, w)
		}
	}
	return out
}
