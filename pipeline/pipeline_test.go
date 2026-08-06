// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline_test

import (
	"cmp"
	"maps"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
	"go.thesmos.sh/eidos/store"
)

func buildBasic(t *testing.T) *pipeline.Pipeline {
	t.Helper()
	p, err := pipeline.New().
		WithFrontend(&stubFE{name: "fe1"}).
		WithFrontend(&stubFE{name: "fe2"}).
		WithAnnotator(&stubAnn{name: "ann1"}).
		WithAnnotator(&stubAnn{name: "ann2"}).
		WithGenerator(&stubGen{name: "gen1"}).
		WithGenerator(&stubGen{name: "gen2"}).
		WithBackend(&stubBE{name: "be"}).
		WithSink(sink.NewMemory()).
		WithCache(cache.NewNone()).
		WithVerbose(true).
		Build()
	assertNoError(t, err)
	return p
}

func TestPipeline_Frontends(t *testing.T) {
	t.Parallel()

	t.Run("returns frontends in registration order", func(t *testing.T) {
		t.Parallel()
		p := buildBasic(t)
		got := p.Frontends()
		if len(got) != 2 || got[0].Name() != "fe1" || got[1].Name() != "fe2" {
			t.Fatalf("Frontends order mismatch: %+v", got)
		}
	})

	t.Run("returns a defensive copy", func(t *testing.T) {
		t.Parallel()
		p := buildBasic(t)
		first := p.Frontends()
		first[0] = nil
		if p.Frontends()[0] == nil {
			t.Fatalf("Frontends should return a defensive copy")
		}
	})
}

func TestPipeline_Annotators(t *testing.T) {
	t.Parallel()

	t.Run("returns annotators in registration order", func(t *testing.T) {
		t.Parallel()
		p := buildBasic(t)
		got := p.Annotators()
		if len(got) != 2 || got[0].Name() != "ann1" || got[1].Name() != "ann2" {
			t.Fatalf("Annotators order mismatch: %+v", got)
		}
	})

	t.Run("returns a defensive copy", func(t *testing.T) {
		t.Parallel()
		p := buildBasic(t)
		first := p.Annotators()
		first[0] = nil
		if p.Annotators()[0] == nil {
			t.Fatalf("Annotators should return a defensive copy")
		}
	})
}

func TestPipeline_Generators(t *testing.T) {
	t.Parallel()

	t.Run("returns generators in registration order", func(t *testing.T) {
		t.Parallel()
		p := buildBasic(t)
		got := p.Generators()
		if len(got) != 2 || got[0].Name() != "gen1" || got[1].Name() != "gen2" {
			t.Fatalf("Generators order mismatch: %+v", got)
		}
	})

	t.Run("returns a defensive copy", func(t *testing.T) {
		t.Parallel()
		p := buildBasic(t)
		first := p.Generators()
		first[0] = nil
		if p.Generators()[0] == nil {
			t.Fatalf("Generators should return a defensive copy")
		}
	})
}

func TestPipeline_Backend(t *testing.T) {
	t.Parallel()

	t.Run("returns the registered backend", func(t *testing.T) {
		t.Parallel()
		p := buildBasic(t)
		if p.Backend() == nil || p.Backend().Name() != "be" {
			t.Fatalf("Backend mismatch: %+v", p.Backend())
		}
	})
}

func TestPipeline_Sink(t *testing.T) {
	t.Parallel()

	t.Run("returns the configured sink", func(t *testing.T) {
		t.Parallel()
		p := buildBasic(t)
		if p.Sink() == nil {
			t.Fatalf("Sink should be non-nil after WithSink")
		}
	})

	t.Run("returns nil when no sink was configured", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		assertNoError(t, err)
		if p.Sink() != nil {
			t.Fatalf("Sink should be nil when WithSink is omitted")
		}
	})
}

func TestPipeline_Cache(t *testing.T) {
	t.Parallel()

	t.Run("returns the configured cache", func(t *testing.T) {
		t.Parallel()
		p := buildBasic(t)
		if p.Cache() == nil {
			t.Fatalf("Cache should be non-nil after WithCache")
		}
	})
}

func TestPipeline_Diag(t *testing.T) {
	t.Parallel()

	t.Run("returns the configured diagnostic sink", func(t *testing.T) {
		t.Parallel()
		p := buildBasic(t)
		if p.Diag() == nil {
			t.Fatalf("Diag should be non-nil")
		}
	})
}

func TestPipeline_Verbose(t *testing.T) {
	t.Parallel()

	t.Run("reports the configured verbose flag", func(t *testing.T) {
		t.Parallel()
		if !buildBasic(t).Verbose() {
			t.Fatalf("Verbose should reflect the configured value")
		}
	})

	t.Run("defaults to false", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		assertNoError(t, err)
		if p.Verbose() {
			t.Fatalf("Verbose should default to false")
		}
	})
}

func TestPipeline_Plan(t *testing.T) {
	t.Parallel()

	t.Run("returns the resolved plan after a successful Build", func(t *testing.T) {
		t.Parallel()
		p := buildBasic(t)
		plan := p.Plan()
		if plan == nil {
			t.Fatalf("Plan should be non-nil after a successful Build")
		}
		if len(plan.Frontends) != 2 || len(plan.Annotators) != 2 || len(plan.Generators) != 2 {
			t.Fatalf("plan slice lengths mismatch: %+v", plan)
		}
		if plan.Backend == nil || plan.Backend.Name() != "be" {
			t.Fatalf("plan backend mismatch")
		}
	})
}

func TestPipeline_DirectiveRegistry(t *testing.T) {
	t.Parallel()

	t.Run("returns a non-nil registry even when no schemas are registered", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		assertNoError(t, err)
		if p.DirectiveRegistry() == nil {
			t.Fatalf("DirectiveRegistry should be non-nil after Build")
		}
	})
}

// TestPipeline_Store covers the post-run store accessor. Before Run
// fires, Store returns nil — the cache is populated on Run entry, so
// tests inspecting plan/registry/backend state pre-run see the nil
// signal that no store yet exists. After Run, Store returns the same
// instance the phases populated, so post-run inspection of node /
// emit content goes through one canonical accessor.
func TestPipeline_Store(t *testing.T) {
	t.Parallel()

	t.Run("returns nil before Run is invoked", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		if p.Store() != nil {
			t.Fatalf("Store should be nil before Run; got %v", p.Store())
		}
	})

	t.Run("returns the populated store after Run completes", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if p.Store() == nil {
			t.Fatalf("Store should be non-nil after Run")
		}
	})

	t.Run("re-running replaces the previously cached store", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		first := p.Store()
		assertNoError(t, p.Run(t.Context()))
		second := p.Store()
		if first == second {
			t.Fatalf("re-running should replace the cached store; got identical pointers")
		}
	})
}

// TestPipeline_EmittedTargets pins the "current claims" view — the
// set of targets prune diffs the prior manifest against. Everything
// in the manifest that this set does NOT contain (and whose pipeline
// and scope match) is an orphan prune deletes from disk.
//
// The accessor had no test at all, so negating its nil-recorder guard
// survived mutation testing with the widest blast radius of any
// survivor in the package: after a real Run the recorder is non-nil,
// the negated guard returns the EMPTY set, and prune reads that as
// "this run claimed nothing" — every generated file in the manifest
// becomes an orphan and is deleted.
//
// The post-Run assertion therefore compares the set's exact contents.
// A len > 0 or non-nil check would pass against the empty map the
// mutation returns, which is exactly the value that destroys the
// generated corpus.
func TestPipeline_EmittedTargets(t *testing.T) {
	t.Parallel()

	// sorted renders a claim set deterministically for the failure
	// message; ranging the map directly would print the same failure
	// differently on each run.
	sorted := func(m map[emit.Target]struct{}) []emit.Target {
		return slices.SortedFunc(maps.Keys(m), func(a, b emit.Target) int {
			return cmp.Or(
				cmp.Compare(a.Dir, b.Dir),
				cmp.Compare(a.Filename, b.Filename),
				cmp.Compare(a.Package, b.Package),
				cmp.Compare(a.ImportPath, b.ImportPath),
			)
		})
	}

	t.Run("a pipeline that has not run claims an empty set rather than panicking", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		got := p.EmittedTargets()
		if got == nil {
			t.Fatalf("EmittedTargets before Run = nil, want an empty non-nil map")
		}
		if len(got) != 0 {
			t.Fatalf("EmittedTargets before Run = %v, want no claims", sorted(got))
		}
	})

	t.Run("the claimed set is exactly the targets the backend wrote", func(t *testing.T) {
		t.Parallel()
		want := map[emit.Target]struct{}{
			{Dir: "internal/repo", Filename: "order_gen.go", Package: "repo"}: {},
			{Dir: "internal/repo", Filename: "user_gen.go", Package: "repo"}:  {},
		}
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				for target := range want {
					_ = ctx.Sink.Write(target, []byte("body"))
				}
			},
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if got := p.EmittedTargets(); !maps.Equal(got, want) {
			t.Fatalf("EmittedTargets = %+v, want %+v", sorted(got), sorted(want))
		}
	})
}

// TestPipeline_LayoutPolicyFor pins the resolved-policy accessor:
// the default policy is alongside-source with empty package/dir;
// the With* overrides populate the corresponding fields; the
// accessor returns the same policy regardless of plugin name in
// this phase (per-plugin merge layers arrive later).
func TestPipeline_LayoutPolicyFor(t *testing.T) {
	t.Parallel()

	t.Run("default policy is alongside-source with empty package and dir", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		got := p.LayoutPolicyFor("any-plugin")
		if got.Layout != pipeline.LayoutAlongsideSource {
			t.Fatalf("default Layout = %q, want %q", got.Layout, pipeline.LayoutAlongsideSource)
		}
		if got.Package != "" || got.Dir != "" {
			t.Fatalf("default Package/Dir should be empty; got %+v", got)
		}
	})

	t.Run("With* overrides populate the resolved policy", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithOutputLayout(pipeline.LayoutCentralised).
			WithOutputPackage("gen").
			WithOutputDir("internal/gen").
			Build()
		assertNoError(t, err)
		got := p.LayoutPolicyFor("repogen")
		want := pipeline.LayoutPolicy{
			Layout:      pipeline.LayoutCentralised,
			LayoutFrom:  manifest.LayerCLI,
			Package:     "gen",
			PackageFrom: manifest.LayerCLI,
			Dir:         "internal/gen",
			DirFrom:     manifest.LayerCLI,
		}
		if got != want {
			t.Fatalf("LayoutPolicyFor = %+v, want %+v", got, want)
		}
	})

	t.Run("default policy stamps every From field as framework", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		got := p.LayoutPolicyFor("repogen")
		want := pipeline.LayoutPolicy{
			Layout:      pipeline.LayoutAlongsideSource,
			LayoutFrom:  manifest.LayerFramework,
			PackageFrom: manifest.LayerFramework,
			DirFrom:     manifest.LayerFramework,
		}
		if got != want {
			t.Fatalf("LayoutPolicyFor = %+v, want %+v (default)", got, want)
		}
	})

	t.Run("NewLayoutPolicy seeds the framework-default attribution", func(t *testing.T) {
		t.Parallel()
		got := pipeline.NewLayoutPolicy()
		want := pipeline.LayoutPolicy{
			Layout:      pipeline.LayoutAlongsideSource,
			LayoutFrom:  manifest.LayerFramework,
			PackageFrom: manifest.LayerFramework,
			DirFrom:     manifest.LayerFramework,
		}
		if got != want {
			t.Fatalf("NewLayoutPolicy = %+v, want %+v", got, want)
		}
	})

	t.Run("project layer stamps LayerProject on the touched fields", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithProjectOutput(pipeline.LayoutCentralised, "gen", "internal/gen").
			Build()
		assertNoError(t, err)
		got := p.LayoutPolicyFor("repogen")
		want := pipeline.LayoutPolicy{
			Layout:      pipeline.LayoutCentralised,
			LayoutFrom:  manifest.LayerProject,
			Package:     "gen",
			PackageFrom: manifest.LayerProject,
			Dir:         "internal/gen",
			DirFrom:     manifest.LayerProject,
		}
		if got != want {
			t.Fatalf("LayoutPolicyFor = %+v, want %+v", got, want)
		}
	})

	t.Run("per-plugin layer overrides project per field", func(t *testing.T) {
		t.Parallel()
		gen := &stubGen{name: "mockgen"}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(gen).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithProjectOutput(pipeline.LayoutAlongsideSource, "gen", "").
			WithPluginOutput("mockgen", pipeline.LayoutCentralised, "mocks", "internal/mocks").
			Build()
		assertNoError(t, err)
		got := p.LayoutPolicyFor("mockgen")
		want := pipeline.LayoutPolicy{
			Layout:      pipeline.LayoutCentralised,
			LayoutFrom:  manifest.LayerPerPlugin,
			Package:     "mocks",
			PackageFrom: manifest.LayerPerPlugin,
			Dir:         "internal/mocks",
			DirFrom:     manifest.LayerPerPlugin,
		}
		if got != want {
			t.Fatalf("LayoutPolicyFor(mockgen) = %+v, want %+v", got, want)
		}
	})

	t.Run("per-plugin layer leaves unset fields at the project layer", func(t *testing.T) {
		t.Parallel()
		gen := &stubGen{name: "mockgen"}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(gen).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithProjectOutput("", "gen", "").
			WithPluginOutput("mockgen", pipeline.LayoutCentralised, "", "").
			Build()
		assertNoError(t, err)
		got := p.LayoutPolicyFor("mockgen")
		want := pipeline.LayoutPolicy{
			Layout:      pipeline.LayoutCentralised,
			LayoutFrom:  manifest.LayerPerPlugin,
			Package:     "gen",
			PackageFrom: manifest.LayerProject,
			DirFrom:     manifest.LayerFramework,
		}
		if got != want {
			t.Fatalf("LayoutPolicyFor(mockgen) = %+v, want %+v (per-plugin Layout, project Package)", got, want)
		}
	})

	t.Run("CLI layer overrides per-plugin override", func(t *testing.T) {
		t.Parallel()
		gen := &stubGen{name: "mockgen"}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(gen).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithPluginOutput("mockgen", pipeline.LayoutCentralised, "mocks", "internal/mocks").
			WithOutputPackage("cli-pkg").
			Build()
		assertNoError(t, err)
		got := p.LayoutPolicyFor("mockgen")
		if got.Package != "cli-pkg" || got.PackageFrom != manifest.LayerCLI {
			t.Fatalf("CLI override missing: %+v", got)
		}
		if got.Dir != "internal/mocks" || got.DirFrom != manifest.LayerPerPlugin {
			t.Fatalf("per-plugin Dir should stick when CLI doesn't override it: %+v", got)
		}
	})

	t.Run("unknown plugin name returns the project + CLI default", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithProjectOutput("", "gen", "").
			Build()
		assertNoError(t, err)
		got := p.LayoutPolicyFor("not-registered")
		if got.Package != "gen" || got.PackageFrom != manifest.LayerProject {
			t.Fatalf("unknown-plugin default = %+v, want project Package", got)
		}
	})
}

// TestPipeline_OutputFilename pins the CLI `-o` accessor: empty
// when not configured, the literal value when set.
func TestPipeline_OutputFilename(t *testing.T) {
	t.Parallel()

	t.Run("default OutputFilename is the empty string", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		if got := p.OutputFilename(); got != "" {
			t.Fatalf("default OutputFilename = %q, want empty", got)
		}
	})

	t.Run("WithOutputFilename threads to the Pipeline accessor", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithOutputFilename("gen.go").
			Build()
		assertNoError(t, err)
		if got := p.OutputFilename(); got != "gen.go" {
			t.Fatalf("OutputFilename = %q, want %q", got, "gen.go")
		}
	})
}

// TestPipeline_TargetSymbolAndScope pins the scope-filter accessor
// pair: empty symbol → empty TargetSymbol + nil Scope; non-empty
// symbol → literal TargetSymbol + non-nil Scope that matches the
// directly-named source decl.
func TestPipeline_TargetSymbolAndScope(t *testing.T) {
	t.Parallel()

	t.Run("default TargetSymbol is empty and Scope is nil", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		if got := p.TargetSymbol(); got != "" {
			t.Fatalf("default TargetSymbol = %q, want empty", got)
		}
		if p.Scope() != nil {
			t.Fatalf("default Scope should be nil")
		}
	})

	t.Run("WithTargetSymbol pins TargetSymbol and produces a matching predicate", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithTargetSymbol("Article").
			Build()
		assertNoError(t, err)
		if got := p.TargetSymbol(); got != "Article" {
			t.Fatalf("TargetSymbol = %q, want %q", got, "Article")
		}
		scope := p.Scope()
		if scope == nil {
			t.Fatalf("non-empty target symbol should yield a non-nil Scope")
		}
		if !scope(&node.Struct{Name: "Article"}) {
			t.Fatalf("scope should match a Struct named Article")
		}
		if scope(&node.Struct{Name: "Other"}) {
			t.Fatalf("scope should not match a Struct named Other")
		}
	})

	t.Run("scope reaches plugin contexts via the per-plugin Reader", func(t *testing.T) {
		t.Parallel()
		var seenStructs int
		fe := &recFE{
			name: "fe",
			loadFn: func(s *store.Store) {
				_ = s.Nodes().AddPackage(&node.Package{
					Name: "users", Path: "example.com/users",
					Structs: []*node.Struct{
						{Name: "Article", Package: "example.com/users"},
						{Name: "Other", Package: "example.com/users"},
					},
				})
			},
		}
		gen := &recGen{
			name: "rec",
			generate: func(ctx *plugin.GeneratorContext) {
				seenStructs = len(ctx.Reader.Structs().Slice())
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithGenerator(gen).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithTargetSymbol("Article").
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if seenStructs != 1 {
			t.Fatalf("scoped generator should see 1 struct (Article only); got %d", seenStructs)
		}
	})
}
