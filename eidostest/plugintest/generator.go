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
}

// RunGeneratorSuite runs the conformance checks every
// [plugin.Generator] must satisfy: it must not panic on an
// empty store, it must not mutate source-side node counts
// during the generator phase (which only writes to the emit
// graph), and its emit production must be deterministic —
// driving Generate against two freshly-built stores produced
// from the same fixture must yield identical emit projections.
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
	assertGeneratorFixtureNamesUnique(t, fixtures)
	for _, fx := range fixtures {
		t.Run("fixture="+fx.Name+"/Generate does not panic", func(t *testing.T) {
			s := buildGeneratorStore(t, fx)
			assertGenerateDoesNotPanic(t, g, s)
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

	if err := runGenerateRecovering(g, s); err != nil {
		tb.Fatalf("Generate panicked: %v", err)
	}

	probes := []struct {
		name  string
		byTag map[string]string
	}{
		{"no tag routed", map[string]string{}},
		{"only foreign tags routed", map[string]string{"nonesuch": "example.com/x"}},
		{"primary routed without a derivable path", map[string]string{"": ""}},
	}

	for _, pending := range s.Emit().PendingOriginSlots() {
		setter, ok := pending.Item.(emit.OutputPackageSetter)
		if !ok {
			continue
		}
		for _, probe := range probes {
			if err := callSetOutputPackagesRecovering(setter, probe.byTag); err != nil {
				tb.Errorf("generator %q: SetOutputPackages panicked on a %s contribution "+
					"when %s: %v; the map carries only tags that routed, so an "+
					"implementor must not assume its own are present",
					g.Name(), pending.Item.Kind(), probe.name, err)
			}
		}
	}
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
	if err := runGenerateRecovering(g, s); err != nil {
		tb.Fatalf("Generate panicked: %v", err)
	}

	// The active language is not observable from the generator alone,
	// so the declared set is the union across the probe languages. An
	// empty union means the plugin declares nothing routable and the
	// check has nothing to say.
	declared := map[string]struct{}{"": {}}
	var declaresAny bool
	for _, lang := range probeLanguages {
		for _, o := range provider.Outputs(lang) {
			declared[o.Tag] = struct{}{}
			declaresAny = true
		}
	}
	if !declaresAny {
		return
	}

	for _, pending := range s.Emit().PendingOriginSlots() {
		tagged, ok := pending.Item.(interface{ OutputTag() string })
		if !ok {
			continue
		}
		tag := tagged.OutputTag()
		if _, ok := declared[tag]; !ok {
			tb.Errorf("generator %q stamped OutputTag %q on a %s contribution but declares no "+
				"Output with that tag; the decl cannot route to the file the tag names",
				g.Name(), tag, pending.Item.Kind())
		}
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
	s := fx.BuildStore(t)
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
	if err := runGenerateRecovering(g, s); err != nil {
		tb.Errorf("Generate panicked on empty store: %v", err)
	}
}

// assertGenerateDoesNotPanic drives the generator against the
// fixture's store and fails if it panics or returns an error.
// Generators surface contract violations through ctx.Diag;
// returned errors are reserved for catastrophic failures.
func assertGenerateDoesNotPanic(tb testing.TB, g plugin.Generator, s *store.Store) {
	tb.Helper()
	if err := runGenerateRecovering(g, s); err != nil {
		tb.Errorf("Generate panicked on fixture store: %v", err)
	}
}

// assertGenerateLeavesSourceNodesUnchanged pins the
// frozen-source contract: generators read source nodes and
// produce emit entities; they must not mutate the source side
// of the store. The check counts source-side nodes before and
// after a single Generate invocation and fails on mismatch.
func assertGenerateLeavesSourceNodesUnchanged(tb testing.TB, g plugin.Generator, s *store.Store) {
	tb.Helper()
	before := snapshotNodeCounts(s)
	if err := runGenerateRecovering(g, s); err != nil {
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
	if err := runGenerateRecovering(g, s1); err != nil {
		tb.Fatalf("Generate panicked on first determinism pass: %v", err)
	}
	if err := runGenerateRecovering(g, s2); err != nil {
		tb.Fatalf("Generate panicked on second determinism pass: %v", err)
	}
	first := emitProjection(s1)
	second := emitProjection(s2)
	if !slices.Equal(first, second) {
		tb.Errorf(
			"emit projection differs across two equivalent inputs; generator is not deterministic\n"+
				"  first run:  %s\n  second run: %s",
			strings.Join(first, ", "), strings.Join(second, ", "),
		)
	}
}

// runGenerateRecovering invokes Generate with a discard
// diagnostic sink and recovers any panic into a returned error.
// The plain Generate error is wrapped on the same path so
// callers can distinguish "panicked" from "returned an error"
// by inspecting the wrapping verb.
func runGenerateRecovering(g plugin.Generator, s *store.Store) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered panic: %v", r)
		}
	}()
	ctx := &plugin.GeneratorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   diag.New(),
	}
	if rerr := g.Generate(ctx); rerr != nil {
		return fmt.Errorf("generate returned error: %w", rerr)
	}
	return nil
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
