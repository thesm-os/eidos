// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest_test

import (
	"errors"
	"fmt"
	"testing"

	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// TestRunFrontendSuite_PassesForWellFormedFrontend pins the
// happy path: a frontend that emits a stable per-pattern node
// graph passes every contract.
func TestRunFrontendSuite_PassesForWellFormedFrontend(t *testing.T) {
	t.Parallel()
	plugintest.RunFrontendSuite(
		t,
		&fakeFrontend{name: "fake-fe"},
		[]plugintest.FrontendFixture{
			{
				Name:    "single-package pattern",
				Pattern: "single",
				Options: map[string]string{"label": "alpha"},
			},
			{
				Name:    "two-package pattern",
				Pattern: "two",
				Options: map[string]string{"label": "beta"},
			},
		},
	)
}

// TestRunFrontendSuite_RejectsPanickingFrontend covers the
// empty-pattern panic rejection.
func TestRunFrontendSuite_RejectsPanickingFrontend(t *testing.T) {
	t.Parallel()
	f := &panickingFrontend{name: "panicky"}
	fake := newFakeT()
	plugintest.AssertLoadEmptyPatternDoesNotPanic(fake, f, plugintest.FrontendFixture{Name: "any"})
	assertFakeMentions(t, fake, "Load panicked on empty pattern")
}

// TestRunFrontendSuite_RejectsNonDeterministicFrontend pins
// the determinism contract: a frontend whose output varies
// across calls (per-call counter, time-derived names) fails
// the comparison.
func TestRunFrontendSuite_RejectsNonDeterministicFrontend(t *testing.T) {
	t.Parallel()
	f := &flappingFrontend{name: "flap"}
	fx := plugintest.FrontendFixture{Name: "single", Pattern: "single"}
	fake := newFakeTIn(t)
	plugintest.AssertLoadIsDeterministic(fake, f, fx)
	assertFakeMentions(t, fake, "frontend is not deterministic")
}

// TestRunFrontendSuite_FailsOnDuplicateFixtureName pins the
// fixture-name uniqueness contract.
func TestRunFrontendSuite_FailsOnDuplicateFixtureName(t *testing.T) {
	t.Parallel()
	fixtures := []plugintest.FrontendFixture{
		{Name: "dup", Pattern: "p"},
		{Name: "dup", Pattern: "p"},
	}
	fake := newFakeT()
	captureFatal(func() {
		plugintest.AssertFrontendFixtureNamesUnique(fake, fixtures)
	})
	assertFakeMentions(t, fake, "duplicate fixture Name")
}

// TestRunFrontendSuite_SurfacesRejectedFixtureOptions pins the
// fixture-shape contract: a frontend whose own schema rejects the
// fixture's Options is a broken fixture, and the suite must say so
// rather than quietly skipping every Load it was asked to drive.
func TestRunFrontendSuite_SurfacesRejectedFixtureOptions(t *testing.T) {
	t.Parallel()

	t.Run("a frontend rejecting its own fixture options fails with a named contract failure", func(t *testing.T) {
		t.Parallel()
		f := &requiredOptionFrontend{name: "needs-root"}
		fx := plugintest.FrontendFixture{Name: "root omitted", Pattern: "single"}
		fake := newFakeT()
		captureFatal(func() { plugintest.AssertLoadDoesNotPanic(fake, f, fx) })
		assertFakeMentions(t, fake, "root")
	})

	t.Run("a frontend rejecting its own fixture options never reaches Load", func(t *testing.T) {
		t.Parallel()
		f := &requiredOptionFrontend{name: "needs-root"}
		fx := plugintest.FrontendFixture{Name: "root omitted", Pattern: "single"}
		fake := newFakeTIn(t)
		captureFatal(func() { plugintest.AssertLoadDoesNotPanic(fake, f, fx) })
		captureFatal(func() { plugintest.AssertLoadIsDeterministic(fake, f, fx) })
		if !fake.Failed() {
			t.Errorf("the suite passed a fixture the frontend rejected; Load ran %d times", f.loads)
		}
	})

	t.Run("Load is invoked at least once per fixture", func(t *testing.T) {
		t.Parallel()
		f := &requiredOptionFrontend{name: "needs-root"}
		fake := newFakeT()
		plugintest.AssertLoadDoesNotPanic(fake, f, validRootFixture())
		if f.loads == 0 {
			t.Errorf("Load was never invoked for a fixture the frontend accepts: %s", joinFake(fake))
		}
	})

	t.Run("the empty-pattern probe reaches Load when a fixture supplies options", func(t *testing.T) {
		t.Parallel()
		f := &requiredOptionFrontend{name: "needs-root"}
		fake := newFakeT()
		plugintest.AssertLoadEmptyPatternDoesNotPanic(fake, f, validRootFixture())
		if f.loads == 0 {
			t.Errorf("the empty-pattern probe never reached Load: %s", joinFake(fake))
		}
		if fake.Failed() {
			t.Errorf("the empty-pattern probe failed a conformant frontend: %s", joinFake(fake))
		}
	})
}

// TestRunFrontendSuite_RequiresAPopulatedStore pins the populate
// contract: a frontend whose Load records nothing leaves two empty
// projections behind, which every other check compares as equal.
func TestRunFrontendSuite_RequiresAPopulatedStore(t *testing.T) {
	t.Parallel()

	t.Run("a frontend that loads nothing fails the populate check", func(t *testing.T) {
		t.Parallel()
		f := &inertFrontend{name: "inert"}
		fx := plugintest.FrontendFixture{Name: "expects nodes", Pattern: "single"}
		fake := newFakeT()
		captureFatal(func() { plugintest.AssertLoadPopulatesStore(fake, f, fx) })
		assertFakeMentions(t, fake, "Load recorded no nodes")
	})

	t.Run("a fixture declaring ExpectsEmpty tolerates an empty store", func(t *testing.T) {
		t.Parallel()
		f := &inertFrontend{name: "inert"}
		fx := plugintest.FrontendFixture{Name: "deliberately empty", Pattern: "single", ExpectsEmpty: true}
		fake := newFakeT()
		captureFatal(func() { plugintest.AssertLoadPopulatesStore(fake, f, fx) })
		if fake.Failed() {
			t.Errorf("ExpectsEmpty did not exempt a deliberately-empty fixture: %s", joinFake(fake))
		}
	})
}

// TestRunFrontendSuite_RequiresFingerprintKeyedCaches pins the
// frontend contract's only capitalised MUST — fold
// [plugin.FrontendContext.Fingerprint] into the cache key — which no
// test in the workspace observed while the suite supplied no
// fingerprint and a cache that could not hit.
func TestRunFrontendSuite_RequiresFingerprintKeyedCaches(t *testing.T) {
	t.Parallel()

	t.Run("a frontend that omits Fingerprint from its cache key serves a stale graph", func(t *testing.T) {
		t.Parallel()
		f := &cachingFrontend{name: "pattern-keyed"}
		fx := plugintest.FrontendFixture{Name: "cached", Pattern: "single"}
		fake := newFakeTIn(t)
		captureFatal(func() { plugintest.AssertLoadIsFingerprintKeyed(fake, f, fx) })
		assertFakeMentions(t, fake, "MUST be folded into")
		if f.parses != 1 {
			t.Errorf("pattern-keyed frontend parsed %d times; want 1 (the second pass served the cache)", f.parses)
		}
	})

	t.Run("a frontend that folds Fingerprint re-parses on a changed composition", func(t *testing.T) {
		t.Parallel()
		f := &cachingFrontend{name: "fingerprint-keyed", foldFingerprint: true}
		fx := plugintest.FrontendFixture{Name: "cached", Pattern: "single"}
		fake := newFakeTIn(t)
		captureFatal(func() { plugintest.AssertLoadIsFingerprintKeyed(fake, f, fx) })
		if fake.Failed() {
			t.Errorf("a correctly-keyed frontend failed the fingerprint check: %s", joinFake(fake))
		}
		if f.parses != 2 {
			t.Errorf("fingerprint-keyed frontend parsed %d times; want 2 (one per composition)", f.parses)
		}
	})

	t.Run("a frontend that ignores the cache entirely passes both fingerprint passes", func(t *testing.T) {
		t.Parallel()
		f := &fakeFrontend{name: "cacheless"}
		fx := plugintest.FrontendFixture{
			Name:    "single-package pattern",
			Pattern: "single",
			Options: map[string]string{"label": "alpha"},
		}
		fake := newFakeTIn(t)
		captureFatal(func() { plugintest.AssertLoadIsFingerprintKeyed(fake, f, fx) })
		if fake.Failed() {
			t.Errorf("the fingerprint check mandated caching: %s", joinFake(fake))
		}
	})
}

// validRootFixture returns the fixture [requiredOptionFrontend]
// accepts. Shared by the probes that assert Load was reached, so the
// option name cannot drift out of step with the schema in one place
// and not the other.
func validRootFixture() plugintest.FrontendFixture {
	return plugintest.FrontendFixture{
		Name:    "root supplied",
		Pattern: "single",
		Options: map[string]string{"root": "example.com/req"},
	}
}

// requiredOptionFrontend declares one required option and counts
// Load invocations. It models the frontend the suite silently
// disabled: a schema whose required field a fixture omits makes
// SetOptions fail before Load is ever reached.
type requiredOptionFrontend struct {
	name  string
	opts  requiredOptionOpts
	loads int
}

// requiredOptionOpts declares a single required field so a fixture
// omitting it trips [opt.ErrMissingRequired] on decode.
type requiredOptionOpts struct {
	// Root is the input root the frontend would load from. Required
	// with no default, exactly like the first such field either
	// in-tree frontend's schema would gain.
	Root string `eidos:"root,required"`
}

// Name returns the configured identifier.
func (f *requiredOptionFrontend) Name() string { return f.name }

// OptionsSchema returns the reflected schema of
// [requiredOptionOpts].
func (*requiredOptionFrontend) OptionsSchema() opt.Schema { return opt.Reflect(requiredOptionOpts{}) }

// SetOptions decodes opts, failing when the required root is absent.
func (f *requiredOptionFrontend) SetOptions(opts opt.Options) error {
	if err := opts.Decode(&f.opts); err != nil {
		return fmt.Errorf("requiredOptionFrontend: SetOptions: %w", err)
	}
	return nil
}

// Load counts the invocation and records one package so a suite
// that reaches it can tell the difference from one that did not.
func (f *requiredOptionFrontend) Load(ctx *plugin.FrontendContext) error {
	f.loads++
	if ctx.Pattern == "" {
		return nil
	}
	pkg := &node.Package{Name: "req", Path: "example.com/req"}
	pkg.Structs = []*node.Struct{{Name: "Root", Package: pkg.Path}}
	if err := ctx.Store.Nodes().AddPackage(pkg); err != nil {
		return fmt.Errorf("requiredOptionFrontend: AddPackage: %w", err)
	}
	return nil
}

// inertFrontend satisfies [plugin.Frontend] and records nothing.
// Models the frontend whose Load body is `return nil` — the shape
// every determinism comparison of two empty stores reports as
// deterministic.
type inertFrontend struct{ name string }

// Name returns the configured identifier.
func (f *inertFrontend) Name() string { return f.name }

// Load records nothing and returns cleanly.
func (*inertFrontend) Load(_ *plugin.FrontendContext) error { return nil }

// cachingFrontend memoises its node graph through ctx.Cache, keyed
// on the pattern alone or on the pattern plus ctx.Fingerprint
// depending on foldFingerprint. The two shapes are the conformant
// and the mis-keyed frontend the fingerprint check must tell apart;
// parses counts the misses so a test can assert which path ran.
type cachingFrontend struct {
	name            string
	foldFingerprint bool
	parses          int
}

// cachedPackagePath is the single package cachingFrontend records.
// It doubles as the cache payload — the graph is small enough that
// the path alone reconstitutes it.
const cachedPackagePath = "example.com/cached"

// Name returns the configured identifier.
func (f *cachingFrontend) Name() string { return f.name }

// Load consults the cache under its composed key, recording the
// cached package on a hit and parsing plus storing on a miss.
func (f *cachingFrontend) Load(ctx *plugin.FrontendContext) error {
	key := "cachingFrontend:" + ctx.Pattern
	if f.foldFingerprint {
		key += ":" + ctx.Fingerprint
	}
	if body, hit := ctx.Cache.Get(key); hit {
		return f.record(ctx, string(body))
	}
	f.parses++
	if err := ctx.Cache.Put(key, []byte(cachedPackagePath)); err != nil {
		return fmt.Errorf("cachingFrontend: Put %q: %w", key, err)
	}
	return f.record(ctx, cachedPackagePath)
}

// record adds the single package both the hit and the miss path
// converge on.
func (*cachingFrontend) record(ctx *plugin.FrontendContext, path string) error {
	pkg := &node.Package{Name: "cached", Path: path}
	pkg.Structs = []*node.Struct{{Name: "Cached", Package: path}}
	if err := ctx.Store.Nodes().AddPackage(pkg); err != nil {
		return fmt.Errorf("cachingFrontend: AddPackage %q: %w", path, err)
	}
	return nil
}

// fakeFrontend is an in-memory frontend that populates the
// store from a pattern-keyed lookup table. Deterministic by
// construction — equivalent patterns produce equivalent node
// graphs.
type fakeFrontend struct {
	name string
	opts fakeFrontendOpts
}

// fakeFrontendOpts is the bound options the fake frontend
// exposes through OptionsProvider so the suite exercises that
// integration path.
type fakeFrontendOpts struct {
	// Label is a free-text annotation the suite passes through
	// fixtures; the frontend stores it on receiver state.
	Label string `eidos:"label"`
}

// Name returns the configured identifier.
func (f *fakeFrontend) Name() string { return f.name }

// OptionsSchema returns the reflected schema of
// [fakeFrontendOpts].
func (*fakeFrontend) OptionsSchema() opt.Schema { return opt.Reflect(fakeFrontendOpts{}) }

// SetOptions decodes opts into the frontend's options.
func (f *fakeFrontend) SetOptions(opts opt.Options) error {
	if err := opts.Decode(&f.opts); err != nil {
		return fmt.Errorf("fakeFrontend: SetOptions: %w", err)
	}
	return nil
}

// Load adds packages keyed by the pattern. Unknown patterns
// add nothing; the empty pattern is treated as "no input" and
// returns cleanly.
func (f *fakeFrontend) Load(ctx *plugin.FrontendContext) error {
	switch ctx.Pattern {
	case "single":
		return f.addPackage(ctx, "example.com/one", "Alpha", "Beta")
	case "two":
		if err := f.addPackage(ctx, "example.com/one", "Alpha"); err != nil {
			return err
		}
		return f.addPackage(ctx, "example.com/two", "Gamma")
	default:
		return nil
	}
}

// addPackage seeds a single package with the supplied struct
// names. Helper for the per-pattern dispatch above.
func (*fakeFrontend) addPackage(ctx *plugin.FrontendContext, path string, structNames ...string) error {
	pkg := &node.Package{Name: pkgShortName(path), Path: path}
	for _, name := range structNames {
		pkg.Structs = append(pkg.Structs, &node.Struct{Name: name, Package: path})
	}
	if err := ctx.Store.Nodes().AddPackage(pkg); err != nil {
		return fmt.Errorf("fakeFrontend: AddPackage %q: %w", path, err)
	}
	return nil
}

// pkgShortName extracts a short package name from a slash-
// separated path. Helper for [fakeFrontend.addPackage].
func pkgShortName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// panickingFrontend panics in Load. Used to verify the
// empty-pattern panic-recovery probe.
type panickingFrontend struct{ name string }

// Name returns the configured identifier.
func (f *panickingFrontend) Name() string { return f.name }

// Load panics on every invocation.
func (*panickingFrontend) Load(_ *plugin.FrontendContext) error {
	panic("plugintest test: panickingFrontend panicking on purpose") //nolint:forbidigo
}

// flappingFrontend produces a different node graph on each
// call by embedding a per-instance counter in the struct names.
type flappingFrontend struct {
	name  string
	count int
}

// Name returns the configured identifier.
func (f *flappingFrontend) Name() string { return f.name }

// Load adds one struct whose name embeds the per-call counter.
func (f *flappingFrontend) Load(ctx *plugin.FrontendContext) error {
	f.count++
	pkg := &node.Package{Name: "flap", Path: "example.com/flap"}
	pkg.Structs = []*node.Struct{{
		Name:    fmt.Sprintf("Flap%d", f.count),
		Package: pkg.Path,
	}}
	if err := ctx.Store.Nodes().AddPackage(pkg); err != nil {
		return fmt.Errorf("flappingFrontend: AddPackage: %w", err)
	}
	return errors.New("flappingFrontend: deliberate return error") //nolint:err113
}
