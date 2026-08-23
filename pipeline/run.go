// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
	"go.thesmos.sh/eidos/store"
)

// Run executes the resolved [Plan] against a fresh [store.Store],
// in plan order: every frontend, then every annotator, then every
// generator, then the backend. Each plugin gets its own
// [store.Reader] so read-tracking is per-plugin (the recorded reads
// later feed the cache layer).
//
// Run is run-to-completion: a non-nil error from any plugin's role
// method becomes a [diag.Error] diagnostic attributed to the plugin
// and execution continues with the next plugin in the same phase
// (and the next phase). Plugin code that panics is contained by a
// [diag.RecoverAs] guard installed at the plugin invocation
// boundary; the panic surfaces as an [diag.Error] with a stack
// trace and the next plugin still runs. Plugin code that emits
// diagnostics directly to ctx.Diag is captured the same way.
//
// After every plugin has run, Run returns [ErrRunHadErrors] when
// any [diag.Error] diagnostic was recorded; otherwise nil. Call
// [Pipeline.Diag] to inspect the per-error details.
//
// Returns [ErrNoSink] without running any phase when no
// [sink.Sink] was configured at Build time — the backend has
// nowhere to write so the run cannot meaningfully complete.
//
// patterns is the per-frontend input list (typically Go-style
// import paths or filesystem globs). Each frontend receives every
// pattern. When [Builder.WithVerbose] was set the pipeline emits
// per-phase Info diagnostics so the user can see progress without
// turning on per-plugin tracing.
func (p *Pipeline) Run(ctx context.Context, patterns ...string) error {
	if p.sink == nil {
		return ErrNoSink
	}
	p.resetRunState()

	// Wrap the configured sink with a recording wrapper so the
	// pipeline can compose a manifest from every captured write at
	// run end. The wrapper writes through to the inner sink so
	// backend output still reaches its destination.
	recorder := newRecordingSink(p.sink)
	p.lastRecorder.Store(recorder)

	s := store.New()
	// So the store can report a caller consulting an index it
	// switched off; see [store.Store.SetDiag].
	s.SetDiag(p.diag)
	p.lastStore.Store(s)

	// Cancellation is checked between phases, not inside them. A
	// phase boundary is the smallest point the run can be abandoned
	// without leaving the store half-built: the node graph freezes
	// after the frontends, the emit graph after layout, and every
	// later phase assumes those invariants hold.
	//
	// This does not bound a single long phase — a frontend loading a
	// large module graph still runs to completion — but it stops the
	// run proceeding through every remaining phase after the caller
	// has given up. Bounding work inside a phase means threading the
	// context into the plugin contexts so plugins can unwind their
	// own I/O, which changes four public struct shapes and is a
	// separate change.
	phases := []struct {
		name string
		run  func()
	}{
		{"frontend", func() { p.runFrontends(s, patterns); s.Nodes().Freeze() }},
		{"annotator", func() { p.runAnnotators(s); p.runDirectiveOverride(s) }},
		{"generator", func() { p.runGenerators(s) }},
		{"layout", func() { p.runLayout(s); s.Emit().Freeze() }},
		{"backend", func() { p.runBackend(s, recorder) }},
		{"manifest", func() { p.writeManifest(recorder, s) }},
	}
	for _, phase := range phases {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w: cancelled before the %s phase", err, phase.name)
		}
		phase.run()
	}
	p.logRunSummary()

	if p.diag.HasErrors() {
		// Layout-phase sentinels are joined in so a caller can
		// classify the failure with errors.Is rather than
		// substring-matching a diagnostic message. ErrRunHadErrors
		// stays first so existing callers matching only on it are
		// unaffected.
		return errors.Join(append([]error{ErrRunHadErrors}, p.layoutErrors()...)...)
	}
	return nil
}

// writeManifest writes the run-end manifest when a path is
// configured via [Builder.WithManifestPath]. Manifest write errors
// surface as Warn diagnostics (the manifest is observability, not
// correctness) so a manifest-write failure does not turn the run
// into a failed one.
//
// Narrow-scope runs (e.g. `eidos run ./sub/...` after a prior
// `./...` run) merge with the prior manifest rather than
// overwriting it: prior entries whose [emit.Target.ImportPath]
// matches a package the current run did NOT load are preserved
// verbatim. Without the merge, a `./sub/...` run would shrink the
// manifest to just `sub/` entries and orphan everything else from
// prune / drift tracking. An in-scope entry whose file is gone is
// dropped, which is what lets the manifest converge instead of
// accumulating claims nothing can clear. See
// [Pipeline.mergeManifestPreservingOutOfScope].
//
// The write is skipped when the merged manifest matches the prior
// on disk modulo RunID: the timestamp would otherwise refresh
// mtime and dirty the file in version control even when nothing
// the manifest describes changed. The RunID stays in the wire
// format (drift / prune tooling reads it to attribute outputs back
// to a run); it just doesn't rewrite for free.
func (p *Pipeline) writeManifest(rec *recordingSink, s *store.Store) {
	if p.manifestPath == "" {
		return
	}
	// Manifest assembly walks plugin-defined emit kinds to attribute
	// each output to its contributors, so plugin-authored code runs
	// here too. A panic while composing observability must not lose
	// a run whose output is already written.
	defer diag.RecoverAs(p.diag.For("pipeline"), position.Pos{})
	current := rec.asManifest(time.Now().UTC().Format(time.RFC3339), s, p.pluginNames(), p)
	prev, _ := manifest.Read(p.manifestPath)
	merged := p.mergeManifestPreservingOutOfScope(prev, current)
	p.lastManifest.Store(merged)
	p.reportOrphans(prev, current)
	if p.dryRun {
		return
	}
	if prev != nil && manifestContentEqual(prev, merged) {
		return
	}
	if err := manifest.Write(p.manifestPath, merged); err != nil {
		p.diag.For("pipeline").Warnf(position.Pos{}, "manifest write failed: %v", err)
	}
}

// sourceImportPath maps an output's import path back to the source
// package the run loaded.
//
// Layout gives any output whose filename ends `_test.go` the external
// test shift, appending `_test` to its package and import path — a
// path no source package ever declares. Matching an output against the
// loaded set without undoing that shift misses every test output, so
// the whole class becomes unreportable rather than some of it.
//
// The prune subcommand applies the same trim for the same reason.
func sourceImportPath(ip string) string {
	return strings.TrimSuffix(ip, "_test")
}

// orphanCap bounds how many paths the orphan warning names before it
// summarises the rest. A run that stops emitting a whole tier produces
// one per node, and a diagnostic longer than the output it describes
// is one an operator scrolls past.
const orphanCap = 5

// rootedSink is a sink whose writes land under a known filesystem
// root — the only shape whose outputs can be stat'd.
//
// [sink.Sink] is one method wide and the pipeline holds it as such,
// so a memory or stdout sink offers no root and no disk to consult.
// Asking the sink rather than joining against [Pipeline.sourceRoot]
// is the difference between a fact and a coincidence: the two agree
// for a CLI run from the module root and part company the moment a
// sink is rooted anywhere else, and the consequence of guessing is
// a manifest entry dropped for a file that is still there.
type rootedSink interface {
	Root() string
}

// outputRoot returns the filesystem root this run's outputs land
// under, and whether the sink offered one.
func (p *Pipeline) outputRoot() (string, bool) {
	rooted, ok := p.sink.(rootedSink)
	if !ok {
		return "", false
	}
	root := rooted.Root()
	if root == "" {
		return "", false
	}
	return root, true
}

// onDisk reports whether the output o claims is present under root.
//
// An absolute Dir bypasses the root, matching how the disk sink
// resolves the same target. A stat error other than not-exist is
// read as present: the file may well be there and unreadable, and
// treating that as absence would drop a manifest entry for an
// output prune could still act on.
func onDisk(root string, o manifest.Output) bool {
	path := filepath.Join(o.Target.Dir, o.Target.Filename)
	if !filepath.IsAbs(o.Target.Dir) {
		path = filepath.Join(root, path)
	}
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}

// reportOrphans warns when a previous run wrote outputs this one no
// longer produces.
//
// The pipeline never deletes: a run cannot tell "no longer generated"
// from "not generated by this invocation", so removal is `eidos prune`
// deliberately. What it can do is say so, because the alternative is
// how this surfaces today — a stale file referencing constructors that
// no longer exist, and a build failure naming the generated file
// rather than the derivation change that orphaned it.
//
// Scoped to packages this run actually loaded, which is what keeps a
// narrow `run ./sub/...` from reporting every other package as
// orphaned. Same rule the prune subcommand applies for the same
// reason.
//
// Every named path is stat'd first. The sentence claims the files
// remain on disk, and claiming it without looking produced a warning
// naming 141 files that were already deleted — a line a reader
// cannot act on trains them past the line, which costs exactly when
// a real orphan appears in it.
func (p *Pipeline) reportOrphans(prev, current *manifest.Manifest) {
	if prev == nil {
		return
	}
	scope := p.ScopeImportPaths()
	if len(scope) == 0 {
		return
	}
	root, rooted := p.outputRoot()
	if !rooted {
		// Nothing to stat against, so every word of the warning
		// below would be unverified. A sink with no root wrote no
		// files this run and can say nothing about a previous one's.
		return
	}
	produced := make(map[emit.Target]struct{}, len(current.Outputs))
	for _, o := range current.Outputs {
		produced[o.Target] = struct{}{}
	}
	var orphans []string
	for _, o := range prev.Outputs {
		if _, still := produced[o.Target]; still {
			continue
		}
		if _, loaded := scope[sourceImportPath(o.Target.ImportPath)]; !loaded {
			continue
		}
		if !onDisk(root, o) {
			// Claimed by the manifest and already gone. The merge
			// drops it, so this run is the last one that could have
			// mentioned it — and there is nothing for a reader to do
			// about a file that is not there.
			continue
		}
		orphans = append(orphans, filepath.Join(o.Target.Dir, o.Target.Filename))
	}
	if len(orphans) == 0 {
		return
	}
	slices.Sort(orphans)
	orphans = slices.Compact(orphans)
	named := orphans
	suffix := ""
	if len(named) > orphanCap {
		named = named[:orphanCap]
		suffix = fmt.Sprintf(" and %d more", len(orphans)-orphanCap)
	}
	p.diag.For("pipeline").Warnf(position.Pos{},
		"%d output(s) from a previous run are no longer produced and remain on disk: %s%s"+
			" — run the `prune` subcommand to remove them",
		len(orphans), strings.Join(named, ", "), suffix)
}

// ScopeImportPaths returns the set of source-package import
// paths the most recent [Pipeline.Run] loaded. The prune
// subcommand consults this so its orphan-identification only
// considers manifest entries whose source package the current
// pipeline actually re-scanned — narrow `run ./sub/...`
// invocations don't trigger prune of entries from packages the
// run didn't load. Returns nil before Run has been called.
func (p *Pipeline) ScopeImportPaths() map[string]struct{} {
	s := p.Store()
	if s == nil {
		return nil
	}
	out := map[string]struct{}{}
	for _, pkg := range s.Nodes().Packages().Items() {
		if pkg.Path != "" {
			out[pkg.Path] = struct{}{}
		}
	}
	return out
}

// mergeManifestPreservingOutOfScope returns a manifest that adds
// or refreshes every entry from current onto prior — never drops.
// Prior entries the current run did not re-emit are carried over
// verbatim; current entries replace prior at the same
// (Target, PipelineID) key. A [manifest.Output] carries no per-entry
// run stamp: the run identifier lives once, on
// [manifest.Manifest.RunID], and the merge takes it from current.
// Carried-over entries are therefore indistinguishable from
// re-emitted ones by inspection of the entry alone — which is why
// staleness is decided by diffing, not by reading a timestamp.
//
// Run's job is ADD / UPDATE only. Identifying orphans (entries
// whose source no longer claims them) and deleting their files is
// prune's job — [manifest.Prune] calls a prior entry an orphan when
// all three hold: its PipelineID matches the current pipeline, its
// [emit.Target.ImportPath] is in the set of packages this run
// loaded, and its Target is absent from the set this run emitted.
//
// A nil prior reduces to current verbatim.
//
// The merged Outputs slice is sorted by
// (Dir, Filename, Package, ImportPath, PipelineID) for a total
// ordering — keeps the on-disk JSON byte-stable across runs even
// when several pipelines coexist in one manifest.
func (p *Pipeline) mergeManifestPreservingOutOfScope(
	prev, current *manifest.Manifest,
) *manifest.Manifest {
	if prev == nil {
		return current
	}
	root, rooted := p.outputRoot()
	scope := p.ScopeImportPaths()
	merged := manifest.New(current.RunID)
	merged.Brand = current.Brand
	type key struct {
		target     emit.Target
		pipelineID string
	}
	currentKeys := map[key]struct{}{}
	for _, o := range current.Outputs {
		currentKeys[key{o.Target, o.PipelineID}] = struct{}{}
	}
	for _, o := range prev.Outputs {
		if _, replaced := currentKeys[key{o.Target, o.PipelineID}]; replaced {
			continue
		}
		if p.converged(root, rooted, scope, o) {
			continue
		}
		merged.Add(o)
	}
	for _, o := range current.Outputs {
		merged.Add(o)
	}
	slices.SortFunc(merged.Outputs, func(a, b manifest.Output) int {
		if c := cmp.Compare(a.Target.Dir, b.Target.Dir); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Target.Filename, b.Target.Filename); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Target.Package, b.Target.Package); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Target.ImportPath, b.Target.ImportPath); c != 0 {
			return c
		}
		return cmp.Compare(a.PipelineID, b.PipelineID)
	})
	return merged
}

// converged reports whether a prior output should leave the manifest
// because there is nothing left to say about it: the run loaded its
// package, did not produce it, and its file is gone.
//
// The disk check is the load-bearing half. An entry dropped while
// its file is still there becomes untracked garbage — the manifest
// is how prune knows what to delete, so forgetting a live orphan
// makes it permanently unprunable. Absence is the only state where
// dropping costs nothing, and it is the state that otherwise
// accumulates forever.
//
// Out-of-scope entries are never dropped: the run cannot distinguish
// "no longer produced" from "not produced by this invocation" for a
// package it never loaded, which is the same reason the pipeline
// does not delete.
func (*Pipeline) converged(
	root string, rooted bool, scope map[string]struct{}, o manifest.Output,
) bool {
	if !rooted || len(scope) == 0 {
		return false
	}
	if _, loaded := scope[sourceImportPath(o.Target.ImportPath)]; !loaded {
		return false
	}
	return !onDisk(root, o)
}

// manifestContentEqual reports whether prev and current describe the
// same on-disk consequence — same Version, Brand, and Outputs set.
// RunID is excluded because it is a per-run timestamp; including it
// would force a rewrite on every run, defeating the
// stable-bytes-across-runs property the manifest is supposed to
// preserve for git-committed projects.
func manifestContentEqual(prev, current *manifest.Manifest) bool {
	if prev == nil || current == nil {
		return false
	}
	if prev.Version != current.Version || prev.Brand != current.Brand {
		return false
	}
	if len(prev.Outputs) != len(current.Outputs) {
		return false
	}
	for i := range prev.Outputs {
		if !manifestOutputEqual(prev.Outputs[i], current.Outputs[i]) {
			return false
		}
	}
	return true
}

// manifestOutputEqual reports whether two [manifest.Output] values
// describe the same emit decl — Target identity, contributing
// plugins, body hash, and resolved-layout block. The slice / map
// fields compare element-by-element so the equality stays robust
// against `reflect.DeepEqual`'s quirks under future Output
// additions.
func manifestOutputEqual(a, b manifest.Output) bool {
	if a.Target != b.Target || a.Hash != b.Hash {
		return false
	}
	if len(a.Plugins) != len(b.Plugins) {
		return false
	}
	for i := range a.Plugins {
		if a.Plugins[i] != b.Plugins[i] {
			return false
		}
	}
	switch {
	case a.ResolvedLayout == nil && b.ResolvedLayout == nil:
		return true
	case a.ResolvedLayout == nil || b.ResolvedLayout == nil:
		return false
	}
	rla, rlb := *a.ResolvedLayout, *b.ResolvedLayout
	switch {
	case rla.Layout != rlb.Layout,
		rla.Package != rlb.Package,
		rla.Dir != rlb.Dir,
		rla.Filename != rlb.Filename,
		len(rla.ResolvedFrom) != len(rlb.ResolvedFrom):
		return false
	}
	for k, v := range rla.ResolvedFrom {
		if rlb.ResolvedFrom[k] != v {
			return false
		}
	}
	return true
}

// pluginNames returns the registered plugins' [plugin.Plugin.Name]
// values in registration order — frontends, annotators, generators,
// then the backend. The manifest's per-output Plugins list quotes
// this slice so every entry shares the run's plugin universe; the
// rendered file's `Plugins:` header is composed from the same set,
// so manifest and on-disk provenance stay aligned.
func (p *Pipeline) pluginNames() []string {
	out := make([]string, 0, len(p.frontends)+len(p.annotators)+len(p.generators)+1)
	for _, fe := range p.frontends {
		out = append(out, fe.Name())
	}
	for _, ann := range p.annotators {
		out = append(out, ann.Name())
	}
	for _, gen := range p.generators {
		out = append(out, gen.Name())
	}
	if p.backend != nil {
		out = append(out, p.backend.Name())
	}
	return out
}

// DryRun returns the resolved [Plan] without executing any phase.
// Tooling such as "eidos explain plan" calls DryRun to display the
// resolved order and any Build-time diagnostics without writing
// files.
//
// The context is accepted for signature symmetry with
// [Pipeline.Run] and is not consulted: the plan is resolved during
// Build, so DryRun performs no I/O and executes no phase. There is
// nothing to cancel.
func (p *Pipeline) DryRun(_ context.Context) *Plan {
	return p.plan
}

// runFrontends invokes Load on every frontend for every pattern.
// Per-call errors and panics become Error diagnostics attributed to
// the frontend's name; subsequent frontends and patterns still run.
// When [PhaseFrontend] is opted into via [Builder.WithParallel] the
// frontend×pattern invocations dispatch concurrently.
func (p *Pipeline) runFrontends(s *store.Store, patterns []string) {
	p.logPhaseStart("frontend", "%d frontend(s), %d pattern(s)", len(p.plan.Frontends), len(patterns))
	if p.parallel[PhaseFrontend] {
		var wg sync.WaitGroup
		for _, fe := range p.plan.Frontends {
			for _, pattern := range patterns {
				wg.Go(func() { p.invokeFrontend(fe, pattern, s) })
			}
		}
		wg.Wait()
		return
	}
	for _, fe := range p.plan.Frontends {
		for _, pattern := range patterns {
			p.invokeFrontend(fe, pattern, s)
		}
	}
}

// invokeFrontend runs one Frontend.Load call with panic containment
// so a misbehaving frontend cannot abort the entire run.
func (p *Pipeline) invokeFrontend(fe plugin.Frontend, pattern string, s *store.Store) {
	ps := p.diag.For(fe.Name())
	defer diag.RecoverAs(ps, position.Pos{})
	ctx := &plugin.FrontendContext{
		Store:       s,
		Diag:        p.diag,
		Registry:    p.registry,
		Parser:      p.parser,
		Cache:       p.cache,
		Pattern:     pattern,
		Fingerprint: p.fingerprint,
	}
	if err := fe.Load(ctx); err != nil {
		p.reportPluginError(ps, fe.Name(), fmt.Sprintf("frontend Load(%q)", pattern), err)
	}
}

// reportPluginError records err as a diagnostic attributed to a
// specific plugin. Errors wrapping [store.ErrFrozen] indicate a
// framework-contract violation (a plugin mutated a view it should
// not have touched) and surface at Internal severity so operators
// can distinguish them from ordinary user-side problems. Every
// other error becomes a normal Error diagnostic.
func (p *Pipeline) reportPluginError(ps *diag.PluginSink, name, role string, err error) {
	if errors.Is(err, store.ErrFrozen) {
		p.diag.Internalf(position.Pos{}, "%s %q violated frozen-store contract: %v", role, name, err)
		return
	}
	ps.Errorf(position.Pos{}, "%s failed: %v", role, err)
}

// runAnnotators invokes Annotate on every annotator. Buckets run
// in ascending priority order; within a bucket plugins run in
// topo-sorted order sequentially, OR concurrently when
// [PhaseAnnotator] is enabled via [Builder.WithParallel] AND every
// plugin in the bucket has pairwise-disjoint [plugin.CapabilityProvider.Provides]
// (per spec §18). Buckets that fail the disjoint check fall back to
// sequential to preserve write-order semantics.
func (p *Pipeline) runAnnotators(s *store.Store) {
	p.logPhaseStart("annotator", "%d annotator(s)", len(p.plan.Annotators))
	for _, bucket := range p.plan.AnnotatorBuckets {
		if p.parallel[PhaseAnnotator] {
			// Build rejects same-bucket plugins that claim the same
			// Provides name (ErrDuplicateProvider), so by the time
			// the runtime sees a bucket every plugin's Provides
			// set is pairwise disjoint and the bucket may safely
			// dispatch concurrently.
			var wg sync.WaitGroup
			for _, ann := range bucket.Plugins {
				wg.Go(func() { p.invokeAnnotator(ann, s) })
			}
			wg.Wait()
			continue
		}
		for _, ann := range bucket.Plugins {
			p.invokeAnnotator(ann, s)
		}
	}
}

// invokeAnnotator runs one Annotator.Annotate call with panic
// containment. After the call returns the recorded
// [store.ReadSet.Hash] is written to the cache under a
// per-plugin key so cache-aware downstream tooling can detect
// "this plugin ran with these reads".
func (p *Pipeline) invokeAnnotator(ann plugin.Annotator, s *store.Store) {
	ps := p.diag.For(ann.Name())
	defer diag.RecoverAs(ps, position.Pos{})
	r := p.newReader(s)
	ctx := &plugin.AnnotatorContext{
		Store:  s,
		Reader: r,
		Diag:   p.diag,
	}
	if err := ann.Annotate(ctx); err != nil {
		p.reportPluginError(ps, ann.Name(), "annotator", err)
	}
	p.recordCacheKey(ann.Name(), r)
}

// runGenerators invokes Generate on every generator. Buckets run
// in ascending priority order; within a bucket plugins run in
// topo-sorted order sequentially, OR concurrently when
// [PhaseGenerator] is enabled via [Builder.WithParallel] AND every
// plugin in the bucket implements [plugin.NodesOnly] returning
// true (i.e. they promise not to read upstream emit). Buckets that
// fail the NodesOnly check fall back to sequential.
func (p *Pipeline) runGenerators(s *store.Store) {
	p.logPhaseStart("generator", "%d generator(s)", len(p.plan.Generators))
	for _, bucket := range p.plan.GeneratorBuckets {
		if p.parallel[PhaseGenerator] && allNodesOnly(bucket.Plugins) {
			var wg sync.WaitGroup
			for _, gen := range bucket.Plugins {
				wg.Go(func() { p.invokeGenerator(gen, s) })
			}
			wg.Wait()
			continue
		}
		for _, gen := range bucket.Plugins {
			p.invokeGenerator(gen, s)
		}
	}
}

// allNodesOnly reports whether every generator in plugins
// implements [plugin.NodesOnly] returning true. A single false /
// non-implementing generator disqualifies the bucket from parallel
// execution because it might read the emit graph another generator
// is mutating.
func allNodesOnly(plugins []plugin.Generator) bool {
	for _, g := range plugins {
		no, ok := any(g).(plugin.NodesOnly)
		if !ok || !no.NodesOnly() {
			return false
		}
	}
	return true
}

// invokeGenerator runs one Generator.Generate call with panic
// containment. After the call returns the recorded
// [store.ReadSet.Hash] is written to the cache under a
// per-plugin key — see [Pipeline.recordCacheKey].
func (p *Pipeline) invokeGenerator(gen plugin.Generator, s *store.Store) {
	ps := p.diag.For(gen.Name())
	defer diag.RecoverAs(ps, position.Pos{})
	r := p.newReader(s)
	ctx := &plugin.GeneratorContext{
		Store:  s,
		Reader: r,
		Diag:   p.diag,
	}
	if err := gen.Generate(ctx); err != nil {
		p.reportPluginError(ps, gen.Name(), "generator", err)
	}
	p.recordCacheKey(gen.Name(), r)
}

// recordCacheKey writes the per-plugin cache marker — a key
// composed of every input the plugin's output depends on — to
// the configured cache. Two kinds of routing input enter the key
// alongside the existing reads-hash:
//
//   - The plugin's resolved [LayoutPolicy] (layout / package /
//     directory after every precedence layer is merged) — a flip
//     of any project, per-plugin, or CLI override fed into the
//     merge produces a different key for that plugin only when
//     the merge actually changes the resolved value.
//   - The run-wide scope inputs the routing layer reads
//     uniformly across every plugin: the literal -target value
//     (the scope filter) and the literal -o value (the per-decl
//     filename override). Either flip invalidates every plugin's
//     cache key for the run.
//
// The entry is a fingerprint, not a memo. Nothing reads it to skip
// a phase, and the stored body is the same read-set hash already
// embedded in the key, so it carries no recoverable output. Its
// value is answering "did this plugin run against these inputs" —
// which `eidos explain` is the natural consumer of.
//
// Making it skip-on-hit is not a small change: a generator's output
// is emit contributions, and the emit graph is a live object graph
// with owner and slot back-pointers, not a byte payload. Frontends
// cache their node graph and do skip on a hit; see
// [frontend/golang] for the working shape.
//
// Errors from the cache are silently dropped because the cache is
// best-effort: a failed write is no worse than running without a
// cache at all.
func (p *Pipeline) recordCacheKey(name string, r *store.Reader) {
	// Hashed once and used twice. Calling Hash for the key and again
	// for the body re-drained the map, re-sorted it, and re-ran
	// SHA-256 to produce the same string four lines apart — and left
	// the two able to disagree if a plugin had a goroutine still
	// recording reads.
	reads := r.ReadSet().Hash()
	key := p.cacheKeyFor(name, reads)
	_ = p.cache.Put(key, []byte(reads)) //nolint:errcheck // best-effort cache marker
}

// cacheKeyFor composes the cache key for one plugin invocation.
//
// Split out from [Pipeline.recordCacheKey] so the composed key is
// assertable without a cache round-trip. Frontends fold the
// composition fingerprint into their own keys and the marker written
// here is filed under this one, so a silent change in either
// invalidates on-disk caches without failing anything — which makes
// the key worth pinning by value rather than by derivation.
func (p *Pipeline) cacheKeyFor(name, reads string) string {
	return cache.NewKey(
		"plugin", name,
		"version", p.pluginVersions[name],
		"reads", reads,
		"routing", p.routingHash(name),
		"scope", p.scopeHash,
	)
}

// cacheRoutingComponents returns the resolved-policy fields that
// enter the per-plugin cache key. Returned in canonical order
// so [cache.HashStrings] produces stable digests across runs.
func (p *Pipeline) cacheRoutingComponents(pluginName string) []string {
	pol := p.LayoutPolicyFor(pluginName)
	return []string{pol.Layout, pol.Package, pol.Dir}
}

// runBackend invokes Render on the backend with a populated
// [plugin.BackendContext] including the registered-order plugin
// list (for template-collection enumeration) and the plan-execution
// order (for deterministic override application). Wraps the call
// in a [diag.RecoverAs] guard so a backend panic is contained.
func (p *Pipeline) runBackend(s *store.Store, dst sink.Sink) {
	p.logPhaseStart("backend", "lang=%s", p.backend.Language())
	ps := p.diag.For(p.backend.Name())
	defer diag.RecoverAs(ps, position.Pos{})
	r := p.newReader(s)
	ctx := &plugin.BackendContext{
		Store:      s,
		Reader:     r,
		Diag:       p.diag,
		Sink:       dst,
		Lang:       p.backend.Language(),
		Plugins:    p.registeredPlugins(),
		Ordered:    p.orderedPlugins(),
		Command:    p.commandHeader(),
		SourceRoot: p.sourceRoot,
		Brand:      p.brand,
	}
	if err := p.backend.Render(ctx); err != nil {
		p.reportPluginError(ps, p.backend.Name(), "backend", err)
	}
	p.recordCacheKey(p.backend.Name(), r)
}

// newReader constructs the per-plugin [store.Reader] every plugin
// phase hands to a plugin. When the pipeline carries a scope
// predicate (set via [Builder.WithTargetSymbol]) the returned
// reader pre-filters node-side range queries to in-scope nodes
// transparently; an unconfigured pipeline returns a vanilla
// unscoped reader.
func (p *Pipeline) newReader(s *store.Store) *store.Reader {
	if p.scope == nil {
		return store.NewReader(s)
	}
	return store.NewScopedReader(s, p.scope)
}

// commandHeader returns the literal string to stamp into the
// "Command:" header line of every rendered file. A caller-supplied
// value through [Builder.WithCommand] wins — letting tests and
// library embedders pin a stable value. Empty falls back to
// [commandLine], which renders `os.Args[1:]` for real CLI runs and
// "(library)" when no positional arguments are present.
func (p *Pipeline) commandHeader() string {
	if p.command != "" {
		return p.command
	}
	return commandLine()
}

// commandLine returns the CLI-style rendering of the current
// process's arguments, used as the [plugin.BackendContext.Command]
// default. The binary's basename leads (e.g. `eidos run ./...`)
// so the rendered `// Command:` header is copy-pasteable back into
// a shell. When the host has no positional arguments (typically
// library / test invocations), returns "(library)" — a stable
// marker that signals programmatic use without leaking
// test-runner flags into the generated output.
//
// Test-runner invocations populate os.Args with per-machine paths
// (`-test.testlogfile=/var/folders/.../testlog.txt`) — a
// determinism leak the [Builder.WithCommand] override exists to
// neutralise.
func commandLine() string {
	if len(os.Args) <= 1 {
		return "(library)"
	}
	return filepath.Base(os.Args[0]) + " " + strings.Join(os.Args[1:], " ")
}

// logPhaseStart writes a verbose-mode Info diagnostic announcing
// the phase boundary. No-op when verbose mode is off so silent runs
// stay silent.
func (p *Pipeline) logPhaseStart(phase, format string, args ...any) {
	if !p.verbose {
		return
	}
	ps := p.diag.For("pipeline")
	ps.Infof(position.Pos{}, "phase=%s "+format, append([]any{phase}, args...)...)
}

// logRunSummary writes a verbose-mode Info diagnostic at run-end
// with a count of diagnostics emitted across all phases. No-op
// when verbose mode is off.
func (p *Pipeline) logRunSummary() {
	if !p.verbose {
		return
	}
	ps := p.diag.For("pipeline")
	ps.Infof(position.Pos{}, "run complete: %d error(s), %d warning(s), %d info",
		p.diag.Count(diag.Error), p.diag.Count(diag.Warn), p.diag.Count(diag.Info))
}

// registeredPlugins returns the full list of plugins in user
// registration order: frontends, then annotators, then generators,
// then the backend. The backend uses this list to find every
// [plugin.TemplateProvider] for template merging.
func (p *Pipeline) registeredPlugins() []plugin.Plugin {
	out := make([]plugin.Plugin, 0,
		len(p.frontends)+len(p.annotators)+len(p.generators)+1)
	for _, x := range p.frontends {
		out = append(out, x)
	}
	for _, x := range p.annotators {
		out = append(out, x)
	}
	for _, x := range p.generators {
		out = append(out, x)
	}
	out = append(out, p.backend)
	return dedupePlugins(out)
}

// orderedPlugins returns the full plugin list in plan-execution
// order: frontends (registration order; the frontend role has no
// priority/topo), then annotators (plan order), then generators
// (plan order), then the backend. The backend uses this list to
// apply [plugin.TemplateProvider.TemplateOverrides] deterministically.
func (p *Pipeline) orderedPlugins() []plugin.Plugin {
	out := make([]plugin.Plugin, 0,
		len(p.plan.Frontends)+len(p.plan.Annotators)+len(p.plan.Generators)+1)
	for _, x := range p.plan.Frontends {
		out = append(out, x)
	}
	for _, x := range p.plan.Annotators {
		out = append(out, x)
	}
	for _, x := range p.plan.Generators {
		out = append(out, x)
	}
	out = append(out, p.plan.Backend)
	return dedupePlugins(out)
}

// precomputeRunInvariants derives the cache-key and fingerprint
// inputs that cannot change once Build has returned.
//
// Called from [Builder.Build] after the Pipeline is assembled,
// because the derivations read through [Pipeline.LayoutPolicyFor] and
// so need the finished value. Everything it computes is a pure
// function of fields Build assigns and nothing mutates afterwards.
//
// Keyed per plugin name at [Pipeline.LayoutPolicyFor]'s granularity,
// not [Pipeline.LayoutPolicyForTag]'s: cacheRoutingComponents reads
// the per-plugin policy, and matching the per-tag one instead would
// change every key it feeds.
func (p *Pipeline) precomputeRunInvariants() {
	plugins := p.registeredPlugins()
	p.pluginVersions = make(map[string]string, len(plugins))
	p.routingHashes = make(map[string]string, len(plugins))

	parts := make([]string, 0, len(plugins))
	for _, pl := range plugins {
		name := pl.Name()
		version := ""
		if v, ok := any(pl).(plugin.Versioned); ok {
			version = v.Version()
		}
		p.pluginVersions[name] = version
		p.routingHashes[name] = cache.HashStrings(p.cacheRoutingComponents(name))
		parts = append(parts, name+"@"+version)
	}

	// registeredPlugins already includes the backend and dedupes a
	// dual-role plugin, so the composition string is exactly what
	// compositionFingerprint built per (frontend × pattern).
	p.fingerprint = cache.HashStrings(parts)
	p.scopeHash = cache.HashStrings([]string{p.targetSym, p.outFilename})
}

// routingHash returns the precomputed routing digest for name.
//
// Falls back to computing it for a name Build never saw. That is not
// dead code: [Pipeline.LayoutPolicyFor] documents the unknown-name
// case as resolving through the project + CLI merge, and a lookup
// that silently answered the empty string instead would change the
// key rather than reproduce it.
func (p *Pipeline) routingHash(name string) string {
	if h, ok := p.routingHashes[name]; ok {
		return h
	}
	return cache.HashStrings(p.cacheRoutingComponents(name))
}
