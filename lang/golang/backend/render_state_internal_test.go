// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"maps"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/lang/golang"
)

// TestReservedFuncNames_MirroredByConformanceSuite pins the two
// reserved-funcmap sets together.
//
// [plugintest] cannot import this package — it is the language-neutral
// conformance suite, and coupling every plugin author's test run to
// one backend would defeat its purpose — so it carries a hand-written
// copy of the names below. A copy with nothing watching it rots: a
// name added here and not there is a name the suite calls legal and
// [mergeExtensions] rejects at merge time, which aborts the whole run
// before a single file is written.
//
// This test is the thing watching it, and it lives here rather than in
// plugintest because only this side can read the authoritative set.
// Internal rather than black-box for the same reason:
// [reservedFuncNames] is unexported and should stay that way.
func TestReservedFuncNames_MirroredByConformanceSuite(t *testing.T) {
	t.Parallel()

	// Guard the guard. Two empty sets compare equal, so a refactor that
	// emptied either side would leave the drift check reporting green
	// over nothing — the exact failure mode it exists to prevent.
	t.Run("both sets are non-empty", func(t *testing.T) {
		t.Parallel()
		if n := len(reservedFuncNames()); n == 0 {
			t.Errorf("the backend reserves no funcmap names; the drift check below would be vacuous")
		}
		if n := len(plugintest.ReservedTemplateFuncNames()); n == 0 {
			t.Errorf("the conformance mirror is empty; the drift check below would be vacuous")
		}
	})

	t.Run("the conformance mirror matches the backend's reserved set", func(t *testing.T) {
		t.Parallel()

		want := slices.Sorted(maps.Keys(reservedFuncNames()))
		got := plugintest.ReservedTemplateFuncNames()
		if slices.Equal(got, want) {
			return
		}
		t.Errorf(
			"plugintest.ReservedTemplateFuncNames() has drifted from the backend's reserved set;\n"+
				"  reserved here but unchecked by the suite: %v\n"+
				"  checked by the suite but not reserved here: %v\n"+
				"update reservedFuncNames in eidostest/plugintest/framework.go to match",
			missingFrom(want, got), missingFrom(got, want),
		)
	})
}

// TestOverrideableFuncNames_MirroredByConformanceSuite is the twin of
// the reserved drift check, for the other half of what the backend
// brings.
//
// It exists because the reserved check could not have caught the gap
// it guards: that one compares the reserved list against the reserved
// set, and the overrideable entries appear in neither. A plugin
// calling `camel` — provided at render, documented as available —
// failed its own conformance suite against a template that renders
// correctly, and nothing in this file objected.
func TestOverrideableFuncNames_MirroredByConformanceSuite(t *testing.T) {
	t.Parallel()

	t.Run("both sets are non-empty", func(t *testing.T) {
		t.Parallel()
		if n := len(backendOwnedExtras()); n == 0 {
			t.Errorf("the backend owns no overrideable names; the drift check below would be vacuous")
		}
		if n := len(plugintest.OverrideableTemplateFuncNames()); n == 0 {
			t.Errorf("the conformance mirror is empty; the drift check below would be vacuous")
		}
	})

	t.Run("the conformance mirror matches the backend's own overrideable set", func(t *testing.T) {
		t.Parallel()

		// extrasFuncMap layers golang.FuncMap on top of the entries
		// this package owns, and only the owned half is compared
		// here. The shared bundle has a mirror of its own, guarded
		// beside it by TestFuncMap_MirroredByConformanceSuite in
		// lang/golang — so folding it in would assert the same names
		// twice and report both when one moved.
		want := slices.Sorted(maps.Keys(backendOwnedExtras()))
		got := plugintest.OverrideableTemplateFuncNames()
		if slices.Equal(got, want) {
			return
		}
		t.Errorf(
			"plugintest.OverrideableTemplateFuncNames() has drifted from the backend's "+
				"overrideable set;\n"+
				"  registered here but unseeded by the suite: %v\n"+
				"  seeded by the suite but not registered here: %v\n"+
				"update overrideableFuncNames in eidostest/plugintest/framework.go to match",
			missingFrom(want, got), missingFrom(got, want),
		)
	})
}

// backendOwnedExtras returns the overrideable entries this package
// declares itself — extrasFuncMap minus the shared lang/golang bundle
// it layers on top.
func backendOwnedExtras() map[string]any {
	out := maps.Clone(extrasFuncMap())
	for name := range golang.FuncMap() {
		delete(out, name)
	}
	return out
}

// missingFrom returns the elements of a that b does not contain, in
// a's order, so the drift report names each direction separately
// instead of dumping two full sets for the reader to diff by eye.
func missingFrom(a, b []string) []string {
	var out []string
	for _, v := range a {
		if !slices.Contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}
