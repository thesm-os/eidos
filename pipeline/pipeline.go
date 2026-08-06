// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline

import (
	"sync"
	"sync/atomic"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
	"go.thesmos.sh/eidos/store"
)

// LayoutAlongsideSource selects the alongside-source layout
// policy: rendered files land in the directory of the originating
// source file, declaring the source package's name. This is the
// framework default.
const LayoutAlongsideSource = "alongside-source"

// LayoutCentralised selects the centralised layout policy:
// rendered files land in a configured shared directory under a
// configured package name, regardless of origin location.
const LayoutCentralised = "centralised"

// LayoutPolicy is the resolved routing policy for one plugin in
// one pipeline run. The pipeline composes it by merging the
// framework default with the project [output] config, the
// per-plugin override, and CLI overrides — field by field, later
// layers winning. The Layout phase reads the policy through
// [Pipeline.LayoutPolicyFor] when composing each decl's Target.
//
// Layout is one of [LayoutAlongsideSource] or [LayoutCentralised].
// Package and Dir are meaningful only under centralised layout;
// alongside-source layout derives them from origin and ignores
// these fields at routing time.
//
// Each value-bearing field carries a sibling [manifest.Layer]
// recording which precedence layer supplied the field — the
// pipeline reads the From fields when stamping the manifest's
// observability block so attribution stays accurate as more
// layers (project / per-plugin config) feed into the merge.
//
// The zero value of LayoutPolicy is NOT a valid policy — its
// `*From` fields are empty strings rather than [manifest.Layer]
// values, which the manifest sink would record as missing
// attribution. Construct policies through [NewLayoutPolicy] (for
// a framework-default seed) or via [Builder.Build] (which runs
// the canonical merge); never construct directly from a struct
// literal in non-test code.
type LayoutPolicy struct {
	Layout      string
	LayoutFrom  manifest.Layer
	Package     string
	PackageFrom manifest.Layer
	Dir         string
	DirFrom     manifest.Layer
}

// NewLayoutPolicy returns a [LayoutPolicy] seeded with the
// framework default — alongside-source layout, empty Package and
// Dir, every `*From` field stamped [manifest.LayerFramework].
// Layers higher in the precedence merge overwrite individual
// fields and re-stamp their `*From` siblings as they take
// effect.
func NewLayoutPolicy() LayoutPolicy {
	return LayoutPolicy{
		Layout:      LayoutAlongsideSource,
		LayoutFrom:  manifest.LayerFramework,
		PackageFrom: manifest.LayerFramework,
		DirFrom:     manifest.LayerFramework,
	}
}

// Pipeline is the validated, ready-to-run artifact returned by
// [Builder.Build]. It holds the participating plugins grouped by
// role plus the shared sink, cache, and diagnostic sink supplied
// during construction, and the resolved execution-ordered [Plan].
//
// A Pipeline is immutable after Build; access to its members is
// read-only and callers must not mutate the slices they receive
// from the typed accessors. Future milestones add Run / DryRun
// methods that execute the phases in plan order.
type Pipeline struct {
	frontends    []plugin.Frontend
	annotators   []plugin.Annotator
	generators   []plugin.Generator
	backend      plugin.Backend
	sink         sink.Sink
	cache        cache.Cache
	diag         *diag.Sink
	verbose      bool
	parallel     map[Phase]bool
	manifestPath string
	pipelineID   string
	brand        string
	dryRun       bool
	lastManifest atomic.Pointer[manifest.Manifest]
	lastRecorder atomic.Pointer[recordingSink]
	command      string
	sourceRoot   string
	// policy is the default [LayoutPolicy] returned by
	// [Pipeline.LayoutPolicyFor] for plugin names that have no
	// per-plugin override; pluginPolicies holds the pre-merged
	// per-plugin policies keyed by plugin name. Both are pinned
	// at Build time so the Layout phase reads a stable policy
	// across the run.
	defaultPolicy     LayoutPolicy
	pluginPolicies    map[string]LayoutPolicy
	pluginTagPolicies map[pluginTagKey]LayoutPolicy
	outFilename       string

	// pluginOutFilenames holds CLI `-o <plugin>[:<tag>]=<path>`
	// overrides keyed by (plugin, tag). The Layout phase looks up
	// `(plugin, tag)` first, falls back to `(plugin, "")` for an
	// unscoped per-plugin override, then to [outFilename] for the
	// legacy unscoped form. Empty map means no per-plugin
	// overrides are active.
	pluginOutFilenames map[pluginTagKey]string

	scope     store.ScopePredicate
	targetSym string
	plan      *Plan
	registry  *directive.Registry
	parser    *directive.Parser

	// directiveOwners maps each directive name to the plugin that
	// registered it via [plugin.DirectiveProvider.Directives]. The
	// Layout phase consults the map to scope per-directive `out=`
	// and `pkg=` routing keys to the owning plugin's output.
	// Directives registered manually via [Builder.WithDirective] or
	// shipped by the framework's [coreDirectives] surface have no
	// owner entry; routing keys on those flow through the existing
	// `+gen:out` `plugin=<name>` selector instead.
	directiveOwners map[directive.Name]string

	// lastStore caches the store from the most recent Run for
	// post-run inspection (test harnesses, "eidos explain"
	// tooling). Stored under an atomic.Pointer so concurrent
	// readers — typically tests dispatching multiple parallel
	// runs against the same Pipeline — observe a coherent value
	// without locking the run path.
	lastStore atomic.Pointer[store.Store]

	// resolvedLayouts records the per-Target routing decision the
	// Layout phase composed in the most recent run, keyed by the
	// resolved [emit.Target]. The recording sink reads from this
	// map at manifest-assembly time so each [manifest.Output]
	// carries its observability block. Layout writes the map
	// from a single goroutine; the manifest reader runs after
	// Layout completes; the mutex protects post-Run reads from
	// parallel test invocations that share a Pipeline.
	resolvedLayoutsMu sync.Mutex
	resolvedLayouts   map[emit.Target]resolvedEntry

	// fingerprint, scopeHash, routingHashes and pluginVersions are
	// the cache-key and composition inputs Build pins.
	//
	// Every one is a pure function of the plugin slices and the
	// routing configuration, all of which are assigned in Build and
	// have no mutator. Deriving them per invocation meant a run over
	// the reference pipeline's 21 plugins discarded 40 SHA-256
	// digests over constant bytes and 420 reflect.Value.Comparable
	// calls — the latter growing with the square of the plugin count,
	// since every one of the 20 recordCacheKey calls re-flattened and
	// re-deduped the whole set to find one version string.
	//
	// Hoisting also moves the plugin.Versioned assertion to Build.
	// The interface documents Version as a release-time constant, so
	// that is where it belongs; a plugin reading mutable state there
	// would now produce a different key, which is the intended
	// reading of the contract.
	fingerprint    string
	scopeHash      string
	routingHashes  map[string]string
	pluginVersions map[string]string

	// layoutErrs holds the sentinel-wrapped errors the Layout phase
	// reported this run, so [Pipeline.Run] can join them into its
	// return value and callers can classify with errors.Is. Guarded
	// by resolvedLayoutsMu, which already serialises the phase's
	// other per-run writes.
	layoutErrs []error
}

// resetRunState clears the per-run derived state a [Pipeline]
// accumulates, so a second [Pipeline.Run] on the same instance
// starts from the same footing as the first.
//
// Run is documented as executing against a fresh [store.Store], and
// it replaces lastStore and lastRecorder to match — but
// resolvedLayouts was allocated lazily and never cleared, which made
// two ordinary cross-run differences look like framework bugs:
//
//   - the Layout phase compares an incoming decision against the
//     retained entry for the same Target and reports divergence as
//     an Internal diagnostic. Routing that legitimately differs
//     between runs — an edited directive, a different pattern set —
//     tripped it.
//   - hasLayoutActivity is a length check, so once any run routed
//     anything it answered true forever, arming the "backend wrote
//     without Layout-composed attribution" check on a later run that
//     routed nothing.
//
// Both surface at Internal severity, which counts toward
// HasErrors, so the second run failed. Most callers build a fresh
// Pipeline per run and never saw it; the exposure is library
// embedders and long-lived processes, which the Builder/Pipeline
// split otherwise encourages.
func (p *Pipeline) resetRunState() {
	p.resolvedLayoutsMu.Lock()
	defer p.resolvedLayoutsMu.Unlock()
	p.resolvedLayouts = nil
	p.layoutErrs = nil
}

// recordLayoutErr retains a sentinel-wrapped Layout-phase error so
// [Pipeline.Run] can join it into its return value.
//
// Layout reports through the diagnostic sink, which takes a format
// string — so a sentinel reaching it via Error() is flattened to
// text and the chain is gone. Every Build-time sentinel is returned
// as a value and matches under errors.Is; without this the four
// Layout-phase sentinels did not, despite sitting in the same file,
// in the same var-block style, with docblocks in the same voice
// telling consumers to use errors.Is. A host wanting to special-case
// "a plugin forgot FilenameProvider" was left substring-matching a
// diagnostic, which CONTRIBUTING.md forbids outright.
//
// The diagnostic is still emitted; this is additive. Errors
// accumulate per run and are cleared by [Pipeline.resetRunState].
func (p *Pipeline) recordLayoutErr(err error) {
	if err == nil {
		return
	}
	p.resolvedLayoutsMu.Lock()
	defer p.resolvedLayoutsMu.Unlock()
	p.layoutErrs = append(p.layoutErrs, err)
}

// layoutErrors returns a copy of the Layout-phase errors recorded
// during the current run, in the order they were reported.
func (p *Pipeline) layoutErrors() []error {
	p.resolvedLayoutsMu.Lock()
	defer p.resolvedLayoutsMu.Unlock()
	if len(p.layoutErrs) == 0 {
		return nil
	}
	out := make([]error, len(p.layoutErrs))
	copy(out, p.layoutErrs)
	return out
}

// Store returns the [store.Store] used by the most recent
// [Pipeline.Run] invocation, or nil when Run has not been called.
// The returned store is read-only from the caller's perspective:
// the pipeline freezes the node + emit views as it transitions
// between phases, and reusing the Pipeline for another Run replaces
// the cached store with the next run's instance.
func (p *Pipeline) Store() *store.Store { return p.lastStore.Load() }

// EmittedTargets returns the set of [emit.Target] values the
// most recent [Pipeline.Run] actually wrote to the sink — the
// "current claims" view the prune subcommand diffs against the
// prior manifest. Targets in prev that are absent from this
// set (and whose pipeline / scope match) are orphans.
//
// Returns an empty map before Run has been called or when the
// last run produced no outputs.
func (p *Pipeline) EmittedTargets() map[emit.Target]struct{} {
	rec := p.lastRecorder.Load()
	if rec == nil {
		return map[emit.Target]struct{}{}
	}
	return rec.emittedTargets()
}

// LastManifest returns the merged manifest the most recent
// [Pipeline.Run] invocation produced, or nil when Run has not
// been called or the manifest path was unconfigured. The
// returned manifest is the post-merge view — every prior entry
// this run did not re-emit, plus every entry this run produced,
// with [manifest.Manifest.RunID] taken from the current run.
// Tooling that wants to know "what would this run have produced"
// without consulting disk reads from here; the prune subcommand
// uses it under `--dry-run` so the manifest stays untouched.
func (p *Pipeline) LastManifest() *manifest.Manifest { return p.lastManifest.Load() }

// PipelineID returns the stable identifier the manifest tags
// each [Output] with. Either the explicit value supplied via
// [Builder.WithPipelineID] or the framework's auto-derived
// digest of the registered plugin set ([derivePipelineID]).
// Used by the scope-aware manifest merge and by the prune
// subcommand to scope to one pipeline's entries in a
// multi-pipeline workdir.
func (p *Pipeline) PipelineID() string { return p.pipelineID }

// Frontends returns the registered frontends in registration order.
func (p *Pipeline) Frontends() []plugin.Frontend {
	out := make([]plugin.Frontend, len(p.frontends))
	copy(out, p.frontends)
	return out
}

// Annotators returns the registered annotators in registration
// order. For the resolved execution order (priority bucket +
// capability topo) call [Pipeline.Plan] and read its Annotators
// field.
func (p *Pipeline) Annotators() []plugin.Annotator {
	out := make([]plugin.Annotator, len(p.annotators))
	copy(out, p.annotators)
	return out
}

// Generators returns the registered generators in registration
// order. For the resolved execution order call [Pipeline.Plan].
func (p *Pipeline) Generators() []plugin.Generator {
	out := make([]plugin.Generator, len(p.generators))
	copy(out, p.generators)
	return out
}

// Backend returns the single registered backend.
func (p *Pipeline) Backend() plugin.Backend { return p.backend }

// Sink returns the destination sink the backend writes through.
// Returns nil when no sink was configured (the pipeline run loop
// later in this milestone surfaces a diagnostic in that case).
func (p *Pipeline) Sink() sink.Sink { return p.sink }

// Cache returns the configured [cache.Cache]. Returns the
// no-op [cache.None] sentinel when no cache was configured.
func (p *Pipeline) Cache() cache.Cache { return p.cache }

// Diag returns the diagnostic sink shared across every phase.
func (p *Pipeline) Diag() *diag.Sink { return p.diag }

// Verbose reports whether verbose-mode diagnostics are enabled.
func (p *Pipeline) Verbose() bool { return p.verbose }

// Plan returns the resolved execution order — annotators and
// generators grouped into priority buckets and topo-sorted within
// each bucket. "eidos explain plan" tooling reads this to display
// the resolved ordering without running the pipeline.
func (p *Pipeline) Plan() *Plan { return p.plan }

// LayoutPolicyFor returns the resolved [LayoutPolicy] for the
// named plugin. The result is composed by merging framework
// default + project config + per-plugin override + CLI overrides
// field by field; the policy is pinned at [Builder.Build] time so
// repeated calls for the same pluginName on the same pipeline
// instance return the same value. Plugin names the Builder didn't
// see at construction time (defensive lookups by tooling that
// queries before the plugin set is finalised) resolve to the
// project + CLI merge — the per-plugin layer is skipped because
// no override is registered.
func (p *Pipeline) LayoutPolicyFor(pluginName string) LayoutPolicy {
	if pol, ok := p.pluginPolicies[pluginName]; ok {
		return pol
	}
	return p.defaultPolicy
}

// LayoutPolicyForTag returns the resolved [LayoutPolicy] for the
// (pluginName, outputTag) pair, preferring a per-(plugin, tag)
// policy when one is registered (the `plugins[*].output.tags.<tag>`
// block in `.eidos.yaml`) and falling back to
// [Pipeline.LayoutPolicyFor] otherwise. An empty outputTag, or a
// non-empty tag absent from the per-tag map, both resolve through
// the per-plugin fallback.
func (p *Pipeline) LayoutPolicyForTag(pluginName, outputTag string) LayoutPolicy {
	if outputTag != "" {
		if pol, ok := p.pluginTagPolicies[pluginTagKey{plugin: pluginName, tag: outputTag}]; ok {
			return pol
		}
	}
	return p.LayoutPolicyFor(pluginName)
}

// OutputFilename returns the literal filename pinned by
// [Builder.WithOutputFilename] (or the legacy unscoped CLI `-o`
// flag). Empty means no global override is in effect and each
// decl resolves Filename from its origin basename + the
// contributing plugin's filename suffix, then through any
// per-plugin override exposed by [Pipeline.PluginOutputFilename].
func (p *Pipeline) OutputFilename() string { return p.outFilename }

// pluginTagKey is the (plugin, tag) composite key for the
// per-plugin CLI override map. Tag is empty for plugin-only
// overrides; non-empty for per-(plugin, tag) overrides.
type pluginTagKey struct {
	plugin string
	tag    string
}

// PluginOutputFilename returns the CLI `-o <plugin>[:<tag>]=<path>`
// override registered for (pluginName, outputTag), preferring the
// more specific (plugin, tag) override over the plugin-only
// (plugin, "") form. Returns ("", false) when no per-plugin
// override applies — callers should fall back to
// [Pipeline.OutputFilename] for the legacy unscoped form.
func (p *Pipeline) PluginOutputFilename(pluginName, outputTag string) (string, bool) {
	if len(p.pluginOutFilenames) == 0 {
		return "", false
	}
	if path, ok := p.pluginOutFilenames[pluginTagKey{plugin: pluginName, tag: outputTag}]; ok {
		return path, true
	}
	if outputTag != "" {
		if path, ok := p.pluginOutFilenames[pluginTagKey{plugin: pluginName}]; ok {
			return path, true
		}
	}
	return "", false
}

// TargetSymbol returns the symbol name pinned by
// [Builder.WithTargetSymbol] (or the CLI `-target` flag). Empty
// means no scope filter is active; every source decl participates.
func (p *Pipeline) TargetSymbol() string { return p.targetSym }

// Scope returns the [store.ScopePredicate] every per-plugin
// Reader is constructed with, or nil when no scope filter is
// configured. Nil means range queries through the Reader
// observe every source node — the framework default.
func (p *Pipeline) Scope() store.ScopePredicate { return p.scope }

// DirectiveRegistry returns the [directive.Registry] populated at
// Build time from every schema supplied via [Builder.WithDirective].
// Frontends consult the registry while parsing source comments to
// validate every directive against its schema; tooling can also
// enumerate registered directives for documentation.
func (p *Pipeline) DirectiveRegistry() *directive.Registry { return p.registry }

// The keys the Layout phase writes into
// [manifest.ResolvedLayout.ResolvedFrom], one per composed Target
// field. They are a wire contract — `eidos explain` and drift tooling
// read them out of the manifest — so they are named once rather than
// spelled as literals at the point of assembly.
const (
	resolvedFromLayout   = "layout"
	resolvedFromPackage  = "package"
	resolvedFromDir      = "dir"
	resolvedFromFilename = "filename"
)

// layerSet is the four manifest layers composeTarget resolves, as a
// comparable value.
//
// The same information used to travel as a map[string]manifest.Layer
// built per routed declaration. As a struct it costs nothing to
// carry, compares in one instruction instead of a ranged lookup, and
// is what makes the record path's memo sound: memoising on Target
// alone would suppress the divergence diagnostic this function exists
// to emit.
type layerSet struct {
	Filename manifest.Layer
	Layout   manifest.Layer
	Dir      manifest.Layer
	Package  manifest.Layer
}

// resolvedEntry is what the phase stores per Target: the manifest
// value plus the comparable layers it was composed from.
type resolvedEntry struct {
	rl     manifest.ResolvedLayout
	layers layerSet
}

// resolved returns the manifest form, materialising ResolvedFrom.
//
// Built here rather than in composeTarget so the four-entry map is
// allocated once per output file instead of once per declaration
// routed into it. The keys are a wire contract — `eidos explain` and
// drift tooling read them out of manifest.json — so they are spelled
// out rather than derived.
func (e resolvedEntry) resolved() manifest.ResolvedLayout {
	out := e.rl
	out.ResolvedFrom = map[string]manifest.Layer{
		resolvedFromFilename: e.layers.Filename,
		resolvedFromLayout:   e.layers.Layout,
		resolvedFromDir:      e.layers.Dir,
		resolvedFromPackage:  e.layers.Package,
	}
	return out
}

// resolvedLayoutFor returns the routing decision the Layout phase
// composed for target, or the zero value when no entry was
// recorded. The manifest sink consults this map at run-end to
// attach the observability block to each output.
func (p *Pipeline) resolvedLayoutFor(target emit.Target) (manifest.ResolvedLayout, bool) {
	p.resolvedLayoutsMu.Lock()
	defer p.resolvedLayoutsMu.Unlock()
	e, ok := p.resolvedLayouts[target]
	if !ok {
		return manifest.ResolvedLayout{}, false
	}
	return e.resolved(), true
}

// hasLayoutActivity reports whether the Layout phase composed at
// least one Target during the most recent run. The manifest sink
// uses it to distinguish "routing engaged, backend mismatched" (a
// framework bug worth an Internal diagnostic) from "routing never
// engaged, backend wrote synthetic targets" (a test pattern with
// no observability metadata available).
func (p *Pipeline) hasLayoutActivity() bool {
	p.resolvedLayoutsMu.Lock()
	defer p.resolvedLayoutsMu.Unlock()
	return len(p.resolvedLayouts) > 0
}

// resolvedRecorder accumulates the Layout phase's per-Target routing
// decisions for the duration of the phase.
//
// It replaces a mutex-guarded write to the Pipeline taken once per
// routed declaration. runLayout is a straight-line sequence on one
// goroutine, so no Layout writer is ever concurrent with another and
// the lock defended against nothing; the Pipeline's mutex still
// guards the publish and every post-Run read, which is the job its
// docblock always described.
type resolvedRecorder struct {
	diag    *diag.Sink
	entries map[emit.Target]resolvedEntry

	// last memoises the previous (Target, layers) pair. routeDecls
	// walks each bucket in insertion order, so declarations composing
	// into one file arrive adjacently and a repeat is the common
	// case for any generator emitting more than one decl per file.
	//
	// Memoising on Target alone would be wrong: it would skip the
	// map and therefore suppress the divergence diagnostic, which is
	// the one thing this path exists to emit. Comparing the layers
	// too is what makes the skip sound, and is only possible because
	// they are a comparable value rather than a map.
	last    emit.Target
	lastSet layerSet
	lastOK  bool
}

// record notes the routing decision composed for target.
//
// The first decl routed to a Target stores it; later decls composing
// the same Target verify agreement and drop their value. A
// disagreement is a framework bug — two plugins resolving one output
// file under different policies — and surfaces as an Internal
// diagnostic, which counts toward HasErrors and fails the run.
func (r *resolvedRecorder) record(target emit.Target, rl manifest.ResolvedLayout, layers layerSet) {
	if r.lastOK && r.last == target && r.lastSet == layers {
		return
	}
	if existing, ok := r.entries[target]; ok {
		if existing.layers != layers || existing.rl.Layout != rl.Layout {
			r.diag.Internalf(position.Pos{},
				"pipeline.layout: divergent ResolvedLayout for target %+v: existing=%+v new=%+v",
				target, existing.resolved(), rl)
		}
		r.last, r.lastSet, r.lastOK = target, layers, true
		return
	}
	r.entries[target] = resolvedEntry{rl: rl, layers: layers}
	r.last, r.lastSet, r.lastOK = target, layers, true
}
