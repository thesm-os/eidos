// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipelinetest

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/sink"
)

// Pipeline is the test-tuned wrapper around [pipeline.Pipeline]. It
// owns the in-memory sink, the diagnostic sink, and the testing.TB
// it was created against; assertion methods report failures through
// that TB without bubbling errors back to callers.
//
// Pipeline values are produced by [Builder.Build] and are not safe
// for concurrent use — they hold a reference to a single test's TB.
type Pipeline struct {
	t     testing.TB
	inner *pipeline.Pipeline
	sink  *sink.Memory
	diag  *diag.Sink
}

// Run executes the underlying pipeline. A non-nil error from
// [pipeline.Pipeline.Run] that is anything other than the expected
// "had errors" disposition fails the test via t.Fatalf. The "had
// errors" disposition is left to the caller to inspect via
// [Pipeline.Diagnostics] — tests that exercise diagnostic-emitting
// plugins need the run to complete so they can assert against the
// recorded diagnostics.
//
// Returns the Pipeline for chaining into assertions.
func (p *Pipeline) Run(patterns ...string) *Pipeline {
	p.t.Helper()
	if len(patterns) == 0 {
		patterns = []string{""}
	}
	err := p.inner.Run(context.Background(), patterns...)
	if err != nil && !errors.Is(err, pipeline.ErrRunHadErrors) {
		p.t.Fatalf("testpipe: run failed: %v", err)
	}
	return p
}

// Diagnostics returns the diagnostic sink the run wrote into. Tests
// that assert against emitted diagnostics inspect the snapshot via
// [diag.Sink.Diagnostics].
func (p *Pipeline) Diagnostics() *diag.Sink { return p.diag }

// Manifest returns the per-run record of every output the backend
// produced, each attributed to the plugins that contributed to it.
// It is the only place a test can observe what `eidos prune` and
// `eidos check` will later consume — [emit.Target], contributing
// plugin names, content hash and resolved routing, per file.
//
// Nil until [Builder.WithManifestPath] is set and [Pipeline.Run] has
// been called: the pipeline returns before assembling anything when
// no path is configured. Pair the path with
// [Builder.WithDryRun](true) — assembly happens before the write, so
// the manifest is fully populated and nothing reaches disk.
//
// The returned value is the pipeline's own manifest, not a copy;
// treat it as read-only.
func (p *Pipeline) Manifest() *manifest.Manifest { return p.inner.LastManifest() }

// Sink returns the in-memory sink the run wrote rendered files into.
// Tests that need fine-grained access (e.g. enumerating every
// recorded target) use this accessor; the per-file assertion methods
// on Pipeline are the typical path.
func (p *Pipeline) Sink() *sink.Memory { return p.sink }

// AssertFile finds the file whose [emit.Target.Filename] matches
// name and returns a [FileAssertion] for chained per-file
// expectations. Calls t.Fatalf when no file matches or when more
// than one file matches — the latter typically means the test
// should disambiguate via [Pipeline.AssertFileInDir].
func (p *Pipeline) AssertFile(name string) *FileAssertion {
	p.t.Helper()
	matches := p.matchFiles(func(tgt emit.Target) bool {
		return tgt.Filename == name
	})
	switch len(matches) {
	case 0:
		p.t.Fatalf("testpipe: no file named %q in captured output (have: %v)", name, p.fileNames())
	case 1:
		// fall through
	default:
		p.t.Fatalf(
			"testpipe: %d files named %q in captured output; disambiguate via AssertFileInDir",
			len(matches), name,
		)
	}
	return &FileAssertion{t: p.t, target: matches[0].target, body: matches[0].body}
}

// AssertFileInDir finds the file matching (dir, name) exactly and
// returns a [FileAssertion]. Use this when several targets share a
// basename.
func (p *Pipeline) AssertFileInDir(dir, name string) *FileAssertion {
	p.t.Helper()
	tgt := emit.Target{Dir: dir, Filename: name}
	for stored, body := range p.sink.Files() {
		if stored.Dir == dir && stored.Filename == name {
			return &FileAssertion{t: p.t, target: stored, body: body}
		}
	}
	p.t.Fatalf("testpipe: no file matching %+v in captured output (have: %v)", tgt, p.fileTargets())
	return nil
}

// AssertNoFile fails the test when any captured target's filename
// matches name. Useful for testing tombstoning and conditional
// emission.
func (p *Pipeline) AssertNoFile(name string) *Pipeline {
	p.t.Helper()
	if matches := p.matchFiles(func(tgt emit.Target) bool {
		return tgt.Filename == name
	}); len(matches) > 0 {
		p.t.Errorf("testpipe: expected no file named %q; found %d", name, len(matches))
	}
	return p
}

// AssertFileCount fails the test when the captured output does not
// contain exactly n files.
func (p *Pipeline) AssertFileCount(n int) *Pipeline {
	p.t.Helper()
	if got := p.sink.Len(); got != n {
		p.t.Errorf("testpipe: expected %d captured file(s); got %d (have: %v)", n, got, p.fileNames())
	}
	return p
}

// matchedFile pairs a target with its captured body. Internal to
// [Pipeline] — used by the AssertFile family for predicate-driven
// lookups without leaking the full target+body map.
type matchedFile struct {
	target emit.Target
	body   []byte
}

// matchFiles returns every captured target+body pair whose target
// satisfies pred.
func (p *Pipeline) matchFiles(pred func(emit.Target) bool) []matchedFile {
	var out []matchedFile
	for tgt, body := range p.sink.Files() {
		if pred(tgt) {
			out = append(out, matchedFile{target: tgt, body: body})
		}
	}
	return out
}

// fileNames returns the captured filenames in sorted order. Used in
// failure messages so the user can see what was actually written.
func (p *Pipeline) fileNames() []string {
	names := make([]string, 0, p.sink.Len())
	for tgt := range p.sink.Files() {
		names = append(names, tgt.Filename)
	}
	slices.Sort(names)
	return names
}

// fileTargets returns the captured targets verbatim. Used in failure
// messages where the basename alone is ambiguous.
func (p *Pipeline) fileTargets() []emit.Target {
	out := make([]emit.Target, 0, p.sink.Len())
	for tgt := range p.sink.Files() {
		out = append(out, tgt)
	}
	return out
}
