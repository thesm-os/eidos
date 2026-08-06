// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// AnnotatorFixture describes a single input scenario the
// [RunAnnotatorSuite] drives a [plugin.Annotator] against.
//
// BuildStore is invoked once per subtest that exercises this
// fixture, so the returned [*store.Store] must be a fresh,
// independent value each call — annotators mutate the store's
// meta state, and a shared store would leak state between
// subtests. The store should contain whichever source-side
// decls the annotator's stamping logic targets (typically built
// via the [storefixture.Builder] helpers).
type AnnotatorFixture struct {
	// Name labels the fixture in subtest paths and failure
	// messages. Required and unique within a single
	// [RunAnnotatorSuite] call.
	Name string

	// BuildStore returns a freshly-populated store. The function
	// is invoked once per subtest; tests fail fast through `t` on
	// builder errors rather than returning them.
	BuildStore func(t *testing.T) *store.Store

	// AllowsPositionlessDiagnostics waives the requirement that
	// every diagnostic the annotator emits on this fixture carries
	// a source position.
	//
	// The zero value is the strict one, because a positionless
	// diagnostic renders as a dash where file and line belong and
	// the reader cannot act on it. Set this only when the
	// annotator's complaints on this input genuinely name no source
	// construct. It does not waive the no-Error-severity contract;
	// that one is not negotiable per fixture.
	AllowsPositionlessDiagnostics bool
}

// RunAnnotatorSuite runs the conformance checks every
// [plugin.Annotator] must satisfy: it must not panic on an
// empty store, it must not add or remove nodes during the
// annotate phase (the source-side store is structurally
// frozen), its meta-stamping must be idempotent — running
// [plugin.Annotator.Annotate] twice on the same store must
// produce identical meta state on the second pass — and it must
// surface no Error-severity diagnostics on an input the fixture
// declares it handles, with every diagnostic it does emit
// carrying a source position.
//
// Fixtures supply realistic input scenarios. The suite drives
// the annotator against each in a dedicated subtest so failure
// attribution stays scoped. Pass an empty fixture slice to run
// only the empty-store contract.
//
// Build- or run-time failures (BuildStore returning a nil
// store, the annotator panicking on a fixture it claims to
// handle) surface through `t.Errorf` / `t.Fatalf` so the
// fixture name appears in the failure path.
func RunAnnotatorSuite(t *testing.T, a plugin.Annotator, fixtures []AnnotatorFixture) {
	t.Helper()
	t.Run("Annotate on empty store does not panic", func(t *testing.T) {
		assertAnnotateEmptyStoreDoesNotPanic(t, a)
	})
	t.Run("Annotate on empty store produces no Error-severity diagnostics", func(t *testing.T) {
		assertAnnotateEmptyStoreCarriesNoErrors(t, a)
	})
	assertAnnotatorFixtureNamesUnique(t, fixtures)
	for _, fx := range fixtures {
		t.Run("fixture="+fx.Name+"/Annotate does not panic", func(t *testing.T) {
			s := buildAnnotatorStore(t, fx)
			assertAnnotateDoesNotPanic(t, a, s)
		})
		t.Run("fixture="+fx.Name+"/Annotate produces no Error-severity diagnostics", func(t *testing.T) {
			s := buildAnnotatorStore(t, fx)
			assertAnnotateCarriesNoErrors(t, a, fx, s)
		})
		t.Run("fixture="+fx.Name+"/Annotate diagnostics carry a source position", func(t *testing.T) {
			s := buildAnnotatorStore(t, fx)
			assertAnnotateDiagnosticsArePositioned(t, a, fx, s)
		})
		t.Run("fixture="+fx.Name+"/node count unchanged by Annotate", func(t *testing.T) {
			s := buildAnnotatorStore(t, fx)
			assertAnnotateLeavesNodeCountUnchanged(t, a, s)
		})
		t.Run("fixture="+fx.Name+"/Annotate is idempotent across two runs", func(t *testing.T) {
			s := buildAnnotatorStore(t, fx)
			assertAnnotateIsIdempotent(t, a, s)
		})
		t.Run("fixture="+fx.Name+"/Annotate is deterministic across two equivalent stores", func(t *testing.T) {
			assertAnnotateIsDeterministic(t, a, buildAnnotatorStore(t, fx), buildAnnotatorStore(t, fx))
		})
	}
}

// assertAnnotatorFixtureNamesUnique fails when two fixtures
// share a Name. Duplicate names would produce identical
// subtest paths, masking which fixture triggered a failure.
func assertAnnotatorFixtureNamesUnique(tb testing.TB, fixtures []AnnotatorFixture) {
	tb.Helper()
	seen := make(map[string]struct{}, len(fixtures))
	for _, fx := range fixtures {
		if fx.Name == "" {
			tb.Fatalf("RunAnnotatorSuite: fixture has empty Name; every AnnotatorFixture must declare one")
		}
		if _, dup := seen[fx.Name]; dup {
			tb.Fatalf("RunAnnotatorSuite: duplicate fixture Name %q", fx.Name)
		}
		seen[fx.Name] = struct{}{}
	}
}

// buildAnnotatorStore invokes fx.BuildStore and surfaces nil /
// builder failures as test fatals. The returned store is the
// per-subtest copy the annotator drives against.
func buildAnnotatorStore(t *testing.T, fx AnnotatorFixture) *store.Store {
	t.Helper()
	if fx.BuildStore == nil {
		t.Fatalf("RunAnnotatorSuite: fixture %q has nil BuildStore", fx.Name)
	}
	s, err := buildFixtureStoreRecovering(func() *store.Store { return fx.BuildStore(t) })
	if err != nil {
		t.Fatalf("RunAnnotatorSuite: fixture %q: %v", fx.Name, err)
	}
	if s == nil {
		t.Fatalf("RunAnnotatorSuite: fixture %q BuildStore returned nil store", fx.Name)
	}
	return s
}

// assertAnnotateEmptyStoreDoesNotPanic drives the annotator
// against a fresh empty store. The contract is that an
// annotator with no source-side nodes to act on completes
// cleanly without panic — the pipeline runs annotators
// unconditionally and an empty-store panic is a runtime crash.
func assertAnnotateEmptyStoreDoesNotPanic(tb testing.TB, a plugin.Annotator) {
	tb.Helper()
	s := store.New()
	if err := runAnnotateRecovering(a, s, diag.Discard()); err != nil {
		tb.Errorf("Annotate %s on an empty store: %v", probeVerb(err), err)
	}
}

// assertAnnotateEmptyStoreCarriesNoErrors drives the annotator
// against a fresh empty store and fails when it complains about it.
//
// An annotator that reports an Error with nothing to stamp reports
// one on every project whose patterns expand to no matches, and the
// pipeline runs annotators unconditionally. The positioned-diagnostic
// check has no counterpart here: the probe carries no fixture to hang
// the waiver on, and an empty store has no source construct to name.
func assertAnnotateEmptyStoreCarriesNoErrors(tb testing.TB, a plugin.Annotator) {
	tb.Helper()
	d := diag.Capture()
	if err := runAnnotateRecovering(a, store.New(), d); err != nil {
		tb.Fatalf("Annotate did not complete on an empty store: %v", err)
	}
	reportErrorDiagnostics(tb, roleAnnotator, emptyStoreSubject, d)
}

// assertAnnotateDoesNotPanic drives the annotator against the
// fixture's store and fails if it panics. Diagnostics emitted
// through ctx.Diag are not an error path; the annotator is
// expected to surface contract violations through the
// diagnostic sink rather than panic.
func assertAnnotateDoesNotPanic(tb testing.TB, a plugin.Annotator, s *store.Store) {
	tb.Helper()
	if err := runAnnotateRecovering(a, s, diag.Discard()); err != nil {
		tb.Errorf("Annotate %s on the fixture store: %v", probeVerb(err), err)
	}
}

// assertAnnotateCarriesNoErrors drives the annotator against the
// fixture's store and fails when the diagnostic sink records any
// Error-severity entry. Mirrors [assertRenderCarriesNoErrors], whose
// rationale transfers verbatim: the fixtures a plugin author supplies
// represent inputs the plugin handles cleanly, and an Error
// diagnostic on one of them reaches the user as a non-zero exit on
// every run.
func assertAnnotateCarriesNoErrors(tb testing.TB, a plugin.Annotator, fx AnnotatorFixture, s *store.Store) {
	tb.Helper()
	d := diag.Capture()
	if err := runAnnotateRecovering(a, s, d); err != nil {
		tb.Fatalf("Annotate did not complete on fixture %q: %v", fx.Name, err)
	}
	reportErrorDiagnostics(tb, roleAnnotator, fixtureSubject(fx.Name), d)
}

// assertAnnotateDiagnosticsArePositioned drives the annotator against
// the fixture's store and fails when any diagnostic it emitted
// carries a zero position, unless the fixture waived the check
// through [AnnotatorFixture.AllowsPositionlessDiagnostics].
func assertAnnotateDiagnosticsArePositioned(
	tb testing.TB,
	a plugin.Annotator,
	fx AnnotatorFixture,
	s *store.Store,
) {
	tb.Helper()
	d := diag.Capture()
	if err := runAnnotateRecovering(a, s, d); err != nil {
		tb.Fatalf("Annotate did not complete on fixture %q: %v", fx.Name, err)
	}
	reportPositionlessDiagnostics(tb, roleAnnotator, fixtureSubject(fx.Name), d, fx.AllowsPositionlessDiagnostics)
}

// assertAnnotateLeavesNodeCountUnchanged pins the annotator's
// frozen-store contract: between the frontend phase and the
// generator phase, no plugin adds or removes nodes. The check
// counts every indexed node before and after a single Annotate
// invocation and fails on mismatch.
func assertAnnotateLeavesNodeCountUnchanged(tb testing.TB, a plugin.Annotator, s *store.Store) {
	tb.Helper()
	before := snapshotNodeCounts(s)
	if err := runAnnotateRecovering(a, s, diag.Discard()); err != nil {
		tb.Fatalf("Annotate %s during the node-count check: %v", probeVerb(err), err)
	}
	after := snapshotNodeCounts(s)
	if !nodeCountsEqual(before, after) {
		tb.Errorf(
			"Annotate changed indexed node counts: before=%v after=%v "+
				"(annotators must not add or remove nodes)",
			before, after,
		)
	}
}

// assertAnnotateIsIdempotent runs Annotate twice on the same
// store and fails when the second pass alters the meta state
// recorded after the first. The bag's JSON marshalling
// represents the current (winning) value at each (name,
// authority) slot — calling [meta.Bag.Set] a second time with
// the same value overwrites in place, so an idempotent
// annotator produces identical JSON across the two passes.
//
// The check uses [bytes.Equal] on the per-node JSON so any
// drift (changed value, changed setBy, changed position) shows
// up as a failure. The first node mismatch is reported with the
// per-side projections for debugging.
func assertAnnotateIsIdempotent(tb testing.TB, a plugin.Annotator, s *store.Store) {
	tb.Helper()
	if err := runAnnotateRecovering(a, s, diag.Discard()); err != nil {
		tb.Fatalf("Annotate %s on the first idempotency pass: %v", probeVerb(err), err)
	}
	first, err := snapshotMetaBags(s)
	if err != nil {
		tb.Fatalf("snapshotMetaBags after first pass: %v", err)
	}
	if rerr := runAnnotateRecovering(a, s, diag.Discard()); rerr != nil {
		tb.Fatalf("Annotate %s on the second idempotency pass: %v", probeVerb(rerr), rerr)
	}
	second, err := snapshotMetaBags(s)
	if err != nil {
		tb.Fatalf("snapshotMetaBags after second pass: %v", err)
	}
	reportFirstDifference(tb, first, second)
}

// assertAnnotateIsDeterministic drives the annotator against two
// independently built stores and fails when the resulting meta state
// differs.
//
// This is not what [assertAnnotateIsIdempotent] checks, and the two
// catch different defects. Idempotency runs twice over one store, so
// the ubiquitous already-stamped guard — "if the key is set, return" —
// makes the second pass a no-op and the comparison uninformative about
// how the value was computed. Determinism gives the annotator a store
// it has never seen, so a value derived from a counter, a map
// iteration, a timestamp or the host environment differs between the
// two and is caught.
//
// Generators and frontends have had the two-store check since they
// were written; the annotator phase — whose metadata every downstream
// generator keys on — had only the single-store one.
func assertAnnotateIsDeterministic(tb testing.TB, a plugin.Annotator, s1, s2 *store.Store) {
	tb.Helper()
	if err := runAnnotateRecovering(a, s1, diag.Discard()); err != nil {
		tb.Fatalf("Annotate %s on the first determinism pass: %v", probeVerb(err), err)
	}
	if err := runAnnotateRecovering(a, s2, diag.Discard()); err != nil {
		tb.Fatalf("Annotate %s on the second determinism pass: %v", probeVerb(err), err)
	}
	first, err := snapshotMetaBags(s1)
	if err != nil {
		tb.Fatalf("snapshotMetaBags after the first determinism pass: %v", err)
	}
	second, err := snapshotMetaBags(s2)
	if err != nil {
		tb.Fatalf("snapshotMetaBags after the second determinism pass: %v", err)
	}
	reportFirstDifference(tb, first, second)
}

// runAnnotateRecovering invokes Annotate against the caller's
// diagnostic sink and recovers any panic into a returned error.
// The plain Annotate error is wrapped on the same path so
// callers can distinguish "panicked" from "returned an error"
// by inspecting the wrapping verb.
//
// The sink is a parameter rather than a local because a check that
// cannot reach it cannot assert on it. Pass [diag.Capture] to inspect
// what was emitted, [diag.Discard] when the check is about panics or
// store state and the diagnostics belong to a sibling check.
func runAnnotateRecovering(a plugin.Annotator, s *store.Store, d *diag.Sink) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrProbePanicked, r)
		}
	}()
	ctx := &plugin.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   d,
	}
	if rerr := a.Annotate(ctx); rerr != nil {
		return fmt.Errorf("%w: %w", ErrProbeReturnedError, rerr)
	}
	return nil
}

// snapshotNodeCounts returns the per-bucket node count for s.
// The keys mirror the bucket method names on
// [store.NodeView] so a delta is greppable to the bucket that
// changed.
func snapshotNodeCounts(s *store.Store) map[string]int {
	nv := s.Nodes()
	return map[string]int{
		"Packages":     nv.Packages().Len(),
		"Files":        nv.Files().Len(),
		"Imports":      nv.Imports().Len(),
		"Structs":      nv.Structs().Len(),
		"Interfaces":   nv.Interfaces().Len(),
		"Methods":      nv.Methods().Len(),
		"Fields":       nv.Fields().Len(),
		"Functions":    nv.Functions().Len(),
		"Variables":    nv.Variables().Len(),
		"Constants":    nv.Constants().Len(),
		"Enums":        nv.Enums().Len(),
		"EnumVariants": nv.EnumVariants().Len(),
		"Aliases":      nv.Aliases().Len(),
	}
}

// nodeCountsEqual reports whether two count snapshots agree on
// every bucket.
func nodeCountsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if b[k] != av {
			return false
		}
	}
	return true
}

// snapshotMetaBags marshals every indexed node's
// [meta.Bag] to its canonical JSON wire form, keyed by a
// stable identifier (`<kind>:<qname>`). The map is consumed
// by the idempotency check; the wire form is byte-deterministic
// (sorted entries, current value per authority slot) so two
// snapshots agree iff every bag's current state agrees.
func snapshotMetaBags(s *store.Store) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, n := range collectIndexedNodes(s) {
		key := identityKey(n)
		raw, err := json.Marshal(n.Meta())
		if err != nil {
			return nil, fmt.Errorf("marshal meta for %s: %w", key, err)
		}
		out[key] = raw
	}
	return out, nil
}

// collectIndexedNodes returns every node indexed in s.Nodes()
// in a stable order. Walked bucket-by-bucket so iteration is
// deterministic across runs.
func collectIndexedNodes(s *store.Store) []node.Node {
	nv := s.Nodes()
	total := len(nv.Packages().Items()) +
		len(nv.Files().Items()) +
		len(nv.Imports().Items()) +
		len(nv.Structs().Items()) +
		len(nv.Interfaces().Items()) +
		len(nv.Methods().Items()) +
		len(nv.Fields().Items()) +
		len(nv.Functions().Items()) +
		len(nv.Variables().Items()) +
		len(nv.Constants().Items()) +
		len(nv.Enums().Items()) +
		len(nv.EnumVariants().Items()) +
		len(nv.Aliases().Items())
	out := make([]node.Node, 0, total)
	for _, n := range nv.Packages().Items() {
		out = append(out, n)
	}
	for _, n := range nv.Files().Items() {
		out = append(out, n)
	}
	for _, n := range nv.Imports().Items() {
		out = append(out, n)
	}
	for _, n := range nv.Structs().Items() {
		out = append(out, n)
	}
	for _, n := range nv.Interfaces().Items() {
		out = append(out, n)
	}
	for _, n := range nv.Methods().Items() {
		out = append(out, n)
	}
	for _, n := range nv.Fields().Items() {
		out = append(out, n)
	}
	for _, n := range nv.Functions().Items() {
		out = append(out, n)
	}
	for _, n := range nv.Variables().Items() {
		out = append(out, n)
	}
	for _, n := range nv.Constants().Items() {
		out = append(out, n)
	}
	for _, n := range nv.Enums().Items() {
		out = append(out, n)
	}
	for _, n := range nv.EnumVariants().Items() {
		out = append(out, n)
	}
	for _, n := range nv.Aliases().Items() {
		out = append(out, n)
	}
	return out
}

// identityKey returns a stable identifier for n composed of
// its kind discriminator and qualified name. Used as a map key
// for cross-snapshot comparison.
func identityKey(n node.Node) string {
	return fmt.Sprintf("%s:%s", n.Kind(), qNameOf(n))
}

// qNameOf returns n.QName for the node kinds that declare one,
// falling back to "<unnamed>" for kinds without an exported
// QName method (the package node itself is the typical case).
// The fallback keys distinct unnamed nodes by their kind alone
// — fine for snapshot comparison because the bucket iteration
// is order-stable so unnamed entries collide deterministically.
func qNameOf(n node.Node) string {
	if q, ok := any(n).(interface{ QName() string }); ok {
		return q.QName()
	}
	if q, ok := any(n).(interface{ Name() string }); ok {
		return q.Name()
	}
	return unnamedSentinel
}

// reportFirstDifference fails t with the first diverging
// (key, lhs-bytes, rhs-bytes) triple between two meta-bag
// snapshots, or with a missing-key complaint when the snapshots
// have different shapes. Reporting the first divergence makes
// the failure easier to read than dumping the whole map.
func reportFirstDifference(tb testing.TB, first, second map[string][]byte) {
	tb.Helper()
	if len(first) != len(second) {
		tb.Errorf(
			"meta-bag snapshot count differs across passes: first=%d second=%d "+
				"(annotators must not add or remove nodes)",
			len(first), len(second),
		)
		return
	}
	keys := make([]string, 0, len(first))
	for k := range first {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		lhs, rhs := first[k], second[k]
		if !bytes.Equal(lhs, rhs) {
			tb.Errorf(
				"meta-bag snapshot differs at %q across passes; annotator is not idempotent\n"+
					"  first pass:  %s\n  second pass: %s",
				k, lhs, rhs,
			)
			return
		}
	}
}
