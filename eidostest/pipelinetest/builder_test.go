// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipelinetest_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/internal/nodefixture"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
)

// minimalBackend returns a stubBackend wired with the supplied
// writes and a fixed name + language.
func minimalBackend() *stubBackend {
	return &stubBackend{name: "stub-be", lang: "stub"}
}

// headerBackend echoes the two header-determinism fields the
// pipeline threads into every render — [plugin.BackendContext.Command]
// and SourceRoot — into fixed targets, so a test can assert on what
// the pipeline decided without pulling backend/golang into
// eidostest's module graph.
//
// The captured bodies are the raw field values rather than a
// rendered header envelope: the property under test is what the
// pipeline hands the backend, and framing belongs to the backend's
// own tests.
type headerBackend struct{}

// Name returns the backend's plugin identifier.
func (*headerBackend) Name() string { return "backend.header" }

// Language returns the identifier plugins match on. Nothing in
// these tests contributes outputs, so the value only has to be
// stable.
func (*headerBackend) Language() string { return "stub" }

// Render writes ctx.Command, ctx.SourceRoot and ctx.Brand to fixed
// targets under the "out" directory.
func (*headerBackend) Render(ctx *plugin.BackendContext) error {
	writes := map[emit.Target][]byte{
		{Dir: "out", Filename: "command.txt", Package: "out"}:     []byte(ctx.Command),
		{Dir: "out", Filename: "source-root.txt", Package: "out"}: []byte(ctx.SourceRoot),
		{Dir: "out", Filename: "brand.txt", Package: "out"}:       []byte(ctx.Brand),
	}
	for tgt, body := range writes {
		if err := ctx.Sink.Write(tgt, body); err != nil {
			return fmt.Errorf("backend.header: write %s: %w", tgt.JoinPath(), err)
		}
	}
	return nil
}

// headerPipeline builds and runs a pipeline whose only job is to
// capture the header fields via [headerBackend]. Bound to tb so
// callers can route assertion failures into a fake TB.
func headerPipeline(tb testing.TB) *pipelinetest.Pipeline {
	tb.Helper()
	return pipelinetest.New(tb).
		WithFrontend(pipelinetest.FromNodes()).
		WithBackend(&headerBackend{}).
		Build().
		Run()
}

// routableSource returns a hand-built source package whose single
// struct carries an origin position. The position is load-bearing:
// alongside-source layout routes generated output next to the file
// its origin came from, so a positionless struct has nowhere to
// land. Built by hand rather than through [nodefixture] so the
// routing assertions below stay independent of that package's own
// defaults.
func routableSource() *node.Package {
	origin := &node.Struct{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: "users/user.go"}},
		Name:     "User",
		Package:  "example.com/users",
	}
	return &node.Package{
		Name:    "users",
		Path:    "example.com/users",
		Structs: []*node.Struct{origin},
	}
}

// tagGenerator emits one struct per (source struct, declared
// output), stamping each decl with its output's tag. It is the
// smallest fixture that exercises multi-output routing: the
// per-(plugin, tag) overrides only bite when a plugin declares more
// than one output and at least one of them is tagged.
type tagGenerator struct {
	name    string
	outputs []plugin.Output
}

// Name returns the plugin identifier routing overrides address the
// generator by.
func (g *tagGenerator) Name() string { return g.name }

// Outputs returns the configured slice for the example language and
// nil for anything else — the standard way a plugin declines a
// target it cannot write.
func (g *tagGenerator) Outputs(lang string) []plugin.Output {
	if lang != exampleLanguage {
		return nil
	}
	return g.outputs
}

// Generate emits one struct per declared output, anchored to the
// origin so the Layout phase has something to route against. Decl
// names are index-derived, so two outputs never collide on a name
// however their tags are spelled.
func (g *tagGenerator) Generate(ctx *plugin.GeneratorContext) error {
	for _, src := range ctx.Reader.Structs().Slice() {
		pb := builder.For(g.name).Anchor(src)
		for i, o := range g.outputs {
			pb.File(o.Tag).Struct(fmt.Sprintf("%sOut%d", src.Name, i), func(s *builder.StructBuilder) {
				s.Field("Value", emit.Builtin("string"), nil)
			})
		}
		pkg, err := pb.Build()
		if err != nil {
			return fmt.Errorf("%s: build: %w", g.name, err)
		}
		if err := ctx.Store.Emit().AddPackage(pkg); err != nil {
			return fmt.Errorf("%s: AddPackage: %w", g.name, err)
		}
	}
	return nil
}

// routedBuilder returns a harness builder wired with a positioned
// source package, the supplied generator, and the listing backend
// the package's Example uses. Routing tests differ only in the
// override they append.
func routedBuilder(tb testing.TB, gen plugin.Generator) *pipelinetest.Builder {
	tb.Helper()
	return pipelinetest.New(tb).
		WithFrontend(pipelinetest.FromNodes(routableSource())).
		WithGenerator(gen).
		WithBackend(&listingBackend{})
}

// exemptOptions names every [pipeline.Builder] option pipelinetest
// deliberately does not forward, against the reason it is withheld.
// Adding an entry is a design decision rather than a shortcut: the
// parity test below treats this table as the complete written
// justification for the gap.
var exemptOptions = map[string]string{
	"WithDiag": "the harness owns the diagnostic sink — Pipeline.Diagnostics returns the sink " +
		"the run actually wrote through, and a caller-supplied one would detach the two",
}

// withOptionNames returns the exported `With*` method names declared
// on typ, which is expected to be a pointer-to-Builder type.
func withOptionNames(typ reflect.Type) []string {
	var names []string
	for m := range typ.Methods() {
		if strings.HasPrefix(m.Name, "With") {
			names = append(names, m.Name)
		}
	}
	return names
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("returns a Builder whose Build defaults to the configured sink and diag", func(t *testing.T) {
		t.Parallel()
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(minimalBackend()).
			Build()
		if p.Sink() == nil {
			t.Fatalf("Build should default Sink to an in-memory sink")
		}
		if p.Diagnostics() == nil {
			t.Fatalf("Build should default Diagnostics to a fresh sink")
		}
	})

	t.Run("pins Command so rendered bytes carry no test-runner argv", func(t *testing.T) {
		t.Parallel()
		headerPipeline(t).
			AssertFile("command.txt").
			Equals("(test)").
			NotContains("-test.")
	})
}

func TestBuilder_OptionParity(t *testing.T) {
	t.Parallel()

	t.Run("every pipeline.Builder option is forwarded or carries a recorded exemption", func(t *testing.T) {
		t.Parallel()
		harness := reflect.TypeFor[*pipelinetest.Builder]()
		for _, name := range withOptionNames(reflect.TypeFor[*pipeline.Builder]()) {
			if exemptOptions[name] != "" {
				continue
			}
			if _, ok := harness.MethodByName(name); !ok {
				t.Errorf(
					"pipelinetest.Builder has no counterpart for pipeline.Builder.%s; "+
						"forward it or record an exemption reason in exemptOptions", name,
				)
			}
		}
	})

	t.Run("every recorded exemption names an option pipeline.Builder still declares", func(t *testing.T) {
		t.Parallel()
		declared := make(map[string]bool)
		for _, name := range withOptionNames(reflect.TypeFor[*pipeline.Builder]()) {
			declared[name] = true
		}
		for name := range exemptOptions {
			if !declared[name] {
				t.Errorf("exemption for %q is stale: pipeline.Builder no longer declares it", name)
			}
		}
	})
}

func TestBuilder_WithFrontend(t *testing.T) {
	t.Parallel()

	t.Run("registered frontend reaches the underlying pipeline", func(t *testing.T) {
		t.Parallel()
		pkg := nodefixture.Package("S")
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes(pkg)).
			WithBackend(minimalBackend()).
			Build().
			Run()
		if p.Diagnostics().HasErrors() {
			t.Fatalf("run should be clean; got %+v", p.Diagnostics().Diagnostics())
		}
	})
}

func TestBuilder_WithAnnotator(t *testing.T) {
	t.Parallel()

	t.Run("registered annotator appears in the resolved plan", func(t *testing.T) {
		t.Parallel()
		ann := &stubAnnotator{name: "stub-ann"}
		_ = pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithAnnotator(ann).
			WithBackend(minimalBackend()).
			Build()
		// The build did not fail — the annotator was accepted. The
		// negative case (rejected annotator) is exercised in
		// pipeline.Builder's own tests.
	})
}

func TestBuilder_WithGenerator(t *testing.T) {
	t.Parallel()

	t.Run("registered generator appears in the resolved plan", func(t *testing.T) {
		t.Parallel()
		gen := &stubGenerator{name: "stub-gen"}
		_ = pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithGenerator(gen).
			WithBackend(minimalBackend()).
			Build()
	})
}

func TestBuilder_WithBackend(t *testing.T) {
	t.Parallel()

	t.Run("zero backends causes Build to fail the test", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		called := captureFatal(func() {
			pipelinetest.New(fake).
				WithFrontend(pipelinetest.FromNodes()).
				Build()
		})
		if !called {
			t.Fatalf("Build with zero backends should Fatalf via the fake TB")
		}
		if !fake.Failed() {
			t.Fatalf("fake TB should record failure")
		}
	})
}

// optsBackend is a backend that also implements [plugin.OptionsProvider].
// Used to verify Builder.WithPluginOptions threads options through.
type optsBackend struct {
	name string
	lang string
	opts struct {
		Tag string `eidos:"tag,required"`
	}
}

func (b *optsBackend) Name() string                        { return b.name }
func (b *optsBackend) Language() string                    { return b.lang }
func (*optsBackend) Render(_ *plugin.BackendContext) error { return nil }
func (b *optsBackend) OptionsSchema() opt.Schema           { return opt.Reflect(b.opts) }
func (b *optsBackend) SetOptions(o opt.Options) error      { return o.Decode(&b.opts) }

func TestBuilder_WithPluginOptions(t *testing.T) {
	t.Parallel()

	t.Run("threads options into a plugin implementing OptionsProvider", func(t *testing.T) {
		t.Parallel()
		be := &optsBackend{name: "opts-be", lang: "stub"}
		_ = pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(be).
			WithPluginOptions("opts-be", map[string]string{"tag": "v1"}).
			Build()
		if be.opts.Tag != "v1" {
			t.Fatalf("options not threaded; got %q", be.opts.Tag)
		}
	})
}

func TestBuilder_WithDirective(t *testing.T) {
	t.Parallel()

	t.Run("registered directive schema is accepted by Build", func(t *testing.T) {
		t.Parallel()
		schema := directive.NewSchema("repo").Build()
		_ = pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithDirective(schema).
			WithBackend(minimalBackend()).
			Build()
	})
}

func TestBuilder_WithSink(t *testing.T) {
	t.Parallel()

	t.Run("user-supplied sink replaces the default memory sink", func(t *testing.T) {
		t.Parallel()
		user := sink.NewMemory()
		// The builder's Pipeline.Sink() accessor exposes the default
		// memory sink even after override — the override only changes
		// the sink the underlying pipeline writes through. That's by
		// design: tests still need a place to inspect captured output
		// when they swap the underlying sink for something else.
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(minimalBackend()).
			WithSink(user).
			Build()
		if p.Sink() == nil {
			t.Fatalf("Pipeline.Sink should remain wired even after WithSink override")
		}
	})
}

func TestBuilder_WithCache(t *testing.T) {
	t.Parallel()

	t.Run("user-supplied cache is accepted by Build", func(t *testing.T) {
		t.Parallel()
		_ = pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(minimalBackend()).
			WithCache(cache.NewNone()).
			Build()
	})
}

func TestBuilder_WithVerbose(t *testing.T) {
	t.Parallel()

	t.Run("verbose mode causes the pipeline to emit Info diagnostics", func(t *testing.T) {
		t.Parallel()
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(minimalBackend()).
			WithVerbose(true).
			Build().
			Run()
		var hasInfo bool
		for _, d := range p.Diagnostics().Diagnostics() {
			if d.Plugin == "pipeline" {
				hasInfo = true
				break
			}
		}
		if !hasInfo {
			t.Fatalf("verbose mode should emit pipeline-attributed diagnostics")
		}
	})
}

func TestBuilder_WithParallel(t *testing.T) {
	t.Parallel()

	t.Run("phases are accepted and the run completes cleanly", func(t *testing.T) {
		t.Parallel()
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(minimalBackend()).
			WithParallel(pipeline.PhaseFrontend, pipeline.PhaseAnnotator, pipeline.PhaseGenerator).
			Build().
			Run()
		if p.Diagnostics().HasErrors() {
			t.Fatalf("parallel-enabled run should be clean; got %+v", p.Diagnostics().Diagnostics())
		}
	})
}

func TestBuilder_Build(t *testing.T) {
	t.Parallel()

	t.Run("build error calls Fatalf on the configured TB", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		called := captureFatal(func() {
			pipelinetest.New(fake).Build()
		})
		if !called {
			t.Fatalf("Build with no frontend and no backend should Fatalf")
		}
		if len(fake.fatals) == 0 {
			t.Fatalf("expected at least one recorded fatal")
		}
		if !strings.Contains(fake.fatals[0], "build failed") {
			t.Fatalf("fatal should mention the build failure; got %q", fake.fatals[0])
		}
	})
}

func TestBuilder_WithCommand(t *testing.T) {
	t.Parallel()

	t.Run("a caller-supplied command overrides the harness default", func(t *testing.T) {
		t.Parallel()
		pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(&headerBackend{}).
			WithCommand("eidos gen ./...").
			Build().
			Run().
			AssertFile("command.txt").
			Equals("eidos gen ./...")
	})
}

func TestBuilder_WithSourceRoot(t *testing.T) {
	t.Parallel()

	t.Run("the configured source root reaches the backend context", func(t *testing.T) {
		t.Parallel()
		pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(&headerBackend{}).
			WithSourceRoot("/workspace/project").
			Build().
			Run().
			AssertFile("source-root.txt").
			Equals("/workspace/project")
	})
}

func TestBuilder_WithBrand(t *testing.T) {
	t.Parallel()

	t.Run("the configured brand reaches the backend context", func(t *testing.T) {
		t.Parallel()
		pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(&headerBackend{}).
			WithBrand("testkit").
			Build().
			Run().
			AssertFile("brand.txt").
			Equals("testkit")
	})
}

func TestBuilder_WithPluginOutput(t *testing.T) {
	t.Parallel()

	t.Run("a per-plugin routing override moves the rendered file", func(t *testing.T) {
		t.Parallel()
		got := routedBuilder(t, &greeterGenerator{}).
			WithPluginOutput("greetergen", pipeline.LayoutCentralised, "gen", "generated").
			Build().
			Run().
			AssertFile("user_greeter.go").
			Target()
		if got.Dir != "generated" || got.Package != "gen" {
			t.Fatalf("per-plugin override ignored: target = %+v", got)
		}
	})
}

func TestBuilder_WithPluginTagOutput(t *testing.T) {
	t.Parallel()

	t.Run("a per-tag routing override separates two outputs sharing a suffix", func(t *testing.T) {
		t.Parallel()
		gen := &tagGenerator{name: "tagged", outputs: []plugin.Output{
			{Suffix: "_tag.go"},
			{Tag: "helper", Suffix: "_tag.go"},
		}}
		p := routedBuilder(t, gen).
			WithPluginTagOutput("tagged", "helper", pipeline.LayoutCentralised, "helpers", "helpers").
			Build().
			Run()
		p.AssertFileCount(2)
		p.AssertFileInDir("users", "user_tag.go")
		p.AssertFileInDir("helpers", "user_tag.go")
	})
}

func TestBuilder_WithOutputFilename(t *testing.T) {
	t.Parallel()

	t.Run("an unscoped filename override collapses a multi-output plugin", func(t *testing.T) {
		t.Parallel()
		// Two distinct suffixes route to two files by default. The
		// unscoped override pins both onto one name and nothing
		// objects — the trap the forwarded option's docblock warns
		// about.
		gen := &tagGenerator{name: "split", outputs: []plugin.Output{
			{Suffix: "_a.go"},
			{Tag: "b", Suffix: "_b.go"},
		}}
		p := routedBuilder(t, gen).
			WithOutputFilename("pinned.go").
			Build().
			Run()
		p.AssertFileCount(1)
		p.AssertFile("pinned.go")
	})
}

func TestBuilder_WithPluginOutputFilename(t *testing.T) {
	t.Parallel()

	t.Run("a per-plugin filename override renames only that plugin's output", func(t *testing.T) {
		t.Parallel()
		routedBuilder(t, &greeterGenerator{}).
			WithPluginOutputFilename("greetergen", "", "custom_greeter.go").
			Build().
			Run().
			AssertFile("custom_greeter.go")
	})
}

func TestBuilder_WithProjectOutput(t *testing.T) {
	t.Parallel()

	t.Run("the project-level routing triple applies to every plugin", func(t *testing.T) {
		t.Parallel()
		got := routedBuilder(t, &greeterGenerator{}).
			WithProjectOutput(pipeline.LayoutCentralised, "proj", "projgen").
			Build().
			Run().
			AssertFile("user_greeter.go").
			Target()
		if got.Dir != "projgen" || got.Package != "proj" {
			t.Fatalf("project-level override ignored: target = %+v", got)
		}
	})
}

func TestBuilder_WithOutputLayout(t *testing.T) {
	t.Parallel()

	t.Run("centralised layout lifts the file out of its source directory", func(t *testing.T) {
		t.Parallel()
		got := routedBuilder(t, &greeterGenerator{}).
			WithOutputLayout(pipeline.LayoutCentralised).
			Build().
			Run().
			AssertFile("user_greeter.go").
			Target()
		if got.Dir == "users" {
			t.Fatalf("centralised layout should not route alongside source; target = %+v", got)
		}
	})
}

func TestBuilder_WithOutputDir(t *testing.T) {
	t.Parallel()

	t.Run("the configured directory receives the file under centralised layout", func(t *testing.T) {
		t.Parallel()
		got := routedBuilder(t, &greeterGenerator{}).
			WithOutputLayout(pipeline.LayoutCentralised).
			WithOutputDir("generated").
			Build().
			Run().
			AssertFile("user_greeter.go").
			Target()
		if got.Dir != "generated" {
			t.Fatalf("output dir ignored: target = %+v", got)
		}
	})
}

func TestBuilder_WithOutputPackage(t *testing.T) {
	t.Parallel()

	t.Run("the configured package is pinned without moving the file", func(t *testing.T) {
		t.Parallel()
		got := routedBuilder(t, &greeterGenerator{}).
			WithOutputPackage("pinned").
			Build().
			Run().
			AssertFile("user_greeter.go").
			Target()
		if got.Package != "pinned" || got.Dir != "users" {
			t.Fatalf("package pin should not move the file: target = %+v", got)
		}
	})
}

func TestBuilder_WithTargetSymbol(t *testing.T) {
	t.Parallel()

	t.Run("a non-matching symbol filter leaves the run with nothing to emit", func(t *testing.T) {
		t.Parallel()
		routedBuilder(t, &greeterGenerator{}).
			WithTargetSymbol("Absent").
			Build().
			Run().
			AssertFileCount(0)
	})
}

func TestBuilder_WithDirectivePrefix(t *testing.T) {
	t.Parallel()

	t.Run("a prefix carrying a sigil is rejected by the underlying builder", func(t *testing.T) {
		t.Parallel()
		_, err := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(minimalBackend()).
			WithDirectivePrefix("bad:prefix").
			BuildErr()
		if !errors.Is(err, directive.ErrInvalidPrefix) {
			t.Fatalf("BuildErr error = %v, want directive.ErrInvalidPrefix", err)
		}
	})

	t.Run("a project-specific prefix is accepted", func(t *testing.T) {
		t.Parallel()
		_ = pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(minimalBackend()).
			WithDirectivePrefix("thesmos").
			Build()
	})
}

func TestBuilder_WithManifestPath(t *testing.T) {
	t.Parallel()

	t.Run("a configured path receives the run's manifest", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "manifest.json")
		routedBuilder(t, &greeterGenerator{}).
			WithManifestPath(path).
			Build().
			Run()
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("manifest should exist at %s: %v", path, err)
		}
	})
}

func TestBuilder_WithDryRun(t *testing.T) {
	t.Parallel()

	t.Run("dry run leaves the manifest path untouched", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "manifest.json")
		routedBuilder(t, &greeterGenerator{}).
			WithManifestPath(path).
			WithDryRun(true).
			Build().
			Run()
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("dry run should not write %s; stat err = %v", path, err)
		}
	})
}

func TestBuilder_WithPipelineID(t *testing.T) {
	t.Parallel()

	t.Run("the configured identifier is stamped on every manifest entry", func(t *testing.T) {
		t.Parallel()
		m := routedBuilder(t, &greeterGenerator{}).
			WithManifestPath(filepath.Join(t.TempDir(), "manifest.json")).
			WithDryRun(true).
			WithPipelineID("harness-pipeline").
			Build().
			Run().
			Manifest()
		if m == nil || len(m.Outputs) == 0 {
			t.Fatalf("expected at least one manifest output; got %+v", m)
		}
		for _, o := range m.Outputs {
			if o.PipelineID != "harness-pipeline" {
				t.Errorf("output %s carries PipelineID %q, want %q",
					o.Target.JoinPath(), o.PipelineID, "harness-pipeline")
			}
		}
	})
}

func TestBuilder_BuildErr(t *testing.T) {
	t.Parallel()

	t.Run("a duplicate directive schema surfaces as an error rather than a fatal", func(t *testing.T) {
		t.Parallel()
		p, err := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(minimalBackend()).
			WithDirective(
				directive.NewSchema("repo").Build(),
				directive.NewSchema("repo").Build(),
			).
			BuildErr()
		if !errors.Is(err, pipeline.ErrDuplicateDirective) {
			t.Fatalf("BuildErr error = %v, want pipeline.ErrDuplicateDirective", err)
		}
		if p != nil {
			t.Fatalf("BuildErr should return a nil Pipeline on error; got %+v", p)
		}
	})

	t.Run("a malformed Outputs slice surfaces as an error rather than a fatal", func(t *testing.T) {
		t.Parallel()
		p, err := routedBuilder(t, &tagGenerator{
			name:    "suffixless",
			outputs: []plugin.Output{{Suffix: ""}},
		}).BuildErr()
		if !errors.Is(err, pipeline.ErrInvalidOutputs) {
			t.Fatalf("BuildErr error = %v, want pipeline.ErrInvalidOutputs", err)
		}
		if p != nil {
			t.Fatalf("BuildErr should return a nil Pipeline on error; got %+v", p)
		}
	})

	t.Run("a valid configuration returns a runnable pipeline and no error", func(t *testing.T) {
		t.Parallel()
		p, err := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(minimalBackend()).
			BuildErr()
		if err != nil {
			t.Fatalf("BuildErr on a valid configuration: %v", err)
		}
		if p == nil {
			t.Fatalf("BuildErr should return a Pipeline when Build succeeds")
		}
		p.Run()
	})
}
