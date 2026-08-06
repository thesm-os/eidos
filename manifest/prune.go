// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package manifest

import (
	"strings"

	"go.thesmos.sh/eidos/emit"
)

// OrphanReason records why a prior entry is no longer claimed.
// The two reasons are reached by disjoint routes and carry
// different risk, so consumers act on them separately rather than
// on a single "orphan" verdict.
type OrphanReason int

const (
	// ReasonUnclaimed means the source package still loads, but
	// nothing in it emits this target any more — the directive was
	// removed, the type deleted, or the plugin dropped from the
	// pipeline. The run examined the package and did not claim the
	// file, which is direct evidence.
	ReasonUnclaimed OrphanReason = iota

	// ReasonSourceGone means the source package no longer exists in
	// the module at all, so the run could not have loaded it and
	// [PruneOptions.Scope] says nothing either way.
	//
	// This is the case Scope alone cannot decide. Scope answers "did
	// this run examine that package", which is false both for a
	// package the run was never asked to look at (a narrow
	// `./sub/...` invocation) and for one that has been deleted.
	// Treating the two alike is correct for the first — a narrow run
	// has no authority over the rest of the tree — and leaves the
	// second permanently unreachable: deleting a source package is
	// precisely the act that strands its outputs, and it was the one
	// case prune could never see.
	ReasonSourceGone
)

// String renders the reason for diagnostics.
func (r OrphanReason) String() string {
	switch r {
	case ReasonUnclaimed:
		return "unclaimed"
	case ReasonSourceGone:
		return "source-gone"
	default:
		return "orphan_reason(?)"
	}
}

// Orphan is a prior [Output] the current run no longer claims,
// paired with why.
type Orphan struct {
	Output

	// Reason distinguishes an entry whose package was examined and
	// did not claim it from one whose package is gone.
	Reason OrphanReason
}

// PruneOptions carries the inputs [PruneAll] classifies against.
type PruneOptions struct {
	// Emitted is the set of targets the current run wrote.
	Emitted map[emit.Target]struct{}

	// Scope is the set of source-package import paths the current
	// run loaded.
	Scope map[string]struct{}

	// PipelineID scopes classification to one pipeline's entries.
	PipelineID string

	// GoneSources holds import paths the caller has established no
	// longer exist in the module being generated. Empty reproduces
	// the pre-existing behaviour exactly, so a caller that does not
	// supply it loses nothing and risks nothing.
	//
	// A set rather than a resolver interface on purpose. This
	// package is pure data and set logic — that is why its
	// classification is exhaustively testable without a filesystem —
	// and resolving an import path to a directory needs the module
	// path, the module root, and the build context. That work
	// belongs at the edge, in the CLI, which already walks up to
	// go.mod. Handing the answer in rather than the means to compute
	// it keeps the decision here total and reproducible.
	GoneSources map[string]struct{}
}

// PruneAll returns every orphan entry in prev, classified by
// [OrphanReason]. Order is preserved from prev so iteration is
// deterministic for the same input.
//
// An entry belongs to this pipeline when [Output.PipelineID]
// matches; entries from other pipelines are off-limits at every
// stage, since their owning pipeline manages their lifecycle. A
// target the current run wrote is never an orphan.
//
// Among the remainder:
//
//   - In scope — the run loaded the source package and did not
//     re-emit the target: [ReasonUnclaimed].
//   - Out of scope but named in [PruneOptions.GoneSources]:
//     [ReasonSourceGone].
//   - Out of scope and not named: not an orphan. A narrow run has
//     no authority over packages it never looked at.
//
// A nil prev, an empty PipelineID, a nil Scope, or a nil Emitted
// set all return nil (nothing identifiable as an orphan).
func PruneAll(prev *Manifest, opts PruneOptions) []Orphan {
	if prev == nil || opts.PipelineID == "" || opts.Scope == nil || opts.Emitted == nil {
		return nil
	}
	var out []Orphan
	for _, o := range prev.Outputs {
		if o.PipelineID != opts.PipelineID {
			continue
		}
		if _, claimed := opts.Emitted[o.Target]; claimed {
			continue
		}
		switch {
		case inScope(o.Target.ImportPath, opts.Scope):
			out = append(out, Orphan{Output: o, Reason: ReasonUnclaimed})
		case isGone(o.Target.ImportPath, opts.GoneSources):
			out = append(out, Orphan{Output: o, Reason: ReasonSourceGone})
		}
	}
	return out
}

// isGone reports whether the import path is one the caller
// established no longer exists. Applies the same `<pkg>_test`
// auto-shift tolerance as [inScope]: the framework routes test
// outputs into a sibling `_test` import path that never had a
// directory of its own, so a gone source package must claim its
// shifted outputs too or they stay stranded.
func isGone(path string, gone map[string]struct{}) bool {
	if path == "" || gone == nil {
		return false
	}
	if _, ok := gone[path]; ok {
		return true
	}
	if stripped, ok := strings.CutSuffix(path, "_test"); ok {
		if _, hit := gone[stripped]; hit {
			return true
		}
	}
	return false
}

// Prune returns the list of orphan [Output] entries in prev —
// entries the current run had authority over but did NOT
// re-emit. The CLI's "eidos prune" command deletes the
// corresponding files; the "eidos check" gate reports them as
// drift without deleting.
//
// Reports only [ReasonUnclaimed] orphans. Callers that can
// establish which source packages have been deleted use [PruneAll]
// with [PruneOptions.GoneSources] and get both classes.
//
// An entry is an orphan iff all three conditions hold:
//
//  1. [Output.PipelineID] equals pipelineID. Other pipelines'
//     entries are off-limits; their owning pipeline manages
//     their lifecycle.
//  2. The entry's [emit.Target.ImportPath] (or its `<pkg>_test`
//     auto-shift variant) is in scope — i.e. the current
//     pipeline loaded that source package. Narrow runs that did
//     not load the source package don't get to call its outputs
//     orphans.
//  3. The entry's Target is NOT in the emitted set — i.e. the
//     current run did not write to that target. Targets that
//     the current run produced are not orphans.
//
// Order is preserved from prev so iteration is deterministic
// for the same input. A nil prev manifest, an empty pipelineID,
// a nil scope, or a nil emitted set all return nil (nothing
// identifiable as an orphan).
func Prune(
	prev *Manifest,
	emitted map[emit.Target]struct{},
	scope map[string]struct{},
	pipelineID string,
) []Output {
	orphans := PruneAll(prev, PruneOptions{
		Emitted:    emitted,
		Scope:      scope,
		PipelineID: pipelineID,
	})
	if len(orphans) == 0 {
		return nil
	}
	out := make([]Output, 0, len(orphans))
	for _, o := range orphans {
		out = append(out, o.Output)
	}
	return out
}

// inScope reports whether the supplied import path lives in the
// scope set — exact match plus the framework's `<pkg>_test`
// auto-shift variant. Empty input is out of scope.
func inScope(path string, scope map[string]struct{}) bool {
	if path == "" {
		return false
	}
	if _, ok := scope[path]; ok {
		return true
	}
	if stripped, ok := strings.CutSuffix(path, "_test"); ok {
		if _, hit := scope[stripped]; hit {
			return true
		}
	}
	return false
}
