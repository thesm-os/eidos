// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/contract"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// GeneratorFixture describes a single input scenario the
// [RunGeneratorSuite] drives a [plugin.Generator] against.
//
// BuildStore is invoked once per subtest that exercises this
// fixture. The function must return a freshly-populated store
// each call — the determinism check builds two stores from
// the same fixture, runs the generator against each, and
// compares the resulting emit graphs; a shared store would
// invalidate the comparison.
type GeneratorFixture struct {
	// Name labels the fixture in subtest paths and failure
	// messages. Required and unique within a single
	// [RunGeneratorSuite] call.
	Name string

	// BuildStore returns a freshly-populated store. The function
	// is invoked once per subtest; tests fail fast through `t` on
	// builder errors rather than returning them.
	BuildStore func(t *testing.T) *store.Store

	// AllowsPositionlessDiagnostics waives the requirement that
	// every diagnostic the generator emits on this fixture carries
	// a source position.
	//
	// The zero value is the strict one, because a positionless
	// diagnostic renders as a dash where file and line belong and
	// the reader cannot act on it. Set this only when the
	// generator's complaints on this input genuinely name no source
	// construct — a run- or configuration-level failure with
	// nothing to point at. It does not waive the no-Error-severity
	// contract; that one is not negotiable per fixture.
	AllowsPositionlessDiagnostics bool
}

// RunGeneratorSuite runs the conformance checks every
// [plugin.Generator] must satisfy: it must not panic on an
// empty store, it must not mutate source-side node counts
// during the generator phase (which only writes to the emit
// graph), its emit production must be deterministic —
// driving Generate against two freshly-built stores produced
// from the same fixture must yield identical emit projections —
// and it must surface no Error-severity diagnostics on an input
// the fixture declares it handles, with every diagnostic it does
// emit carrying a source position.
//
// Fixtures supply realistic input scenarios. The suite drives
// the generator against each in a dedicated subtest so failure
// attribution stays scoped. Pass an empty fixture slice to run
// only the empty-store contract.
//
// Build- or run-time failures (BuildStore returning a nil
// store, the generator panicking on a fixture it claims to
// handle) surface through `t.Errorf` / `t.Fatalf` so the
// fixture name appears in the failure path.
func RunGeneratorSuite(t *testing.T, g plugin.Generator, fixtures []GeneratorFixture) {
	t.Helper()
	t.Run("Generate on empty store does not panic", func(t *testing.T) {
		assertGenerateEmptyStoreDoesNotPanic(t, g)
	})
	t.Run("Generate on empty store produces no Error-severity diagnostics", func(t *testing.T) {
		assertGenerateEmptyStoreCarriesNoErrors(t, g)
	})
	assertGeneratorFixtureNamesUnique(t, fixtures)
	for _, fx := range fixtures {
		t.Run("fixture="+fx.Name+"/Generate does not panic", func(t *testing.T) {
			s := buildGeneratorStore(t, fx)
			assertGenerateDoesNotPanic(t, g, s)
		})
		t.Run("fixture="+fx.Name+"/Generate produces no Error-severity diagnostics", func(t *testing.T) {
			s := buildGeneratorStore(t, fx)
			assertGenerateCarriesNoErrors(t, g, fx, s)
		})
		t.Run("fixture="+fx.Name+"/Generate diagnostics carry a source position", func(t *testing.T) {
			s := buildGeneratorStore(t, fx)
			assertGenerateDiagnosticsArePositioned(t, g, fx, s)
		})
		t.Run("fixture="+fx.Name+"/source-side node counts unchanged by Generate", func(t *testing.T) {
			s := buildGeneratorStore(t, fx)
			assertGenerateLeavesSourceNodesUnchanged(t, g, s)
		})
		t.Run("fixture="+fx.Name+"/Generate is deterministic across two runs", func(t *testing.T) {
			s1 := buildGeneratorStore(t, fx)
			s2 := buildGeneratorStore(t, fx)
			assertGenerateIsDeterministic(t, g, s1, s2)
		})
		t.Run("fixture="+fx.Name+"/NodesOnly declaration is truthful", func(t *testing.T) {
			assertNodesOnlyIsTruthful(t, g, buildGeneratorStore(t, fx), buildGeneratorStore(t, fx))
		})
		t.Run("fixture="+fx.Name+"/emitted values carry the generator's own SetBy", func(t *testing.T) {
			s := buildGeneratorStore(t, fx)
			assertEmitValuesAreAttributed(t, g, s)
		})
		t.Run("fixture="+fx.Name+"/emitted output tags are declared", func(t *testing.T) {
			s := buildGeneratorStore(t, fx)
			assertEmittedTagsAreDeclared(t, g, s)
		})
		t.Run("fixture="+fx.Name+"/output-package dispatch tolerates partial routing", func(t *testing.T) {
			s := buildGeneratorStore(t, fx)
			assertOutputPackagesTolerateMissingTags(t, g, s)
		})
	}
}

// assertOutputPackagesTolerateMissingTags exercises the awkward half
// of the [emit.OutputPackageSetter] contract: the map a value
// receives carries only the tags that actually routed, so a plugin
// declaring three outputs may be handed one, or one it did not
// expect, or none of them.
//
// That is not a hypothetical. A run where one output fails to route
// — a missing suffix for the active language, a directive naming an
// unknown tag, a one-file-one-package conflict — reaches dispatch
// with a partial map. An implementor that indexes the map and uses
// the result without checking produces a reference to the empty
// package, which renders as a bare name and binds to whatever else
// is in scope. That failure is silent in the emit graph and only
// surfaces as a miscompile in the generated output, which is why it
// belongs in the conformance suite rather than in each plugin's own
// tests.
//
// The check probes each implementing value with an empty map, a map
// of tags the plugin never declared, and a map holding only the
// primary tag with no derivable path — the three shapes a partly
// failed run produces. Only panics fail the check; what an
// implementor does with a missing tag is its own decision, but doing
// it without crashing is not.
func assertOutputPackagesTolerateMissingTags(tb testing.TB, g plugin.Generator, s *store.Store) {
	tb.Helper()

	if err := runGenerateRecovering(g, s, diag.Discard()); err != nil {
		tb.Fatalf("Generate %s: %v", probeVerb(err), err)
	}

	probes := []struct {
		name  string
		byTag map[string]string
	}{
		{"no tag routed", map[string]string{}},
		{"only foreign tags routed", map[string]string{"nonesuch": "example.com/x"}},
		{"primary routed without a derivable path", map[string]string{"": ""}},
	}

	// Every implementor in the graph, not only the queued slot
	// contributions. Layout's own dispatch walks the emit graph from
	// both roots, so a decl-level implementor is handed the same
	// partial map — and until this walked the graph too, none of them
	// was ever probed.
	for _, n := range walkEmitNodes(s) {
		setter, ok := n.(emit.OutputPackageSetter)
		if !ok {
			continue
		}
		for _, probe := range probes {
			if err := callSetOutputPackagesRecovering(setter, probe.byTag); err != nil {
				tb.Errorf("generator %q: SetOutputPackages panicked on a %s value "+
					"when %s: %v; the map carries only tags that routed, so an "+
					"implementor must not assume its own are present",
					g.Name(), n.Kind(), probe.name, err)
			}
		}
	}
}

// walkEmitNodes returns every node reachable in s's emit graph plus
// every queued origin-slot contribution, in traversal order.
//
// The roots mirror Layout's own dispatch — packages and files both —
// so a check written against this list sees what routing sees. The
// pending queue is appended because those contributions are not in the
// graph until Layout materialises them, and a contract that only holds
// after materialisation is one the suite could never have checked.
func walkEmitNodes(s *store.Store) []emit.Node {
	ev := s.Emit()
	var out []emit.Node
	var collect emit.VisitorFunc
	collect = func(n emit.Node) emit.Visitor {
		out = append(out, n)
		return collect
	}
	for _, p := range ev.Packages().Items() {
		emit.Walk(p, collect)
	}
	for _, f := range ev.Files().Items() {
		emit.Walk(f, collect)
	}
	for _, pending := range ev.PendingOriginSlots() {
		out = append(out, pending.Item)
	}
	return out
}

// callSetOutputPackagesRecovering invokes SetOutputPackages and
// converts a panic into an error, so one misbehaving implementor
// reports as a failure rather than taking down the whole suite run.
func callSetOutputPackagesRecovering(setter emit.OutputPackageSetter, byTag map[string]string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	setter.SetOutputPackages(byTag)
	return nil
}

// assertEmittedTagsAreDeclared pins that every OutputTag a generator
// stamps on an emit value corresponds to an [plugin.Output] the same
// generator declares.
//
// The two halves of multi-output generation are wired by a bare
// string and nothing checked they agree. A value carrying a tag the
// plugin never declared does not fail loudly: the Layout phase finds
// no matching Output, and the decl routes somewhere other than the
// file the tag names. That is the same shape as a detector that is
// registered but never reachable — a declaration that looks
// load-bearing and is inert.
//
// Per-plugin tests cannot catch it, because they assert on the emit
// graph without running routing. This check needs only the
// generator, so it belongs with the other contract assertions.
func assertEmittedTagsAreDeclared(tb testing.TB, g plugin.Generator, s *store.Store) {
	tb.Helper()

	provider, isProvider := any(g).(plugin.FilenameProvider)
	if !isProvider {
		// A generator owning no routable decl declares no outputs;
		// pure slot weavers are the documented case.
		return
	}
	if err := runGenerateRecovering(g, s, diag.Discard()); err != nil {
		tb.Fatalf("Generate %s: %v", probeVerb(err), err)
	}

	// The active language is not observable from the generator alone,
	// so the declared set is the union across the probe languages. An
	// empty union means the plugin declares nothing routable and the
	// check has nothing to say.
	// Seeded from the declared Outputs alone. The empty tag used to be
	// whitelisted unconditionally, which made the plugin's *primary*
	// output — the one most decls carry — unfalsifiable: a generator
	// declaring only tagged outputs still routes its untagged decls
	// nowhere, and Layout reports ErrNoDefaultOutput for exactly that.
	declared := map[string]struct{}{}
	for _, lang := range probeLanguages {
		for _, o := range provider.Outputs(lang) {
			declared[o.Tag] = struct{}{}
		}
	}
	if len(declared) == 0 {
		return
	}

	for _, n := range walkEmitNodes(s) {
		tagged, ok := n.(interface{ OutputTag() string })
		if !ok {
			continue
		}
		tag := tagged.OutputTag()
		if _, ok := declared[tag]; ok {
			continue
		}
		tb.Errorf("generator %q stamped OutputTag %q on a %s value but declares no "+
			"Output with that tag; Layout finds no matching Output and drops the decl, so "+
			"it never reaches the file the tag names",
			g.Name(), tag, n.Kind())
	}
}

// assertGeneratorFixtureNamesUnique fails when two fixtures
// share a Name. Duplicate names would produce identical
// subtest paths, masking which fixture triggered a failure.
func assertGeneratorFixtureNamesUnique(tb testing.TB, fixtures []GeneratorFixture) {
	tb.Helper()
	seen := make(map[string]struct{}, len(fixtures))
	for _, fx := range fixtures {
		if fx.Name == "" {
			tb.Fatalf("RunGeneratorSuite: fixture has empty Name; every GeneratorFixture must declare one")
		}
		if _, dup := seen[fx.Name]; dup {
			tb.Fatalf("RunGeneratorSuite: duplicate fixture Name %q", fx.Name)
		}
		seen[fx.Name] = struct{}{}
	}
}

// buildGeneratorStore invokes fx.BuildStore and surfaces nil /
// builder failures as test fatals. Returns the per-subtest
// copy the generator runs against.
func buildGeneratorStore(t *testing.T, fx GeneratorFixture) *store.Store {
	t.Helper()
	if fx.BuildStore == nil {
		t.Fatalf("RunGeneratorSuite: fixture %q has nil BuildStore", fx.Name)
	}
	s, err := buildFixtureStoreRecovering(func() *store.Store { return fx.BuildStore(t) })
	if err != nil {
		t.Fatalf("RunGeneratorSuite: fixture %q: %v", fx.Name, err)
	}
	if s == nil {
		t.Fatalf("RunGeneratorSuite: fixture %q BuildStore returned nil store", fx.Name)
	}
	return s
}

// assertGenerateEmptyStoreDoesNotPanic drives the generator
// against a fresh empty store. The pipeline runs generators
// unconditionally; a generator that panics with no source
// decls is a runtime crash waiting to happen on a project
// whose patterns expand to no matches.
func assertGenerateEmptyStoreDoesNotPanic(tb testing.TB, g plugin.Generator) {
	tb.Helper()
	s := store.New()
	if err := runGenerateRecovering(g, s, diag.Discard()); err != nil {
		tb.Errorf("Generate %s on an empty store: %v", probeVerb(err), err)
	}
}

// assertGenerateEmptyStoreCarriesNoErrors drives the generator
// against a fresh empty store and fails when it complains about it.
//
// A generator that reports an Error on an empty store reports one on
// every project whose patterns expand to no matches — a first-run
// experience of a non-zero exit and a diagnostic about input the user
// never wrote. The positioned-diagnostic check has no counterpart
// here: the probe carries no fixture, so there is nothing to hang the
// waiver on, and an empty store has no source construct to name
// anyway.
func assertGenerateEmptyStoreCarriesNoErrors(tb testing.TB, g plugin.Generator) {
	tb.Helper()
	d := diag.Capture()
	if err := runGenerateRecovering(g, store.New(), d); err != nil {
		tb.Fatalf("Generate did not complete on an empty store: %v", err)
	}
	reportErrorDiagnostics(tb, roleGenerator, emptyStoreSubject, d)
}

// assertGenerateDoesNotPanic drives the generator against the
// fixture's store and fails if it panics or returns an error.
// Generators surface contract violations through ctx.Diag;
// returned errors are reserved for catastrophic failures.
func assertGenerateDoesNotPanic(tb testing.TB, g plugin.Generator, s *store.Store) {
	tb.Helper()
	if err := runGenerateRecovering(g, s, diag.Discard()); err != nil {
		tb.Errorf("Generate %s on the fixture store: %v", probeVerb(err), err)
	}
}

// assertGenerateCarriesNoErrors drives the generator against the
// fixture's store and fails when the diagnostic sink records any
// Error-severity entry. Mirrors [assertRenderCarriesNoErrors], whose
// rationale transfers verbatim: the fixtures a plugin author supplies
// represent inputs the plugin handles cleanly, and an Error
// diagnostic on one of them is a contract failure the author intends
// to surface — to the user, on every run, as a non-zero exit.
func assertGenerateCarriesNoErrors(tb testing.TB, g plugin.Generator, fx GeneratorFixture, s *store.Store) {
	tb.Helper()
	d := diag.Capture()
	if err := runGenerateRecovering(g, s, d); err != nil {
		tb.Fatalf("Generate did not complete on fixture %q: %v", fx.Name, err)
	}
	reportErrorDiagnostics(tb, roleGenerator, fixtureSubject(fx.Name), d)
}

// assertGenerateDiagnosticsArePositioned drives the generator against
// the fixture's store and fails when any diagnostic it emitted
// carries a zero position, unless the fixture waived the check
// through [GeneratorFixture.AllowsPositionlessDiagnostics].
func assertGenerateDiagnosticsArePositioned(
	tb testing.TB,
	g plugin.Generator,
	fx GeneratorFixture,
	s *store.Store,
) {
	tb.Helper()
	d := diag.Capture()
	if err := runGenerateRecovering(g, s, d); err != nil {
		tb.Fatalf("Generate did not complete on fixture %q: %v", fx.Name, err)
	}
	reportPositionlessDiagnostics(tb, roleGenerator, fixtureSubject(fx.Name), d, fx.AllowsPositionlessDiagnostics)
}

// assertGenerateLeavesSourceNodesUnchanged pins the
// frozen-source contract: generators read source nodes and
// produce emit entities; they must not mutate the source side
// of the store. The check counts source-side nodes before and
// after a single Generate invocation and fails on mismatch.
func assertGenerateLeavesSourceNodesUnchanged(tb testing.TB, g plugin.Generator, s *store.Store) {
	tb.Helper()
	before := snapshotNodeCounts(s)
	if err := runGenerateRecovering(g, s, diag.Discard()); err != nil {
		tb.Fatalf("Generate panicked during source-node check: %v", err)
	}
	after := snapshotNodeCounts(s)
	if !nodeCountsEqual(before, after) {
		tb.Errorf(
			"Generate changed source-side node counts: before=%v after=%v "+
				"(generators must not mutate Store.Nodes())",
			before, after,
		)
	}
}

// assertGenerateIsDeterministic runs the generator against two
// freshly-built stores produced from the same fixture and
// compares the resulting emit projections. The projection is
// a sorted list of stable identity tuples — kind, qualified
// name, and target — covering every emit entity the suite
// recognises. Equal projections imply the generator produces
// the same set of emit entities at the same target paths for
// equivalent inputs.
//
// Per-entity content (field shape, method signatures, slot
// contributions) is intentionally outside the suite's scope:
// downstream tests assert against rendered output through
// [pipelinetest] / [backendtest], where deviations surface as
// golden-diff failures with full context. The projection here
// catches the structural-determinism property the pipeline's
// scheduling and caching layers rely on.
func assertGenerateIsDeterministic(tb testing.TB, g plugin.Generator, s1, s2 *store.Store) {
	tb.Helper()
	if err := runGenerateRecovering(g, s1, diag.Discard()); err != nil {
		tb.Fatalf("Generate %s on the first determinism pass: %v", probeVerb(err), err)
	}
	if err := runGenerateRecovering(g, s2, diag.Discard()); err != nil {
		tb.Fatalf("Generate %s on the second determinism pass: %v", probeVerb(err), err)
	}
	if first, second := emitDeepProjection(s1), emitDeepProjection(s2); !slices.Equal(first, second) {
		tb.Errorf(
			"emit projection differs across two equivalent inputs; generator is not deterministic\n"+
				"  first run:  %s\n  second run: %s\n"+
				"  identity-only diff (order and slots erased):\n    first:  %s\n    second: %s",
			strings.Join(first, ", "), strings.Join(second, ", "),
			strings.Join(emitProjection(s1), ", "), strings.Join(emitProjection(s2), ", "),
		)
	}
}

// emitDeepProjection returns an ordered projection of everything a
// generator emitted: the walked emit graph in traversal order,
// followed by the queued origin-slot contributions in registration
// order.
//
// Three properties distinguish it from [emitProjection], and each
// closes a class that projection could not see.
//
// It is ordered. [emitProjection] sorts, which is right for a
// readable diff and wrong for an oracle: a generator that emits the
// same set of declarations in a different order every run renders a
// different file every run, and the sorted form calls the two equal.
//
// It walks rather than enumerating buckets. [emit.Walk] descends into
// slots, so a contribution appended to a host's slot is visible; the
// bucket enumeration never saw one. The roots mirror the production
// traversal in Layout — packages and files both — so the suite reads
// the graph the way the thing it stands in for does.
//
// It includes the pending origin-slot queue. Those contributions are
// not in the graph until Layout materialises them, so nothing that
// walks the graph alone can observe a generator whose entire output
// goes through [store.EmitView.AppendOriginSlot] — the shape four
// in-tree plugins ship, and the one the old oracle scored green
// whatever it did.
func emitDeepProjection(s *store.Store) []string {
	ev := s.Emit()
	var out []string

	var record emit.VisitorFunc
	record = func(n emit.Node) emit.Visitor {
		out = append(out, fmt.Sprintf("%d:%s", len(out), emitNodeIdentity(n)))
		return record
	}
	for _, p := range ev.Packages().Items() {
		emit.Walk(p, record)
	}
	for _, f := range ev.Files().Items() {
		emit.Walk(f, record)
	}

	// Registration order is load-bearing: Layout drains this queue in
	// plugin-topo order across plugins and FIFO within each, so two
	// runs that queue the same contributions in a different order
	// render them in a different order.
	for i, pending := range ev.PendingOriginSlots() {
		out = append(out, fmt.Sprintf(
			"pending:%d:slot=%s:origin=%s:setBy=%s:%s",
			i, pending.SlotName, nodeOwnerName(pending.Origin),
			pending.Prov.SetBy, emitNodeIdentity(pending.Item),
		))
	}
	return out
}

// emitNodeIdentity renders one emit node's identity for the ordered
// projection: its kind, its name where it has one, and the routing
// fields a divergence would show up in.
//
// SetBy and OutputTag are included because both decide where the value
// lands, so a run that changes either changes the output file even
// when every name matches.
func emitNodeIdentity(n emit.Node) string {
	if n == nil {
		return "<nil>"
	}
	name := unnamedSentinel
	if named, ok := any(n).(interface{ QName() string }); ok {
		name = named.QName()
	} else if named, ok := any(n).(interface{ GetName() string }); ok {
		name = named.GetName()
	}
	return fmt.Sprintf("%s:%s:setBy=%s:tag=%s", n.Kind(), name, n.SetBy(), n.OutputTag())
}

// assertEmitValuesAreAttributed pins that every value a generator
// emits carries that generator's own [plugin.Plugin.Name] in SetBy.
//
// SetBy is the only cross-plugin identity key in the emit graph, and
// three separate consumers look values up by it: Layout resolves the
// declared Outputs of the plugin named there, slot rendering orders
// contributions by it against the resolved plugin order, and the
// rendered header attributes the file to it. A value carrying anything
// else matches no entry in any of them — its declarations are dropped
// at routing, and the diagnostic names a plugin string the author has
// never seen.
//
// Nothing writes the field automatically. A generator that hand-builds
// its emit values, or passes a literal to the emit builder rather than
// its own Name, produces exactly this.
func assertEmitValuesAreAttributed(tb testing.TB, g plugin.Generator, s *store.Store) {
	tb.Helper()
	if err := runGenerateRecovering(g, s, diag.Discard()); err != nil {
		tb.Fatalf("Generate did not complete during the attribution check: %v", err)
	}

	want := g.Name()
	report := func(where, got, identity string) {
		tb.Errorf(
			"%s carries SetBy %q, but the generator's Name is %q; Layout looks a plugin's declared "+
				"Outputs up under SetBy, slot rendering orders by it and the file header attributes "+
				"by it, so a foreign value drops the declaration at routing\n  value: %s",
			where, got, want, identity,
		)
	}

	ev := s.Emit()
	var visit emit.VisitorFunc
	visit = func(n emit.Node) emit.Visitor {
		// The empty string is "unattributed", which the framework
		// treats as legitimate for values no plugin claims; only a
		// foreign non-empty name is a contract failure.
		if got := n.SetBy(); got != "" && got != want {
			report("an emitted value", got, emitNodeIdentity(n))
		}
		return visit
	}
	for _, p := range ev.Packages().Items() {
		emit.Walk(p, visit)
	}
	for _, f := range ev.Files().Items() {
		emit.Walk(f, visit)
	}
	for _, pending := range ev.PendingOriginSlots() {
		if got := pending.Prov.SetBy; got != "" && got != want {
			report("a queued slot contribution's Provenance", got, emitNodeIdentity(pending.Item))
		}
	}
}

// runGenerateRecovering invokes Generate against the caller's
// diagnostic sink and recovers any panic into a returned error.
// The plain Generate error is wrapped on the same path so
// callers can distinguish "panicked" from "returned an error"
// by inspecting the wrapping verb.
//
// The sink is a parameter rather than a local because a check that
// cannot reach it cannot assert on it — which is how three role
// suites certified plugins that reported an Error on every input.
// Pass [diag.Capture] to inspect what was emitted, [diag.Discard]
// when the check is about panics or store state and the diagnostics
// belong to a sibling check.
func runGenerateRecovering(g plugin.Generator, s *store.Store, d *diag.Sink) error {
	return runGenerateWithReader(g, s, store.NewReader(s), d)
}

// emitProjection returns a sorted slice of stable identity
// strings — one per emit entity in s.Emit() — covering every
// kind the suite recognises. The format is
// "<kind>:<qname>:<target-joined-path>" with a per-kind
// identifier in the qname slot; empty fields render as
// "<empty>" so missing data is visible in failure output.
//
// The projection is stable across runs: bucket iteration is
// insertion-order-deterministic and the sort produces a
// canonical form independent of insertion order.
func emitProjection(s *store.Store) []string {
	ev := s.Emit()
	total := ev.Packages().Len() + ev.Files().Len() + ev.Imports().Len() +
		ev.Structs().Len() + ev.Interfaces().Len() + ev.Methods().Len() +
		ev.Fields().Len() + ev.Functions().Len() + ev.Variables().Len() +
		ev.Constants().Len() + ev.Enums().Len() + ev.EnumVariants().Len() +
		ev.Aliases().Len()
	out := make([]string, 0, total)
	for _, n := range ev.Packages().Items() {
		out = append(out, fmt.Sprintf("package:%s:%s", n.Name, n.Path))
	}
	for _, n := range ev.Files().Items() {
		out = append(out, fmt.Sprintf("file:%s:%s", n.Name, formatTarget(n.Target())))
	}
	for _, n := range ev.Imports().Items() {
		out = append(out, fmt.Sprintf("import:%s:alias=%s", n.Path, n.Alias))
	}
	for _, n := range ev.Structs().Items() {
		out = append(out, fmt.Sprintf("struct:%s:%s", n.QName(), formatTarget(n.Target)))
	}
	for _, n := range ev.Interfaces().Items() {
		out = append(out, fmt.Sprintf("interface:%s:%s", n.QName(), formatTarget(n.Target)))
	}
	for _, n := range ev.Methods().Items() {
		out = append(out, fmt.Sprintf("method:%s.%s", emitOwnerName(n.Owner), n.Name))
	}
	for _, n := range ev.Fields().Items() {
		out = append(out, fmt.Sprintf("field:%s.%s", emitOwnerName(n.Owner), n.Name))
	}
	for _, n := range ev.Functions().Items() {
		out = append(out, fmt.Sprintf("function:%s:%s", n.QName(), formatTarget(n.Target)))
	}
	for _, n := range ev.Variables().Items() {
		out = append(out, "variable:"+n.QName())
	}
	for _, n := range ev.Constants().Items() {
		out = append(out, "constant:"+n.QName())
	}
	for _, n := range ev.Enums().Items() {
		out = append(out, fmt.Sprintf("enum:%s:%s", n.QName(), formatTarget(n.Target)))
	}
	for _, n := range ev.EnumVariants().Items() {
		out = append(out, fmt.Sprintf("enum-variant:%s.%s", emitOwnerName(n.Owner), n.Name))
	}
	for _, n := range ev.Aliases().Items() {
		out = append(out, fmt.Sprintf("alias:%s:%s", n.QName(), formatTarget(n.File)))
	}
	slices.Sort(out)
	return out
}

// emitOwnerName returns the qualified name of an emit owner
// node (the [emit.Method.Owner] / [emit.Field.Owner] /
// [emit.EnumVariant.Owner] back-pointer). The owner is always
// a kind that implements QName when set; nil owners — possible
// when an emit value was constructed without going through the
// builder — surface as [unownedSentinel] so failure output
// remains readable.
func emitOwnerName(owner contract.Node) string {
	if owner == nil {
		return unownedSentinel
	}
	if q, ok := any(owner).(interface{ QName() string }); ok {
		return q.QName()
	}
	return unnamedSentinel
}

// formatTarget formats a [emit.Target] as "<dir>/<filename>;package=<pkg>"
// so the failure output renders all three fields without ambiguity.
// [emptyTargetSentinel] marks fields the generator left at their
// zero value.
func formatTarget(t emit.Target) string {
	dir := t.Dir
	if dir == "" {
		dir = emptyTargetSentinel
	}
	filename := t.Filename
	if filename == "" {
		filename = emptyTargetSentinel
	}
	pkg := t.Package
	if pkg == "" {
		pkg = emptyTargetSentinel
	}
	return fmt.Sprintf("%s/%s;package=%s", dir, filename, pkg)
}

// emitReadPrefix is the tag prefix [store.Reader] stamps on every
// emit-side query it records. Node-side reads use "node:".
const emitReadPrefix = "emit:"

// canaryEmitPackage is the import path of the package
// [assertNodesOnlyIsTruthful] pre-seeds into the emit graph. Named so
// a failure message can distinguish the suite's own decl from the
// generator's output.
const canaryEmitPackage = "example.com/plugintest/canary"

// assertNodesOnlyIsTruthful fails a generator that declares
// [plugin.NodesOnly] and then reads the emit graph anyway.
//
// The declaration is not documentation. The pipeline reads it at
// Build and dispatches an entire generator bucket concurrently when
// every generator in that bucket returns true. A generator that lies
// races against whichever sibling is mutating the emit graph in
// another goroutine, and the result is a corrupted emit graph on some
// runs and not others — the worst shape of defect this framework can
// produce, because the output still compiles.
//
// Two independent detections, because each covers the other's blind
// spot:
//
//   - The ReadSet names the culprit. Every [store.Reader] accessor
//     tags its reads, emit-side ones with [emitReadPrefix], so a
//     violation reports exactly which accessor was called. It is
//     blind to a generator that reaches around the Reader straight
//     into ctx.Store.Emit(), which the store documents as legal.
//
//   - The differential catches that one. Generate runs against two
//     equivalent stores differing only in whether the emit graph
//     already holds a canary package. A NodesOnly generator's output
//     cannot depend on emit state, so its contribution must be
//     identical in both. This covers every read path at the cost of
//     not naming which one.
func assertNodesOnlyIsTruthful(tb testing.TB, g plugin.Generator, clean, seeded *store.Store) {
	tb.Helper()

	no, ok := any(g).(plugin.NodesOnly)
	if !ok || !no.NodesOnly() {
		return // declaring nothing, or declaring false, is always honest
	}

	reader := store.NewReader(clean)
	// Discard rather than Capture: this check reads the ReadSet and
	// the emit projection, and whatever the generator says about the
	// store is the diagnostic checks' business.
	if err := runGenerateWithReader(g, clean, reader, diag.Discard()); err != nil {
		return // panics and errors are the business of the other checks
	}
	for _, key := range reader.ReadSet().Keys() {
		if strings.HasPrefix(key, emitReadPrefix) {
			tb.Errorf("NodesOnly reports true but Generate read %q; a NodesOnly generator may "+
				"not read the emit graph, because the pipeline dispatches its whole bucket "+
				"concurrently on the strength of that declaration", key)
		}
	}

	seedCanaryEmit(tb, seeded)
	baseline := emitProjection(seeded)
	if err := runGenerateRecovering(g, seeded, diag.Discard()); err != nil {
		return
	}
	contributed := withoutEntries(emitProjection(seeded), baseline)

	if want := emitProjection(clean); !slices.Equal(contributed, want) {
		tb.Errorf("NodesOnly reports true but Generate produced different output when the emit "+
			"graph was pre-populated, so it depends on emit state it declared it would not "+
			"read.\n with a clean emit graph: %v\n with a pre-seeded one:   %v", want, contributed)
	}
}

// seedCanaryEmit places one package into the emit graph so a
// generator that reads emit state observes something to react to.
func seedCanaryEmit(tb testing.TB, s *store.Store) {
	tb.Helper()
	pkg := &emit.Package{Name: "canary", Path: canaryEmitPackage}
	pkg.Structs = []*emit.Struct{{Name: "Canary", Package: canaryEmitPackage}}
	if err := s.Emit().AddPackage(pkg); err != nil {
		tb.Fatalf("seeding the canary emit package failed: %v", err)
	}
}

// withoutEntries returns the entries of all that do not appear in
// remove. Both are the sorted output of [emitProjection], so the
// result is the generator's own contribution.
func withoutEntries(all, remove []string) []string {
	drop := make(map[string]struct{}, len(remove))
	for _, e := range remove {
		drop[e] = struct{}{}
	}
	out := make([]string, 0, len(all))
	for _, e := range all {
		if _, skip := drop[e]; !skip {
			out = append(out, e)
		}
	}
	return out
}

// runGenerateWithReader is [runGenerateRecovering] with a
// caller-supplied [store.Reader], so a check can inspect what the
// generator read. Both share this body; the reader is the only thing
// the two forms disagree about.
func runGenerateWithReader(g plugin.Generator, s *store.Store, r *store.Reader, d *diag.Sink) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("%w: %v", ErrProbePanicked, rec)
		}
	}()
	ctx := &plugin.GeneratorContext{Store: s, Reader: r, Diag: d}
	if gerr := g.Generate(ctx); gerr != nil {
		return fmt.Errorf("%w: %w", ErrProbeReturnedError, gerr)
	}
	return nil
}
