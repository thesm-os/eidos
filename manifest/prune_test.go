// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package manifest_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
)

func TestPrune(t *testing.T) {
	t.Parallel()

	scope := map[string]struct{}{"example.com/a": {}}

	t.Run("returns entries the current run did not re-emit", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("run-2")
		freshEntry := manifest.Output{
			Target:     targetAtPath("a", "fresh.go", "example.com/a"),
			PipelineID: "p",
		}
		staleEntry := manifest.Output{
			Target:     targetAtPath("a", "stale.go", "example.com/a"),
			PipelineID: "p",
		}
		prev.Add(freshEntry)
		prev.Add(staleEntry)
		// Current run emitted only the fresh target.
		emitted := map[emit.Target]struct{}{freshEntry.Target: {}}

		got := manifest.Prune(prev, emitted, scope, "p")
		if len(got) != 1 || got[0].Target.Filename != "stale.go" {
			t.Fatalf("Prune should return only the un-claimed entry; got %+v", got)
		}
	})

	t.Run("scope filter excludes out-of-scope entries", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("run-2")
		prev.Add(manifest.Output{
			Target:     targetAtPath("b", "stale.go", "example.com/b"),
			PipelineID: "p",
		})
		// Un-emitted, but its import path is outside the scope set —
		// current run did not load this package, so prune must not
		// consider it an orphan.
		emitted := map[emit.Target]struct{}{}
		if got := manifest.Prune(prev, emitted, scope, "p"); got != nil {
			t.Errorf("out-of-scope entry must not be returned; got %+v", got)
		}
	})

	t.Run("test-shifted import path matches non-test scope entry", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("run-2")
		prev.Add(manifest.Output{
			Target:     targetAtPath("a", "x_test.go", "example.com/a_test"),
			PipelineID: "p",
		})
		emitted := map[emit.Target]struct{}{}
		got := manifest.Prune(prev, emitted, scope, "p")
		if len(got) != 1 {
			t.Fatalf("`<pkg>_test` auto-shift entry must match non-test scope; got %+v", got)
		}
	})

	t.Run("PipelineID mismatch excludes the entry", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("run-2")
		prev.Add(manifest.Output{
			Target:     targetAtPath("a", "x_bench.go", "example.com/a"),
			PipelineID: "bench",
		})
		// Un-emitted, in scope, but owned by a different pipeline —
		// prune for "suite" must not touch it.
		emitted := map[emit.Target]struct{}{}
		if got := manifest.Prune(prev, emitted, scope, "suite"); got != nil {
			t.Errorf("other-pipeline entry must not be returned; got %+v", got)
		}
	})

	t.Run("empty pipelineID returns nil (refuses to scope without identity)", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("run-2")
		prev.Add(manifest.Output{
			Target: targetAtPath("a", "x.go", "example.com/a"),
		})
		emitted := map[emit.Target]struct{}{}
		if got := manifest.Prune(prev, emitted, scope, ""); got != nil {
			t.Errorf("empty pipelineID must not return candidates; got %+v", got)
		}
	})

	t.Run("nil prev / nil scope / nil emitted all return nil", func(t *testing.T) {
		t.Parallel()
		if got := manifest.Prune(nil, map[emit.Target]struct{}{}, scope, "p"); got != nil {
			t.Errorf("nil prev must return nil; got %+v", got)
		}
		if got := manifest.Prune(manifest.New("r"), map[emit.Target]struct{}{}, nil, "p"); got != nil {
			t.Errorf("nil scope must return nil; got %+v", got)
		}
		if got := manifest.Prune(manifest.New("r"), nil, scope, "p"); got != nil {
			t.Errorf("nil emitted must return nil; got %+v", got)
		}
	})

	t.Run("preserves manifest order in the returned slice", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("run-2")
		prev.Add(manifest.Output{
			Target:     targetAtPath("a", "first.go", "example.com/a"),
			PipelineID: "p",
		})
		prev.Add(manifest.Output{
			Target:     targetAtPath("a", "second.go", "example.com/a"),
			PipelineID: "p",
		})
		emitted := map[emit.Target]struct{}{}
		got := manifest.Prune(prev, emitted, scope, "p")
		if len(got) != 2 ||
			got[0].Target.Filename != "first.go" ||
			got[1].Target.Filename != "second.go" {

			t.Fatalf("Prune must preserve order; got %+v", got)
		}
	})
}

// TestPruneAll_SourceGone covers the classification Scope alone
// cannot make.
//
// Scope answers "did this run load that package", which is false both
// for a package a narrow run was never asked to examine and for one
// that has been deleted. Prune treated the two alike, so deleting a
// source package — the act that actually strands an output — produced
// an entry no invocation could ever reach.
func TestPruneAll_SourceGone(t *testing.T) {
	t.Parallel()

	scope := map[string]struct{}{"example.com/a": {}}

	// gonePrev builds a manifest holding one entry for a package that
	// is out of scope, so only GoneSources can classify it.
	gonePrev := func() (*manifest.Manifest, map[emit.Target]struct{}) {
		prev := manifest.New("run-2")
		prev.Add(manifest.Output{
			Target:     targetAtPath("b", "gone.go", "example.com/b"),
			PipelineID: "p",
		})
		return prev, map[emit.Target]struct{}{}
	}

	t.Run("a deleted source package is classified ReasonSourceGone", func(t *testing.T) {
		t.Parallel()
		prev, emitted := gonePrev()
		got := manifest.PruneAll(prev, manifest.PruneOptions{
			Emitted: emitted, Scope: scope, PipelineID: "p",
			GoneSources: map[string]struct{}{"example.com/b": {}},
		})
		if len(got) != 1 {
			t.Fatalf("expected one orphan; got %+v", got)
		}
		if got[0].Reason != manifest.ReasonSourceGone {
			t.Fatalf("Reason = %v, want ReasonSourceGone", got[0].Reason)
		}
	})

	t.Run("without GoneSources the same entry is not an orphan", func(t *testing.T) {
		t.Parallel()
		prev, emitted := gonePrev()
		got := manifest.PruneAll(prev, manifest.PruneOptions{
			Emitted: emitted, Scope: scope, PipelineID: "p",
		})
		if len(got) != 0 {
			t.Fatalf("a narrow run must not orphan what it never examined; got %+v", got)
		}
	})

	t.Run("an in-scope entry stays ReasonUnclaimed", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("run-2")
		prev.Add(manifest.Output{
			Target:     targetAtPath("a", "stale.go", "example.com/a"),
			PipelineID: "p",
		})
		got := manifest.PruneAll(prev, manifest.PruneOptions{
			Emitted: map[emit.Target]struct{}{}, Scope: scope, PipelineID: "p",
			// Deliberately also named gone: scope is checked first, so
			// a package that both loaded and appears here is unclaimed,
			// not gone. Direct evidence outranks the inference.
			GoneSources: map[string]struct{}{"example.com/a": {}},
		})
		if len(got) != 1 || got[0].Reason != manifest.ReasonUnclaimed {
			t.Fatalf("in-scope entry must classify as unclaimed; got %+v", got)
		}
	})

	t.Run("a re-emitted target is never an orphan even when named gone", func(t *testing.T) {
		t.Parallel()
		prev, _ := gonePrev()
		got := manifest.PruneAll(prev, manifest.PruneOptions{
			Emitted:     map[emit.Target]struct{}{prev.Outputs[0].Target: {}},
			Scope:       scope,
			PipelineID:  "p",
			GoneSources: map[string]struct{}{"example.com/b": {}},
		})
		if len(got) != 0 {
			t.Fatalf("a claimed target must never be pruned; got %+v", got)
		}
	})

	t.Run("another pipeline's entry is never classified", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("run-2")
		prev.Add(manifest.Output{
			Target:     targetAtPath("b", "gone.go", "example.com/b"),
			PipelineID: "other",
		})
		got := manifest.PruneAll(prev, manifest.PruneOptions{
			Emitted: map[emit.Target]struct{}{}, Scope: scope, PipelineID: "p",
			GoneSources: map[string]struct{}{"example.com/b": {}},
		})
		if len(got) != 0 {
			t.Fatalf("other pipelines own their lifecycle; got %+v", got)
		}
	})

	t.Run("a _test-shifted output follows its real package", func(t *testing.T) {
		t.Parallel()
		// The framework routes test outputs to a sibling `_test`
		// import path that never had a directory, so the probe names
		// the real package. Without the shift these outputs stay
		// stranded exactly as before.
		prev := manifest.New("run-2")
		prev.Add(manifest.Output{
			Target:     targetAtPath("b", "gone_test.go", "example.com/b_test"),
			PipelineID: "p",
		})
		got := manifest.PruneAll(prev, manifest.PruneOptions{
			Emitted: map[emit.Target]struct{}{}, Scope: scope, PipelineID: "p",
			GoneSources: map[string]struct{}{"example.com/b": {}},
		})
		if len(got) != 1 || got[0].Reason != manifest.ReasonSourceGone {
			t.Fatalf("a _test-shifted output must follow its package; got %+v", got)
		}
	})

	t.Run("Prune reports only unclaimed orphans", func(t *testing.T) {
		t.Parallel()
		// The narrow entry point must not gain a behaviour its
		// callers never opted into.
		prev, emitted := gonePrev()
		prev.Add(manifest.Output{
			Target:     targetAtPath("a", "stale.go", "example.com/a"),
			PipelineID: "p",
		})
		got := manifest.Prune(prev, emitted, scope, "p")
		if len(got) != 1 || got[0].Target.Filename != "stale.go" {
			t.Fatalf("Prune must stay scope-only; got %+v", got)
		}
	})
}

func TestPruneAll_NotAnOrphan(t *testing.T) {
	t.Parallel()

	// entryAt builds a manifest holding one unclaimed entry at the
	// supplied import path, so each case below differs only in the
	// path and the options it is classified against.
	entryAt := func(importPath string) (*manifest.Manifest, map[emit.Target]struct{}) {
		prev := manifest.New("run-3")
		prev.Add(manifest.Output{
			Target:     targetAtPath("c", "out.go", importPath),
			PipelineID: "p",
		})
		return prev, map[emit.Target]struct{}{}
	}

	t.Run("an out-of-scope path absent from GoneSources is not an orphan", func(t *testing.T) {
		t.Parallel()
		// Distinct from the nil-GoneSources case: here the caller
		// did establish which packages are gone, and this is not
		// one of them. Absence from a populated set must read as
		// "still exists", not as "unknown".
		prev, emitted := entryAt("example.com/c")
		got := manifest.PruneAll(prev, manifest.PruneOptions{
			Emitted:     emitted,
			Scope:       map[string]struct{}{"example.com/a": {}},
			PipelineID:  "p",
			GoneSources: map[string]struct{}{"example.com/b": {}},
		})
		if len(got) != 0 {
			t.Fatalf("a package absent from GoneSources still exists; got %+v", got)
		}
	})

	t.Run("an entry carrying no import path is not an orphan", func(t *testing.T) {
		t.Parallel()
		// An output the router never bound to a source package
		// cannot be attributed to one, so neither scope nor
		// gone-ness can claim it.
		prev, emitted := entryAt("")
		got := manifest.PruneAll(prev, manifest.PruneOptions{
			Emitted:     emitted,
			Scope:       map[string]struct{}{"example.com/a": {}},
			PipelineID:  "p",
			GoneSources: map[string]struct{}{"example.com/b": {}},
		})
		if len(got) != 0 {
			t.Fatalf("a pathless entry is unattributable; got %+v", got)
		}
	})

	t.Run("an empty scope entry is not claimed by an empty scope set", func(t *testing.T) {
		t.Parallel()
		// Guards the pairing of the two empty-path guards: an
		// empty path must not match an empty-string key were one
		// ever to enter the scope set.
		prev, emitted := entryAt("")
		got := manifest.PruneAll(prev, manifest.PruneOptions{
			Emitted:     emitted,
			Scope:       map[string]struct{}{"": {}},
			PipelineID:  "p",
			GoneSources: map[string]struct{}{"": {}},
		})
		if len(got) != 0 {
			t.Fatalf("the empty path must never match; got %+v", got)
		}
	})
}

func TestOrphanReason_String(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		r    manifest.OrphanReason
		want string
	}{
		{"Unclaimed", manifest.ReasonUnclaimed, "unclaimed"},
		{"SourceGone", manifest.ReasonSourceGone, "source-gone"},
		{"unknown carries a marker", manifest.OrphanReason(99), "orphan_reason(?)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.r.String(); got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}
