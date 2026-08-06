// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// Composition fingerprints the frontend suite drives its cache
// passes with. Both values are the suite's to choose rather than the
// fixture author's: a fixture leaving [FrontendFixture.Fingerprint]
// empty still gets two distinct compositions driven, and one that
// sets it still gets a guaranteed-distinct second value because
// recomposedFingerprintSuffix is appended rather than substituted.
const (
	// defaultFingerprint stands in for a real pipeline composition
	// hash when a fixture declares none. Its value is arbitrary;
	// only its difference from the recomposed form is load-bearing.
	defaultFingerprint = "plugintest-composition"

	// recomposedFingerprintSuffix models "the plugin set changed
	// between runs" — the one event [plugin.FrontendContext.Fingerprint]
	// exists to propagate into a frontend's cache key.
	recomposedFingerprintSuffix = "+recomposed"
)

// FrontendFixture describes a single source-loading scenario
// the [RunFrontendSuite] drives a [plugin.Frontend] against.
// The fixture declares the user-facing Pattern and the
// per-plugin Options the frontend's [plugin.OptionsProvider]
// (when implemented) decodes via SetOptions.
//
// Frontends typically encode their input root through Options
// (`dir`, `root`, `entrypoint`, …) rather than through the
// pattern — patterns scope what to load within that root. The
// suite calls SetOptions once with the supplied map before each
// Load invocation, so the frontend re-receives its configured
// values on every pass, and a rejection fails the check that
// asked for them rather than skipping Load in silence.
type FrontendFixture struct {
	// Name labels the fixture in subtest paths and failure
	// messages. Required and unique within a single
	// [RunFrontendSuite] call.
	Name string

	// Pattern is the literal value supplied to
	// [plugin.FrontendContext.Pattern]. Typically "./..." or a
	// language-specific glob; the frontend interprets it
	// language-appropriately.
	Pattern string

	// Options is forwarded verbatim to the frontend's
	// [plugin.OptionsProvider.SetOptions] when the frontend
	// implements that capability. Frontends that do not
	// implement OptionsProvider ignore this map; the suite
	// surfaces a contract failure if the frontend implements the
	// interface but rejects the supplied values.
	Options map[string]string

	// ExpectsEmpty declares that this fixture is meant to record
	// no nodes at all, so the suite's populate check tolerates an
	// empty store rather than reporting it.
	//
	// The zero value is the safe one: a fixture that loads
	// nothing is nearly always a fixture whose input path is
	// wrong, and that failure is otherwise invisible — every
	// downstream check compares empty against empty and agrees.
	// Set this only when "nothing to load" is the outcome the
	// fixture exists to pin.
	ExpectsEmpty bool

	// AllowsPositionlessDiagnostics waives the requirement that
	// every diagnostic the frontend emits on this fixture carries a
	// source position.
	//
	// This is the role that most often needs it. A frontend's
	// run-level failures — a pattern that resolves to nothing, a
	// package the toolchain refuses to load — name no source
	// construct, so there is no line to point at and inventing one
	// would be worse than the dash. Per-declaration complaints are a
	// different matter: those have a position and the check exists
	// to keep them carrying it.
	//
	// It does not waive the no-Error-severity contract; that one is
	// not negotiable per fixture.
	AllowsPositionlessDiagnostics bool

	// Fingerprint is the composition fingerprint the suite stamps
	// on [plugin.FrontendContext.Fingerprint] for the first of its
	// two cache passes. Leave it empty and the suite picks one.
	//
	// The suite derives the second pass's value from this one, so
	// setting it never disables the changed-composition check —
	// it only lets a fixture whose frontend is sensitive to the
	// literal string choose a realistic shape.
	Fingerprint string
}

// RunFrontendSuite runs the conformance checks every
// [plugin.Frontend] must satisfy: Load must not panic on an
// empty / minimal context; for each fixture, Load must succeed
// without panicking, must record something, must be
// deterministic — two invocations driven against equivalent
// contexts (independent stores, same pattern, same options) must
// produce equivalent node graphs — must not replay a cached
// graph across a change of composition fingerprint, and must
// surface no Error-severity diagnostics on a fixture it declares
// it handles, with every diagnostic it does emit carrying a
// source position.
//
// The empty-pattern probe is deliberately outside the diagnostic
// contract: a frontend is permitted to reject an empty pattern, and
// complaining about it is the conforming behaviour.
//
// The suite calls [plugin.OptionsProvider.SetOptions] before
// every Load when the frontend implements the capability, so
// the fixture's Options apply to every probe including the
// empty-pattern one, which inherits the first fixture's Options.
// A frontend that rejects those Options fails the check that
// asked for them rather than silently skipping Load. Pass an
// empty fixture slice to run only the empty-pattern contract.
//
// The cache passes run against a [cache.Disk] rooted in
// t.TempDir(), so a frontend that writes on Put touches the
// filesystem here where earlier releases handed it a
// [cache.None] that discarded everything.
func RunFrontendSuite(t *testing.T, f plugin.Frontend, fixtures []FrontendFixture) {
	t.Helper()
	assertFrontendFixtureNamesUnique(t, fixtures)
	t.Run("Load on empty pattern does not panic", func(t *testing.T) {
		assertLoadEmptyPatternDoesNotPanic(t, f, firstFixture(fixtures))
	})
	for _, fx := range fixtures {
		t.Run("fixture="+fx.Name+"/Load does not panic", func(t *testing.T) {
			assertLoadDoesNotPanic(t, f, fx)
		})
		t.Run("fixture="+fx.Name+"/Load populates the store", func(t *testing.T) {
			assertLoadPopulatesStore(t, f, fx)
		})
		t.Run("fixture="+fx.Name+"/Load produces no Error-severity diagnostics", func(t *testing.T) {
			assertLoadCarriesNoErrors(t, f, fx)
		})
		t.Run("fixture="+fx.Name+"/Load diagnostics carry a source position", func(t *testing.T) {
			assertLoadDiagnosticsArePositioned(t, f, fx)
		})
		t.Run("fixture="+fx.Name+"/Load is deterministic across two runs", func(t *testing.T) {
			assertLoadIsDeterministic(t, f, fx)
		})
		t.Run("fixture="+fx.Name+"/Load re-parses when the composition fingerprint changes", func(t *testing.T) {
			assertLoadIsFingerprintKeyed(t, f, fx)
		})
	}
}

// firstFixture returns the fixture the empty-pattern probe borrows
// its Options from, or the zero fixture when the caller supplied
// none. The probe needs options a required-option frontend accepts:
// driven with a nil map it fails SetOptions and never reaches Load,
// which is the whole contract it exists to check.
func firstFixture(fixtures []FrontendFixture) FrontendFixture {
	if len(fixtures) == 0 {
		return FrontendFixture{}
	}
	return fixtures[0]
}

// fingerprintPair returns the two composition fingerprints the cache
// checks drive fx against — the fixture's own (or the suite default)
// and a recomposed derivative guaranteed to differ from it.
func fingerprintPair(fx FrontendFixture) (first, second string) {
	first = fx.Fingerprint
	if first == "" {
		first = defaultFingerprint
	}
	return first, first + recomposedFingerprintSuffix
}

// withFingerprint returns a copy of fx carrying fp. The copy is
// shallow — Options is shared with the original and read-only on
// every path the suite drives.
func withFingerprint(fx FrontendFixture, fp string) FrontendFixture {
	fx.Fingerprint = fp
	return fx
}

// assertFrontendFixtureNamesUnique fails when two fixtures
// share a Name.
func assertFrontendFixtureNamesUnique(tb testing.TB, fixtures []FrontendFixture) {
	tb.Helper()
	seen := make(map[string]struct{}, len(fixtures))
	for _, fx := range fixtures {
		if fx.Name == "" {
			tb.Fatalf("RunFrontendSuite: fixture has empty Name; every FrontendFixture must declare one")
		}
		if _, dup := seen[fx.Name]; dup {
			tb.Fatalf("RunFrontendSuite: duplicate fixture Name %q", fx.Name)
		}
		seen[fx.Name] = struct{}{}
	}
}

// assertLoadEmptyPatternDoesNotPanic drives the frontend with
// an empty pattern and an otherwise minimal context. The
// frontend is permitted to fail (returning a non-nil error or
// surfacing diagnostics) on an empty pattern — the contract
// here is the narrower no-panic invariant. Panics on an empty
// pattern crash the process on projects whose patterns expand
// to nothing.
//
// The sink is discarded here and only here. The generator and
// annotator empty-store probes read theirs, because a plugin with
// nothing to do has nothing to complain about; a frontend handed no
// pattern has been asked for something impossible, and rejecting it
// loudly is what the role is supposed to do. Reading the sink here
// would fail conforming frontends for their conformance.
//
// fx supplies the Options only; its Pattern is cleared here, so
// callers hand over a real fixture rather than assembling a
// pattern-less copy. A frontend whose schema declares a required
// field would otherwise reject the probe's empty option map and
// never reach Load — the probe would then assert nothing at all.
func assertLoadEmptyPatternDoesNotPanic(tb testing.TB, f plugin.Frontend, fx FrontendFixture) {
	tb.Helper()
	fx.Pattern = ""
	res := runLoadRecovering(f, fx, store.New(), cache.NewNone(), diag.Discard())
	failOnRejectedFixtureOptions(tb, fx, res)
	if res.panicked {
		tb.Errorf("Load panicked on empty pattern: %v", res.panicValue)
	}
}

// assertLoadCarriesNoErrors drives the frontend against the fixture's
// pattern and fails when the diagnostic sink records any
// Error-severity entry. Mirrors [assertRenderCarriesNoErrors], whose
// rationale transfers verbatim: the fixture is the author's own
// declaration that this input loads cleanly, and an Error diagnostic
// on it fails every user run through pipeline.ErrRunHadErrors.
//
// A non-nil Load return is not itself a failure here — the frontend
// role reserves it for what the suite cannot classify — but a run
// that ended in a panic is, because a sink from a half-executed Load
// says nothing about the contract.
func assertLoadCarriesNoErrors(tb testing.TB, f plugin.Frontend, fx FrontendFixture) {
	tb.Helper()
	d := diag.Capture()
	res := runLoadRecovering(f, fx, store.New(), cache.NewNone(), d)
	failOnRejectedFixtureOptions(tb, fx, res)
	if res.panicked {
		tb.Fatalf("Load panicked on fixture %q: %v", fx.Name, res.panicValue)
	}
	reportErrorDiagnostics(tb, roleFrontend, fixtureSubject(fx.Name), d)
}

// assertLoadDiagnosticsArePositioned drives the frontend against the
// fixture's pattern and fails when any diagnostic it emitted carries
// a zero position, unless the fixture waived the check through
// [FrontendFixture.AllowsPositionlessDiagnostics].
//
// The frontend role is the one that documents the positioned
// requirement (see [plugin.Frontend]), which is why the check lands
// here rather than only on the roles whose docs are silent.
func assertLoadDiagnosticsArePositioned(tb testing.TB, f plugin.Frontend, fx FrontendFixture) {
	tb.Helper()
	d := diag.Capture()
	res := runLoadRecovering(f, fx, store.New(), cache.NewNone(), d)
	failOnRejectedFixtureOptions(tb, fx, res)
	if res.panicked {
		tb.Fatalf("Load panicked on fixture %q: %v", fx.Name, res.panicValue)
	}
	reportPositionlessDiagnostics(tb, roleFrontend, fixtureSubject(fx.Name), d, fx.AllowsPositionlessDiagnostics)
}

// assertLoadDoesNotPanic drives the frontend against the
// fixture's pattern and fails if Load panics. Returned errors
// are not a contract failure on their own — frontends surface
// per-input issues through ctx.Diag and reserve the return
// value for catastrophic failures the suite can't classify.
func assertLoadDoesNotPanic(tb testing.TB, f plugin.Frontend, fx FrontendFixture) {
	tb.Helper()
	res := runLoadRecovering(f, fx, store.New(), cache.NewNone(), diag.Discard())
	failOnRejectedFixtureOptions(tb, fx, res)
	if res.panicked {
		tb.Errorf("Load panicked on fixture %q: %v", fx.Name, res.panicValue)
	}
}

// assertLoadPopulatesStore fails when Load records nothing and the
// fixture has not declared that outcome through
// [FrontendFixture.ExpectsEmpty].
//
// Without this check every other frontend contract holds vacuously:
// two empty projections compare equal, an empty graph panics
// nowhere, and a suite that never reached Load reports the same
// green as one that loaded a thousand declarations. It is the only
// check here that distinguishes "the frontend works" from "the
// frontend ran".
func assertLoadPopulatesStore(tb testing.TB, f plugin.Frontend, fx FrontendFixture) {
	tb.Helper()
	s := store.New()
	res := runLoadRecovering(f, fx, s, cache.NewNone(), diag.Discard())
	failOnRejectedFixtureOptions(tb, fx, res)
	if res.panicked {
		tb.Fatalf("Load panicked on fixture %q: %v", fx.Name, res.panicValue)
	}
	if fx.ExpectsEmpty || len(nodeProjection(s)) > 0 {
		return
	}
	tb.Errorf(
		"Load recorded no nodes for fixture %q (pattern=%q, Load returned %v); "+
			"every downstream contract passes vacuously against an empty store. "+
			"Check the fixture's input path, or set FrontendFixture.ExpectsEmpty "+
			"when loading nothing is the outcome this fixture pins",
		fx.Name, fx.Pattern, res.loadErr,
	)
}

// assertLoadIsDeterministic drives Load twice against fresh
// stores and compares the resulting node-graph projections.
// The projection is a sorted slice of stable identity tuples
// covering every indexed node — kind, qualified name, package
// — so the diff catches missing or extra nodes the frontend
// produced inconsistently across runs.
//
// Both passes share one [cache.Disk] rooted in tb.TempDir(), so
// the second runs warm against whatever the first wrote. That
// sharing is the check: a frontend whose cache round-trip loses a
// node — a field the marshalled form drops, an owner back-pointer
// never rewired — produces a graph on the hit path that differs
// from the one it parsed, and no in-tree test observed that while
// the suite handed every frontend a [cache.None] that could not hit.
//
// Per-node detail (positions, doc comments, directive args) is
// outside the determinism check's scope: downstream tests
// assert against full source mapping through [frontendtest],
// where divergences surface with line-level context. The
// projection here pins the structural-determinism property the
// pipeline's cache and incremental-rebuild paths rely on.
//
// Allocates a temp directory per invocation; the testing package
// removes it when the test that owns tb finishes.
func assertLoadIsDeterministic(tb testing.TB, f plugin.Frontend, fx FrontendFixture) {
	tb.Helper()
	shared := cache.NewDisk(tb.TempDir())
	first, _ := fingerprintPair(fx)
	pass := withFingerprint(fx, first)

	cold := store.New()
	res := runLoadRecovering(f, pass, cold, shared, diag.Discard())
	failOnRejectedFixtureOptions(tb, fx, res)
	if res.panicked {
		tb.Fatalf("Load panicked on first determinism pass of fixture %q: %v", fx.Name, res.panicValue)
	}
	// A return error from Load is permitted (the frontend may
	// surface contract failures through it). The determinism check
	// still runs against the partial store the frontend populated;
	// only a panic aborts.
	warm := store.New()
	res = runLoadRecovering(f, pass, warm, shared, diag.Discard())
	failOnRejectedFixtureOptions(tb, fx, res)
	if res.panicked {
		tb.Fatalf("Load panicked on second determinism pass of fixture %q: %v", fx.Name, res.panicValue)
	}

	coldProj := nodeProjection(cold)
	warmProj := nodeProjection(warm)
	if !slices.Equal(coldProj, warmProj) {
		tb.Errorf(
			"node projection differs across two runs of fixture %q; frontend is not deterministic\n"+
				"  first run:  %s\n  second run: %s",
			fx.Name,
			strings.Join(coldProj, ", "), strings.Join(warmProj, ", "),
		)
	}
}

// assertLoadIsFingerprintKeyed pins the frontend contract's only
// capitalised MUST: a frontend that caches a parsed node graph folds
// [plugin.FrontendContext.Fingerprint] into its cache key.
//
// Two passes share one [cache.Disk]: a cold pass under the fixture's
// fingerprint, then a pass under a recomposed one. Between them the
// probe stops recording writes and starts watching reads, so a Get
// that hits an entry the cold pass wrote is a frontend serving a
// graph parsed for a different plugin set — the stale-cache failure
// the field exists to close, and the one `--no-cache` was the
// workaround for.
//
// Reading the reuse rather than counting hits is deliberate. A
// frontend that never touches ctx.Cache is conformant; it records no
// keys, so it passes here without the suite mandating that it cache
// anything. The projection comparison alone cannot do this job: the
// two passes drive identical input, so a mis-keyed frontend replays a
// graph identical to the one the check would otherwise compare
// against.
//
// Known limit: a frontend that deliberately shares an entry across
// compositions — auxiliary data whose validity genuinely does not
// depend on the plugin set — trips this. The contract admits no such
// entry today, so the suite reports it rather than guessing.
//
// Allocates a temp directory per invocation; the testing package
// removes it when the test that owns tb finishes.
func assertLoadIsFingerprintKeyed(tb testing.TB, f plugin.Frontend, fx FrontendFixture) {
	tb.Helper()
	probe := newFingerprintProbe(cache.NewDisk(tb.TempDir()))
	first, second := fingerprintPair(fx)

	cold := store.New()
	res := runLoadRecovering(f, withFingerprint(fx, first), cold, probe, diag.Discard())
	failOnRejectedFixtureOptions(tb, fx, res)
	if res.panicked {
		tb.Fatalf("Load panicked on the cold fingerprint pass of fixture %q: %v", fx.Name, res.panicValue)
	}

	probe.recompose()

	recomposed := store.New()
	res = runLoadRecovering(f, withFingerprint(fx, second), recomposed, probe, diag.Discard())
	failOnRejectedFixtureOptions(tb, fx, res)
	if res.panicked {
		tb.Fatalf("Load panicked on the recomposed fingerprint pass of fixture %q: %v", fx.Name, res.panicValue)
	}

	if reused := probe.reusedKeys(); len(reused) > 0 {
		tb.Errorf(
			"fixture %q: Load read back cache entries written under composition fingerprint %q "+
				"while running under %q; plugin.FrontendContext.Fingerprint MUST be folded into "+
				"the cache key, or an upgraded plugin set is served a graph parsed before it existed\n"+
				"  reused keys: %s",
			fx.Name, first, second, strings.Join(reused, ", "),
		)
	}

	coldProj := nodeProjection(cold)
	recomposedProj := nodeProjection(recomposed)
	if !slices.Equal(coldProj, recomposedProj) {
		tb.Errorf(
			"node projection differs after the composition fingerprint changed for fixture %q; "+
				"Load must record the same nodes whatever the plugin set\n"+
				"  cold pass:       %s\n  recomposed pass: %s",
			fx.Name,
			strings.Join(coldProj, ", "), strings.Join(recomposedProj, ", "),
		)
	}
}

// loadResult records how one probing Load invocation ended.
//
// Three outcomes travel through three fields rather than through one
// error whose text each caller re-classifies with a string prefix.
// Under the prefix scheme a rejected fixture matched no prefix and
// was discarded — the suite had no vocabulary for "the fixture was
// wrong", so it spelled it "not a panic" and reported green with
// Load never invoked. Adding a field costs one compile error at every
// site that must decide; adding a prefix costs nothing and is
// therefore silently forgotten.
type loadResult struct {
	// panicked reports that SetOptions or Load panicked. The
	// recovered value is in panicValue.
	panicked bool

	// panicValue is whatever recover returned; only meaningful when
	// panicked is set.
	panicValue any

	// optionsErr is the frontend's own rejection of the fixture's
	// Options. Non-nil means Load was never invoked, so every
	// assertion downstream of it is meaningless.
	optionsErr error

	// loadErr is Load's return value. Not a contract failure on its
	// own — frontends surface per-input issues through ctx.Diag and
	// reserve the return for failures the suite cannot classify —
	// but worth reporting alongside one.
	loadErr error
}

// failOnRejectedFixtureOptions aborts the calling check when the
// frontend's own schema rejected the fixture's Options. Mirrors
// [assertOptionsFixtureCoversRequired]: a fixture the plugin refuses
// is a fixture-shape failure, and continuing past it asserts
// properties of a store nothing ever wrote to.
//
// Fatal rather than an error: every remaining assertion in the check
// would run against an empty store and report a second, misleading
// failure.
func failOnRejectedFixtureOptions(tb testing.TB, fx FrontendFixture, res loadResult) {
	tb.Helper()
	if res.optionsErr == nil {
		return
	}
	tb.Fatalf(
		"FrontendFixture %q: the frontend rejected the fixture's Options: %v (options=%v); "+
			"populate them so Load is actually driven — a rejected fixture skips Load entirely "+
			"and leaves every check downstream comparing empty against empty",
		fx.Name, res.optionsErr, fx.Options,
	)
}

// runLoadRecovering applies fixture options (when the frontend
// supports OptionsProvider) and invokes Load against s, c and d.
// Panics from either call are recovered into the returned
// [loadResult] rather than propagating: a fixture that crashes one
// frontend must not take the suite's remaining checks down with it.
//
// The sink is a parameter rather than a local because a check that
// cannot reach it cannot assert on it. Pass [diag.Capture] to inspect
// what was emitted, [diag.Discard] when the check is about panics,
// node counts or cache keys and the diagnostics belong to a sibling
// check.
func runLoadRecovering(
	f plugin.Frontend,
	fx FrontendFixture,
	s *store.Store,
	c cache.Cache,
	d *diag.Sink,
) (res loadResult) {
	defer func() {
		if r := recover(); r != nil {
			res.panicked = true
			res.panicValue = r
		}
	}()
	if provider, ok := any(f).(plugin.OptionsProvider); ok {
		if err := provider.SetOptions(opt.New(provider.OptionsSchema(), fx.Options)); err != nil {
			res.optionsErr = err
			return res
		}
	}
	res.loadErr = f.Load(&plugin.FrontendContext{
		Store:       s,
		Diag:        d,
		Registry:    directive.NewRegistry(),
		Parser:      directive.DefaultParser(),
		Cache:       c,
		Pattern:     fx.Pattern,
		Fingerprint: fx.Fingerprint,
	})
	return res
}

// fingerprintProbe wraps a [cache.Cache] and records whether the
// frontend read back an entry it had written under an earlier
// composition fingerprint.
//
// The probe has two phases. Before [fingerprintProbe.recompose] every
// Put is recorded as part of the baseline composition; after it, any
// Get that hits a baseline key is recorded as reuse. Keys are
// otherwise opaque — the probe reads no structure into them, so it
// stays correct for whatever key shape a frontend composes.
//
// Safe for concurrent use: a frontend may parse packages in
// parallel, so both maps are guarded. The wrapped cache's own I/O
// happens outside the lock, so a slow disk does not serialise the
// frontend's parallelism.
type fingerprintProbe struct {
	inner cache.Cache

	mu       sync.Mutex
	baseline map[string]struct{}
	reused   map[string]struct{}
	probing  bool
}

// newFingerprintProbe returns a probe wrapping inner, in its
// baseline-recording phase.
func newFingerprintProbe(inner cache.Cache) *fingerprintProbe {
	return &fingerprintProbe{
		inner:    inner,
		baseline: make(map[string]struct{}),
		reused:   make(map[string]struct{}),
	}
}

// Get delegates to the wrapped cache and, once probing, records a hit
// on a baseline key as reuse across the composition change.
func (p *fingerprintProbe) Get(key string) ([]byte, bool) {
	body, ok := p.inner.Get(key)
	p.mu.Lock()
	defer p.mu.Unlock()
	if ok && p.probing {
		if _, baseline := p.baseline[key]; baseline {
			p.reused[key] = struct{}{}
		}
	}
	return body, ok
}

// Put records the key as part of the baseline composition while the
// probe is still recording, then delegates to the wrapped cache.
func (p *fingerprintProbe) Put(key string, value []byte) error {
	p.mu.Lock()
	if !p.probing {
		p.baseline[key] = struct{}{}
	}
	p.mu.Unlock()
	if err := p.inner.Put(key, value); err != nil {
		return fmt.Errorf("plugintest: fingerprint probe put: %w", err)
	}
	return nil
}

// recompose closes the baseline phase. Every subsequent hit on a
// baseline key counts as reuse.
func (p *fingerprintProbe) recompose() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.probing = true
}

// reusedKeys returns the baseline keys the frontend read back after
// the composition changed, sorted so failure output is stable.
func (p *fingerprintProbe) reusedKeys() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := slices.Collect(maps.Keys(p.reused))
	slices.Sort(out)
	return out
}

// nodeOwnerName returns the qualified name of a source-node
// owner pointer (Method.Owner / Field.Owner). The owner is
// always a kind that implements QName when set; nil owners
// surface as [unownedSentinel] so failure output stays readable.
func nodeOwnerName(owner any) string {
	if owner == nil {
		return unownedSentinel
	}
	if q, ok := owner.(interface{ QName() string }); ok {
		return q.QName()
	}
	return unnamedSentinel
}

// nodeProjection returns a sorted slice of stable identity
// strings — one per indexed node in s — covering every kind the
// suite recognises. Mirrors [emitProjection] in shape; the
// frontend suite uses it for determinism comparison.
func nodeProjection(s *store.Store) []string {
	nv := s.Nodes()
	total := nv.Packages().Len() + nv.Files().Len() + nv.Imports().Len() +
		nv.Structs().Len() + nv.Interfaces().Len() + nv.Methods().Len() +
		nv.Fields().Len() + nv.Functions().Len() + nv.Variables().Len() +
		nv.Constants().Len() + nv.Enums().Len() + nv.EnumVariants().Len() +
		nv.Aliases().Len()
	out := make([]string, 0, total)
	for _, n := range nv.Packages().Items() {
		out = append(out, fmt.Sprintf("package:%s:%s", n.Name, n.Path))
	}
	for _, n := range nv.Files().Items() {
		out = append(out, "file:"+n.Path)
	}
	for _, n := range nv.Imports().Items() {
		out = append(out, fmt.Sprintf("import:%s:alias=%s", n.Path, n.Alias))
	}
	for _, n := range nv.Structs().Items() {
		out = append(out, "struct:"+n.QName())
	}
	for _, n := range nv.Interfaces().Items() {
		out = append(out, "interface:"+n.QName())
	}
	for _, n := range nv.Methods().Items() {
		out = append(out, fmt.Sprintf("method:%s.%s", nodeOwnerName(n.Owner), n.Name))
	}
	for _, n := range nv.Fields().Items() {
		out = append(out, fmt.Sprintf("field:%s.%s", nodeOwnerName(n.Owner), n.Name))
	}
	for _, n := range nv.Functions().Items() {
		out = append(out, "function:"+n.QName())
	}
	for _, n := range nv.Variables().Items() {
		out = append(out, "variable:"+n.QName())
	}
	for _, n := range nv.Constants().Items() {
		out = append(out, "constant:"+n.QName())
	}
	for _, n := range nv.Enums().Items() {
		out = append(out, "enum:"+n.QName())
	}
	for _, n := range nv.EnumVariants().Items() {
		owner := unownedSentinel
		if n.Owner != nil {
			owner = n.Owner.QName()
		}
		out = append(out, fmt.Sprintf("enum-variant:%s.%s", owner, n.Name))
	}
	for _, n := range nv.Aliases().Items() {
		out = append(out, "alias:"+n.QName())
	}
	slices.Sort(out)
	return out
}
