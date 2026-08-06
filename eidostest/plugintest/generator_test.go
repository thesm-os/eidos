// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest_test

import (
	"fmt"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// TestRunGeneratorSuite_PassesForWellFormedGenerator pins the
// happy path of [plugintest.RunGeneratorSuite]: a generator
// that emits one struct per source struct passes every contract
// — does not panic, leaves source-node counts intact, and
// produces identical emit projections across two equivalent
// runs.
func TestRunGeneratorSuite_PassesForWellFormedGenerator(t *testing.T) {
	t.Parallel()
	plugintest.RunGeneratorSuite(
		t,
		&mirroringGenerator{name: "mirror"},
		[]plugintest.GeneratorFixture{
			{
				Name: "package with one struct",
				BuildStore: func(t *testing.T) *store.Store {
					t.Helper()
					return storefixture.New().
						Struct("User", nil).
						Build()
				},
			},
			{
				Name: "package with three structs",
				BuildStore: func(t *testing.T) *store.Store {
					t.Helper()
					return storefixture.New().
						Struct("User", nil).
						Struct("Order", nil).
						Struct("Invoice", nil).
						Build()
				},
			},
		},
	)
}

// TestRunGeneratorSuite_RejectsPanickingGenerator covers the
// empty-store panic rejection.
func TestRunGeneratorSuite_RejectsPanickingGenerator(t *testing.T) {
	t.Parallel()
	a := &panickingGenerator{name: "panicky"}
	fake := newFakeT()
	plugintest.AssertGenerateEmptyStoreDoesNotPanic(fake, a)
	assertFakeMentions(t, fake, "Generate panicked on empty store")
}

// TestRunGeneratorSuite_RejectsNonDeterministicGenerator pins
// the determinism contract: a generator whose output depends on
// call-count (or any other call-site-varying input) fails the
// two-run comparison.
func TestRunGeneratorSuite_RejectsNonDeterministicGenerator(t *testing.T) {
	t.Parallel()
	s1 := storefixture.New().Struct("User", nil).Build()
	s2 := storefixture.New().Struct("User", nil).Build()
	g := &flappingGenerator{name: "flap"}
	fake := newFakeT()
	plugintest.AssertGenerateIsDeterministic(fake, g, s1, s2)
	assertFakeMentions(t, fake, "generator is not deterministic")
}

// TestRunGeneratorSuite_RejectsSourceMutator pins the
// frozen-source-nodes contract: a generator that mutates the
// source side of the store fails the node-count check.
func TestRunGeneratorSuite_RejectsSourceMutator(t *testing.T) {
	t.Parallel()
	s := storefixture.New().Struct("User", nil).Build()
	g := &sourceMutatingGenerator{name: "mutate"}
	fake := newFakeT()
	plugintest.AssertGenerateLeavesSourceNodesUnchanged(fake, g, s)
	assertFakeMentions(t, fake, "Generate changed source-side node counts")
}

// TestRunGeneratorSuite_FailsOnDuplicateFixtureName pins the
// fixture-name uniqueness contract.
func TestRunGeneratorSuite_FailsOnDuplicateFixtureName(t *testing.T) {
	t.Parallel()
	fixtures := []plugintest.GeneratorFixture{
		{Name: "dup", BuildStore: func(t *testing.T) *store.Store {
			t.Helper()
			return storefixture.New().Build()
		}},
		{Name: "dup", BuildStore: func(t *testing.T) *store.Store {
			t.Helper()
			return storefixture.New().Build()
		}},
	}
	fake := newFakeT()
	captureFatal(func() {
		plugintest.AssertGeneratorFixtureNamesUnique(fake, fixtures)
	})
	assertFakeMentions(t, fake, "duplicate fixture Name")
}

// TestAssertEmittedTagsAreDeclared covers the tag-declaration check
// in both directions: an undeclared tag is named in the diagnostic,
// and a declared one passes in silence.
//
// The happy-path [plugintest.FixturePlugin] has a no-op Generate, so
// running this check against it iterates zero contributions and
// would report success over no work at all. Every subtest here
// therefore asserts the pending-slot count the check actually
// walked — a green result over an empty list proves nothing, and
// that is precisely the failure this check was added to close.
func TestAssertEmittedTagsAreDeclared(t *testing.T) {
	t.Parallel()

	t.Run("an undeclared output tag is rejected", func(t *testing.T) {
		t.Parallel()
		s := storefixture.New().Struct("User", nil).Build()
		g := &slotCompanionGenerator{
			name:    "companion",
			tag:     "nowhere",
			outputs: []plugin.Output{{Suffix: "_companion.go"}, {Tag: companionTag, Suffix: "_companion_test.go"}},
			newItem: newTaggedCompanion,
		}
		fake := newFakeT()
		plugintest.AssertEmittedTagsAreDeclared(fake, g, s)
		assertQueuedSlots(t, s, 1)
		assertFakeMentions(t, fake, `stamped OutputTag "nowhere" on a`)
	})

	t.Run("a declared output tag passes silently", func(t *testing.T) {
		t.Parallel()
		s := storefixture.New().Struct("User", nil).Build()
		g := &slotCompanionGenerator{
			name:    "companion",
			tag:     companionTag,
			outputs: []plugin.Output{{Suffix: "_companion.go"}, {Tag: companionTag, Suffix: "_companion_test.go"}},
			newItem: newTaggedCompanion,
		}
		fake := newFakeT()
		plugintest.AssertEmittedTagsAreDeclared(fake, g, s)
		assertQueuedSlots(t, s, 1)
		if fake.failed {
			t.Fatalf("declared tag reported a failure: errs=%v fatals=%v", fake.errs, fake.fatals)
		}
	})

	t.Run("the empty primary tag passes without being declared", func(t *testing.T) {
		t.Parallel()
		// The primary output is addressed by the empty tag whether or
		// not the plugin spells it out in Outputs, so a plugin that
		// declares only tagged outputs must still be able to emit
		// against the primary.
		s := storefixture.New().Struct("User", nil).Build()
		g := &slotCompanionGenerator{
			name:    "companion",
			outputs: []plugin.Output{{Tag: companionTag, Suffix: "_companion_test.go"}},
			newItem: newTaggedCompanion,
		}
		fake := newFakeT()
		plugintest.AssertEmittedTagsAreDeclared(fake, g, s)
		assertQueuedSlots(t, s, 1)
		if fake.failed {
			t.Fatalf("primary tag reported a failure: errs=%v fatals=%v", fake.errs, fake.fatals)
		}
	})
}

// TestAssertOutputPackagesTolerateMissingTags covers the
// partial-routing probe in both directions: an implementor that
// assumes its own tag is present is rejected, and one that checks
// before using the entry passes.
//
// The rejection subtest asserts the probe name in the message, not
// merely that something failed. All three probes drive the same
// naive implementor into the same panic, so an assertion on the bare
// phrase "panicked" could not tell a check that runs one probe from
// a check that runs three — and the three shapes are the whole point
// of the contract.
func TestAssertOutputPackagesTolerateMissingTags(t *testing.T) {
	t.Parallel()

	t.Run("an implementor that assumes its tag routed is rejected", func(t *testing.T) {
		t.Parallel()
		s := storefixture.New().Struct("User", nil).Build()
		g := &slotCompanionGenerator{name: "companion", tag: companionTag, newItem: newNaiveCompanion}
		fake := newFakeT()
		plugintest.AssertOutputPackagesTolerateMissingTags(fake, g, s)
		assertQueuedSlots(t, s, 1)
		assertFakeMentions(t, fake, "SetOutputPackages panicked on a")
		assertFakeMentions(t, fake, "when no tag routed")
		assertFakeMentions(t, fake, "when only foreign tags routed")
		assertFakeMentions(t, fake, "when primary routed without a derivable path")
	})

	t.Run("an implementor that checks before using the entry passes silently", func(t *testing.T) {
		t.Parallel()
		s := storefixture.New().Struct("User", nil).Build()
		g := &slotCompanionGenerator{name: "companion", tag: companionTag, newItem: newTolerantCompanion}
		fake := newFakeT()
		plugintest.AssertOutputPackagesTolerateMissingTags(fake, g, s)
		assertQueuedSlots(t, s, 1)
		if fake.failed {
			t.Fatalf("tolerant implementor reported a failure: errs=%v fatals=%v", fake.errs, fake.fatals)
		}
	})

	t.Run("a contribution that implements no setter is skipped silently", func(t *testing.T) {
		t.Parallel()
		s := storefixture.New().Struct("User", nil).Build()
		g := &slotCompanionGenerator{name: "companion", tag: companionTag, newItem: newTaggedCompanion}
		fake := newFakeT()
		plugintest.AssertOutputPackagesTolerateMissingTags(fake, g, s)
		assertQueuedSlots(t, s, 1)
		if fake.failed {
			t.Fatalf("non-implementer reported a failure: errs=%v fatals=%v", fake.errs, fake.fatals)
		}
	})
}

// assertQueuedSlots fails t when s does not hold exactly want queued
// origin-anchored slot contributions.
//
// Both generator-side conformance checks walk
// [store.EmitView.PendingOriginSlots] and say nothing when it is
// empty. Every test above therefore pins the count so a refactor
// that stops the fixture emitting — or stops the check running
// Generate at all — turns the surrounding "passes silently"
// assertions red instead of leaving them vacuously green.
func assertQueuedSlots(t *testing.T, s *store.Store, want int) {
	t.Helper()
	if got := len(s.Emit().PendingOriginSlots()); got != want {
		t.Fatalf("generator queued %d origin slot(s); want %d — the check under test had nothing to walk", got, want)
	}
}

// companionTag is the secondary-output tag the companion fixtures
// below address. A single shared constant keeps the "declared" and
// "stamped" halves of the tag-declaration check impossible to
// desynchronise by typo, which is the very defect that check exists
// to catch in real plugins.
const companionTag = "test"

// companionKind is the [kind.Kind] every companion fixture reports.
// Plugin-defined emit kinds are free-form strings outside the
// framework's own set; the conformance checks never switch on the
// value, they only render it into failure messages.
const companionKind kind.Kind = "plugintest.companion"

// taggedCompanion is the smallest useful plugin-defined emit kind:
// enough of [emit.Node] to be queued into an origin-anchored slot
// while carrying an OutputTag. It deliberately does not implement
// [emit.OutputPackageSetter], so it doubles as the "skipped" case of
// the partial-routing probe.
type taggedCompanion struct {
	emit.BaseEmit
}

// Kind returns [companionKind].
func (*taggedCompanion) Kind() kind.Kind { return companionKind }

// newTaggedCompanion builds a plain tagged contribution anchored to
// origin.
func newTaggedCompanion(origin node.Node, tag string) emit.Node {
	return &taggedCompanion{BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: tag}}
}

// naiveCompanion models the implementor the partial-routing check
// exists to catch: it holds a reference into another of its plugin's
// outputs and derives the referenced package name from the import
// path Layout resolved.
type naiveCompanion struct {
	emit.BaseEmit

	// PkgName is the referenced output's package, derived during
	// [naiveCompanion.SetOutputPackages].
	PkgName string
}

// Kind returns [companionKind].
func (*naiveCompanion) Kind() kind.Kind { return companionKind }

// SetOutputPackages derives the referenced package's name from the
// import path its own tag routed to — assuming, wrongly, that the
// tag is always present.
//
// The unchecked [strings.LastIndex] is the realistic shape of the
// bug: on a fully-routed run the lookup yields a real import path
// and the slice expression is correct, so the defect is invisible
// until a run routes partially. Then the lookup yields "", LastIndex
// returns -1, and the slice bound goes negative.
func (c *naiveCompanion) SetOutputPackages(byTag map[string]string) {
	path := byTag[companionTag]
	c.PkgName = path[:strings.LastIndex(path, "/")]
}

// newNaiveCompanion builds a contribution whose SetOutputPackages
// assumes its tag routed.
func newNaiveCompanion(origin node.Node, tag string) emit.Node {
	return &naiveCompanion{BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: tag}}
}

// tolerantCompanion is the same shape as [naiveCompanion] with the
// contract honoured: it treats a missing or empty entry as "not
// derivable" and leaves its own state alone.
type tolerantCompanion struct {
	emit.BaseEmit

	// PkgName is the referenced output's package, left empty when
	// the tag did not route.
	PkgName string
}

// Kind returns [companionKind].
func (*tolerantCompanion) Kind() kind.Kind { return companionKind }

// SetOutputPackages derives the referenced package's name only when
// the map carries a usable path for its own tag.
func (c *tolerantCompanion) SetOutputPackages(byTag map[string]string) {
	path := byTag[companionTag]
	if cut := strings.LastIndex(path, "/"); cut >= 0 {
		c.PkgName = path[:cut]
	}
}

// newTolerantCompanion builds a contribution whose
// SetOutputPackages honours the partial-routing contract.
func newTolerantCompanion(origin node.Node, tag string) emit.Node {
	return &tolerantCompanion{BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: tag}}
}

// slotCompanionGenerator queues one origin-anchored slot
// contribution per source struct, built by the configured factory
// and stamped with the configured output tag.
//
// Origin-anchored slots rather than a whole [emit.Package] is not an
// arbitrary choice: both checks under test walk
// [store.EmitView.PendingOriginSlots], so a generator emitting
// through AddPackage produces nothing either check can see.
type slotCompanionGenerator struct {
	name string
	tag  string

	// outputs backs [plugin.FilenameProvider.Outputs] for the
	// framework's Go probe language. Leaving it empty makes the
	// generator declare nothing routable, which
	// assertEmittedTagsAreDeclared treats as "no opinion".
	outputs []plugin.Output

	// newItem builds the queued emit value. Swapping it is what
	// distinguishes the tolerant implementor from the naive one
	// without duplicating the generator.
	newItem func(origin node.Node, tag string) emit.Node
}

// Name returns the configured identifier.
func (g *slotCompanionGenerator) Name() string { return g.name }

// Outputs declares the configured output set for the framework's Go
// probe language and nothing for any other.
func (g *slotCompanionGenerator) Outputs(lang string) []plugin.Output {
	if lang != golangProbeLanguage {
		return nil
	}
	return g.outputs
}

// Generate queues one companion per source struct.
func (g *slotCompanionGenerator) Generate(ctx *plugin.GeneratorContext) error {
	for _, s := range ctx.Store.Nodes().Structs().Items() {
		item := g.newItem(s, g.tag)
		prov := emit.Provenance{SetBy: g.name, Pos: s.Pos()}
		if err := ctx.Store.Emit().AppendOriginSlot(s, "top", item, prov); err != nil {
			return fmt.Errorf("slotCompanionGenerator: AppendOriginSlot: %w", err)
		}
	}
	return nil
}

// golangProbeLanguage is the language identifier the conformance
// suite drives capability lookups with. Declared here rather than
// imported so the fixture stays independent of the backend module.
const golangProbeLanguage = "golang"

// mirroringGenerator emits a single output [emit.Struct] per
// source [node.Struct], with a deterministic name and target
// derived from the source. Idempotent / deterministic by
// construction — re-running with equivalent inputs produces an
// equivalent emit set.
type mirroringGenerator struct{ name string }

// Name returns the configured identifier.
func (g *mirroringGenerator) Name() string { return g.name }

// Generate copies every source struct into one emit struct in
// a single output package.
func (*mirroringGenerator) Generate(ctx *plugin.GeneratorContext) error {
	structs := ctx.Store.Nodes().Structs().Items()
	if len(structs) == 0 {
		return nil
	}
	pkg := &emit.Package{
		Name: "mirror",
		Path: "example.com/mirror",
	}
	for _, src := range structs {
		pkg.Structs = append(pkg.Structs, &emit.Struct{
			Name:    src.Name + "Mirror",
			Package: pkg.Name,
			Target: emit.Target{
				Dir:      pkg.Path,
				Filename: src.Name + "_mirror.go",
				Package:  pkg.Name,
			},
		})
	}
	if err := ctx.Store.Emit().AddPackage(pkg); err != nil {
		return fmt.Errorf("mirroringGenerator: AddPackage: %w", err)
	}
	return nil
}

// panickingGenerator panics in Generate. Used to verify the
// empty-store probe recovers and reports a contract failure.
type panickingGenerator struct{ name string }

// Name returns the configured identifier.
func (g *panickingGenerator) Name() string { return g.name }

// Generate panics on every call.
func (*panickingGenerator) Generate(_ *plugin.GeneratorContext) error {
	panic("plugintest test: panickingGenerator panicking on purpose") //nolint:forbidigo
}

// flappingGenerator emits a struct whose name embeds a
// per-instance counter, so two distinct generator instances
// produce identical output on equivalent inputs but the *same*
// instance varies its output across two calls. The suite's
// determinism check runs against the same generator instance
// for both passes, so this exhibits the flapping behaviour the
// check is supposed to catch.
type flappingGenerator struct {
	name  string
	count int
}

// Name returns the configured identifier.
func (g *flappingGenerator) Name() string { return g.name }

// Generate emits a struct whose name embeds the per-call
// counter, breaking determinism across runs.
func (g *flappingGenerator) Generate(ctx *plugin.GeneratorContext) error {
	g.count++
	pkg := &emit.Package{Name: "flap", Path: "example.com/flap"}
	pkg.Structs = append(pkg.Structs, &emit.Struct{
		Name:    fmt.Sprintf("Flap%d", g.count),
		Package: pkg.Name,
		Target: emit.Target{
			Dir:      pkg.Path,
			Filename: fmt.Sprintf("flap_%d.go", g.count),
			Package:  pkg.Name,
		},
	})
	if err := ctx.Store.Emit().AddPackage(pkg); err != nil {
		return fmt.Errorf("flappingGenerator: AddPackage: %w", err)
	}
	return nil
}

// sourceMutatingGenerator violates the frozen-source contract
// by appending a synthetic struct to the source side of the
// store during Generate.
type sourceMutatingGenerator struct{ name string }

// Name returns the configured identifier.
func (g *sourceMutatingGenerator) Name() string { return g.name }

// Generate adds a synthetic struct to the first source package
// it sees.
func (*sourceMutatingGenerator) Generate(ctx *plugin.GeneratorContext) error {
	pkgs := ctx.Store.Nodes().Packages().Items()
	if len(pkgs) == 0 {
		return nil
	}
	pkg := pkgs[0]
	if err := ctx.Store.Nodes().Structs().Add("Injected", nil); err != nil {
		return fmt.Errorf("sourceMutatingGenerator: Add: %w", err)
	}
	_ = pkg
	return nil
}
