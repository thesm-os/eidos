// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline_test

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
	"text/template"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/manifest"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
)

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("returns a non-nil Builder", func(t *testing.T) {
		t.Parallel()
		if pipeline.New() == nil {
			t.Fatalf("New should return a non-nil Builder")
		}
	})
}

func TestBuilder_With(t *testing.T) {
	t.Parallel()

	t.Run("With* methods return the receiver for chaining", func(t *testing.T) {
		t.Parallel()
		b := pipeline.New()
		out := b.WithFrontend(&stubFE{name: "fe"}).
			WithAnnotator(&stubAnn{name: "ann"}).
			WithGenerator(&stubGen{name: "gen"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithCache(cache.NewNone()).
			WithDiag(diag.New()).
			WithVerbose(true).
			WithBrand("testkit").
			WithPluginOptions("p", map[string]string{"k": "v"})
		if out != b {
			t.Fatalf("With* should return the receiver")
		}
	})
}

// TestPipeline_PipelineID pins the identifier the manifest stamps on
// every Output. The scope-aware merge and the prune subcommand both
// key on it to stay inside one pipeline's entries in a
// multi-pipeline workdir, so an empty or unstable value lets one
// pipeline delete another's files.
func TestPipeline_PipelineID(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, id string) *pipeline.Pipeline {
		t.Helper()
		b := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory())
		if id != "" {
			b = b.WithPipelineID(id)
		}
		p, err := b.Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		return p
	}

	t.Run("returns the explicitly configured id", func(t *testing.T) {
		t.Parallel()
		if got := build(t, "pid-explicit").PipelineID(); got != "pid-explicit" {
			t.Fatalf("PipelineID = %q, want pid-explicit", got)
		}
	})

	t.Run("derives a non-empty id when none is configured", func(t *testing.T) {
		t.Parallel()
		if got := build(t, "").PipelineID(); got == "" {
			t.Fatalf("PipelineID is empty; the manifest cannot scope its entries")
		}
	})

	t.Run("derives the same id for the same plugin set", func(t *testing.T) {
		t.Parallel()
		if a, b := build(t, "").PipelineID(), build(t, "").PipelineID(); a != b {
			t.Fatalf("derived PipelineID is unstable: %q vs %q", a, b)
		}
	})
}

func TestBuilder_Build(t *testing.T) {
	t.Parallel()

	t.Run("succeeds with one frontend, one backend, and no options", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		assertNoError(t, err)
		if p == nil {
			t.Fatalf("Build should return a non-nil Pipeline on success")
		}
	})

	t.Run("populates default cache and diag when not configured", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		assertNoError(t, err)
		if p.Cache() == nil {
			t.Fatalf("Build should default Cache when not configured")
		}
		if p.Diag() == nil {
			t.Fatalf("Build should default Diag when not configured")
		}
	})

	t.Run("rejects duplicate plugin names with ErrDuplicatePlugin", func(t *testing.T) {
		t.Parallel()
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "shared"}).
			WithAnnotator(&stubAnn{name: "shared"}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		if !errors.Is(err, pipeline.ErrDuplicatePlugin) {
			t.Fatalf("Build should return ErrDuplicatePlugin; got %v", err)
		}
	})

	t.Run("rejects zero frontends with ErrNoFrontend", func(t *testing.T) {
		t.Parallel()
		_, err := pipeline.New().WithBackend(&stubBE{name: "be"}).Build()
		if !errors.Is(err, pipeline.ErrNoFrontend) {
			t.Fatalf("Build should return ErrNoFrontend; got %v", err)
		}
	})

	t.Run("rejects zero backends with ErrNoBackend", func(t *testing.T) {
		t.Parallel()
		_, err := pipeline.New().WithFrontend(&stubFE{name: "fe"}).Build()
		if !errors.Is(err, pipeline.ErrNoBackend) {
			t.Fatalf("Build should return ErrNoBackend; got %v", err)
		}
	})

	t.Run("rejects multiple backends with ErrMultipleBackends", func(t *testing.T) {
		t.Parallel()
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be1"}).
			WithBackend(&stubBE{name: "be2"}).
			Build()
		if !errors.Is(err, pipeline.ErrMultipleBackends) {
			t.Fatalf("Build should return ErrMultipleBackends; got %v", err)
		}
	})

	t.Run("calls SetOptions on plugins implementing OptionsProvider", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFEWithOpts{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithPluginOptions("fe", map[string]string{"output": "internal/users"}).
			Build()
		assertNoError(t, err)
		if p == nil {
			t.Fatalf("Build should succeed when options are valid")
		}
	})

	t.Run("returns ErrInvalidOptions when SetOptions fails", func(t *testing.T) {
		t.Parallel()
		_, err := pipeline.New().
			WithFrontend(&stubFEWithOpts{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			// "output" is required; supplying nothing triggers
			// ErrMissingRequired inside Decode.
			Build()
		if !errors.Is(err, pipeline.ErrInvalidOptions) {
			t.Fatalf("Build should return ErrInvalidOptions; got %v", err)
		}
	})

	t.Run("writes one diagnostic per validation error", func(t *testing.T) {
		t.Parallel()
		d := diag.New()
		_, _ = pipeline.New().
			WithFrontend(&stubFE{name: "shared"}).
			WithAnnotator(&stubAnn{name: "shared"}).
			WithDiag(d).
			Build()
		// Expected errors: duplicate name + no backend = 2.
		if d.Count(diag.Error) < 2 {
			t.Fatalf("Build should write per-error diagnostics; got %d", d.Count(diag.Error))
		}
	})

	t.Run("aggregates multiple errors via errors.Join", func(t *testing.T) {
		t.Parallel()
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "shared"}).
			WithAnnotator(&stubAnn{name: "shared"}).
			Build()
		if !errors.Is(err, pipeline.ErrDuplicatePlugin) {
			t.Fatalf("aggregate should match ErrDuplicatePlugin; got %v", err)
		}
		if !errors.Is(err, pipeline.ErrNoBackend) {
			t.Fatalf("aggregate should match ErrNoBackend; got %v", err)
		}
	})

	t.Run("ignores empty plugin names when checking duplicates", func(t *testing.T) {
		t.Parallel()
		// Two plugins reporting the empty string are not considered
		// duplicates — the empty name signals an unnamed stub which
		// the pipeline tolerates here (later milestones may surface
		// it as its own diagnostic).
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: ""}).
			WithAnnotator(&stubAnn{name: ""}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		if errors.Is(err, pipeline.ErrDuplicatePlugin) {
			t.Fatalf("empty names must not collide as duplicates; got %v", err)
		}
	})
}

func TestBuilder_WithDirective(t *testing.T) {
	t.Parallel()

	t.Run("registers schemas on the pipeline's directive.Registry", func(t *testing.T) {
		t.Parallel()
		repo := directive.NewSchema("repo").Build()
		mock := directive.NewSchema("mock").Build()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithDirective(repo, mock).
			Build()
		assertNoError(t, err)
		reg := p.DirectiveRegistry()
		if _, ok := reg.Lookup("repo"); !ok {
			t.Fatalf("registry should contain 'repo'")
		}
		if _, ok := reg.Lookup("mock"); !ok {
			t.Fatalf("registry should contain 'mock'")
		}
	})

	t.Run("variadic and repeated calls accumulate", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithDirective(directive.NewSchema("a").Build()).
			WithDirective(directive.NewSchema("b").Build(), directive.NewSchema("c").Build()).
			Build()
		assertNoError(t, err)
		got := p.DirectiveRegistry().Names()
		// The pipeline always registers its core directives ("out"
		// for the Router phase, "value" for per-source string-form
		// overrides any plugin can consume) ahead of user-supplied
		// schemas, so the expected count is the user schemas plus
		// the core set.
		want := []string{"a", "b", "c", "out", "value"}
		if len(got) != len(want) {
			t.Fatalf("registered names: got %v, want %v", got, want)
		}
		set := make(map[string]bool, len(got))
		for _, n := range got {
			set[string(n)] = true
		}
		for _, n := range want {
			if !set[n] {
				t.Fatalf("expected registry to contain %q; got %v", n, got)
			}
		}
	})

	t.Run("duplicate schemas return ErrDuplicateDirective", func(t *testing.T) {
		t.Parallel()
		schema := directive.NewSchema("dup").Build()
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithDirective(schema, schema).
			Build()
		if !errors.Is(err, pipeline.ErrDuplicateDirective) {
			t.Fatalf("Build should return ErrDuplicateDirective; got %v", err)
		}
	})

	t.Run("auto-collects schemas from plugins implementing DirectiveProvider", func(t *testing.T) {
		t.Parallel()
		provider := &stubFEWithDirectives{
			stubFE:  stubFE{name: "fe"},
			schemas: []directive.Schema{directive.NewSchema("auto").Build()},
		}
		p, err := pipeline.New().
			WithFrontend(provider).
			WithBackend(&stubBE{name: "be"}).
			Build()
		assertNoError(t, err)
		if _, ok := p.DirectiveRegistry().Lookup("auto"); !ok {
			t.Fatalf("registry should contain auto-collected 'auto' directive")
		}
	})

	t.Run("DirectiveProvider auto-collected duplicate fails Build", func(t *testing.T) {
		t.Parallel()
		schema := directive.NewSchema("collide").Build()
		provider := &stubFEWithDirectives{
			stubFE:  stubFE{name: "fe"},
			schemas: []directive.Schema{schema},
		}
		_, err := pipeline.New().
			WithFrontend(provider).
			WithBackend(&stubBE{name: "be"}).
			WithDirective(schema).
			Build()
		if !errors.Is(err, pipeline.ErrDuplicateDirective) {
			t.Fatalf("Build should return ErrDuplicateDirective for plugin/manual collision; got %v", err)
		}
	})
}

// stubFEWithDirectives extends stubFE with the DirectiveProvider
// surface so the auto-collection path can be exercised through
// the public Build flow.
type stubFEWithDirectives struct {
	stubFE
	schemas []directive.Schema
}

func (s *stubFEWithDirectives) Directives() []directive.Schema { return s.schemas }

// TestBuilder_Build_OutputsValidation pins the four Outputs-shape
// rules the framework enforces at Build time for every
// [plugin.FilenameProvider]: every Suffix is non-empty; tags
// within one slice are unique; at most one Output declares an
// empty Tag; the empty-Tag Output is at index 0 when present.
// Each failure surfaces [pipeline.ErrInvalidOutputs].
func TestBuilder_Build_OutputsValidation(t *testing.T) {
	t.Parallel()

	build := func(outputs []plugin.Output) error {
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&outputsGen{name: "gen", outputs: outputs}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		return err
	}

	t.Run("rejects an output with empty Suffix", func(t *testing.T) {
		t.Parallel()
		err := build([]plugin.Output{{Suffix: ""}})
		if !errors.Is(err, pipeline.ErrInvalidOutputs) {
			t.Fatalf("Build should return ErrInvalidOutputs for empty Suffix; got %v", err)
		}
	})

	t.Run("rejects duplicate Tag values", func(t *testing.T) {
		t.Parallel()
		err := build([]plugin.Output{
			{Suffix: "_x.go"},
			{Tag: "test", Suffix: "_x_test.go"},
			{Tag: "test", Suffix: "_y_test.go"},
		})
		if !errors.Is(err, pipeline.ErrInvalidOutputs) {
			t.Fatalf("Build should return ErrInvalidOutputs for duplicate Tag; got %v", err)
		}
	})

	t.Run("rejects more than one output with empty Tag", func(t *testing.T) {
		t.Parallel()
		err := build([]plugin.Output{
			{Suffix: "_x.go"},
			{Suffix: "_y.go"},
		})
		if !errors.Is(err, pipeline.ErrInvalidOutputs) {
			t.Fatalf("Build should return ErrInvalidOutputs for two empty-Tag outputs; got %v", err)
		}
	})

	t.Run("rejects empty-Tag output not at index 0", func(t *testing.T) {
		t.Parallel()
		err := build([]plugin.Output{
			{Tag: "test", Suffix: "_x_test.go"},
			{Suffix: "_x.go"},
		})
		if !errors.Is(err, pipeline.ErrInvalidOutputs) {
			t.Fatalf("Build should return ErrInvalidOutputs for misplaced empty-Tag output; got %v", err)
		}
	})

	t.Run("accepts a well-formed single-Output slice", func(t *testing.T) {
		t.Parallel()
		if err := build([]plugin.Output{{Suffix: "_x.go"}}); err != nil {
			t.Fatalf("Build rejected a well-formed Outputs slice: %v", err)
		}
	})

	t.Run("accepts a well-formed multi-Output slice", func(t *testing.T) {
		t.Parallel()
		err := build([]plugin.Output{
			{Suffix: "_x.go"},
			{Tag: "test", Suffix: "_x_test.go"},
		})
		if err != nil {
			t.Fatalf("Build rejected a well-formed multi-Output slice: %v", err)
		}
	})

	t.Run("accepts every-output-tagged (no empty-tag primary)", func(t *testing.T) {
		t.Parallel()
		err := build([]plugin.Output{
			{Tag: "production", Suffix: "_x.go"},
			{Tag: "test", Suffix: "_x_test.go"},
		})
		if err != nil {
			t.Fatalf("Build rejected an all-tagged Outputs slice: %v", err)
		}
	})
}

func TestBuilder_Build_EmitVersionCompatibility(t *testing.T) {
	t.Parallel()

	t.Run("rejects a plugin whose declared majors omit the current emit major", func(t *testing.T) {
		t.Parallel()
		_, err := pipeline.New().
			WithFrontend(&emitVersionedFE{name: "fe", versions: []string{"99"}}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		if !errors.Is(err, pipeline.ErrIncompatibleEmitVersion) {
			t.Fatalf("Build should return ErrIncompatibleEmitVersion; got %v", err)
		}
	})

	t.Run("accepts a plugin whose declared majors include the current emit major", func(t *testing.T) {
		t.Parallel()
		_, err := pipeline.New().
			WithFrontend(&emitVersionedFE{name: "fe", versions: []string{"1"}}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		assertNoError(t, err)
	})

	t.Run("plugins not implementing EmitVersioned are assumed compatible", func(t *testing.T) {
		t.Parallel()
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		assertNoError(t, err)
	})
}

func TestBuilder_WithDirectivePrefix(t *testing.T) {
	t.Parallel()

	t.Run("returns the receiver for chaining", func(t *testing.T) {
		t.Parallel()
		b := pipeline.New()
		if out := b.WithDirectivePrefix("myapp"); out != b {
			t.Fatalf("WithDirectivePrefix should return the receiver")
		}
	})

	t.Run("accepts a valid custom prefix", func(t *testing.T) {
		t.Parallel()
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithDirectivePrefix("myapp").
			Build()
		assertNoError(t, err)
	})

	t.Run("an invalid prefix returns ErrInvalidDirectivePrefix wrapping ErrInvalidPrefix", func(t *testing.T) {
		t.Parallel()
		// "+bad" contains a reserved sigil; directive.NewParser
		// rejects it with directive.ErrInvalidPrefix, which Build
		// surfaces as ErrInvalidDirectivePrefix.
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithDirectivePrefix("+bad").
			Build()
		if !errors.Is(err, pipeline.ErrInvalidDirectivePrefix) {
			t.Fatalf("Build err = %v, want ErrInvalidDirectivePrefix", err)
		}
		if !errors.Is(err, directive.ErrInvalidPrefix) {
			t.Fatalf("Build err = %v, want underlying directive.ErrInvalidPrefix", err)
		}
	})
}

func TestBuilder_WithParallel(t *testing.T) {
	t.Parallel()

	t.Run("returns the receiver for chaining", func(t *testing.T) {
		t.Parallel()
		b := pipeline.New()
		if out := b.WithParallel(pipeline.PhaseFrontend, pipeline.PhaseAnnotator); out != b {
			t.Fatalf("WithParallel should return the receiver")
		}
	})
}

func TestBuilder_WithManifestPath(t *testing.T) {
	t.Parallel()

	t.Run("returns the receiver for chaining", func(t *testing.T) {
		t.Parallel()
		b := pipeline.New()
		if out := b.WithManifestPath("/tmp/manifest.json"); out != b {
			t.Fatalf("WithManifestPath should return the receiver")
		}
	})
}

// TestBuilder_WithCommand verifies the receiver chaining contract;
// behavioural coverage (the value reaches the BackendContext) is
// pinned in TestPipeline_Run_CommandOverride.
func TestBuilder_WithCommand(t *testing.T) {
	t.Parallel()

	t.Run("returns the receiver for chaining", func(t *testing.T) {
		t.Parallel()
		b := pipeline.New()
		if out := b.WithCommand("(library)"); out != b {
			t.Fatalf("WithCommand should return the receiver")
		}
	})
}

// TestBuilder_WithSourceRoot verifies the receiver chaining
// contract; behavioural coverage (the value reaches the
// BackendContext) is pinned in TestPipeline_Run_SourceRootOverride.
func TestBuilder_WithSourceRoot(t *testing.T) {
	t.Parallel()

	t.Run("returns the receiver for chaining", func(t *testing.T) {
		t.Parallel()
		b := pipeline.New()
		if out := b.WithSourceRoot("/home/dev/proj"); out != b {
			t.Fatalf("WithSourceRoot should return the receiver")
		}
	})
}

// TestBuilder_WithRoutingOverrides covers receiver chaining for
// every routing-override With* method. Behavioural coverage (each
// value reaches the constructed Pipeline) lives in
// TestPipeline_RoutingPolicy.
func TestBuilder_WithRoutingOverrides(t *testing.T) {
	t.Parallel()

	type setFunc func(*pipeline.Builder) *pipeline.Builder
	cases := []struct {
		name string
		set  setFunc
	}{
		{"WithOutputFilename", func(b *pipeline.Builder) *pipeline.Builder { return b.WithOutputFilename("gen.go") }},
		{"WithOutputPackage", func(b *pipeline.Builder) *pipeline.Builder { return b.WithOutputPackage("gen") }},
		{"WithOutputLayout", func(b *pipeline.Builder) *pipeline.Builder {
			return b.WithOutputLayout(pipeline.LayoutCentralised)
		}},
		{"WithOutputDir", func(b *pipeline.Builder) *pipeline.Builder { return b.WithOutputDir("internal/gen") }},
		{"WithTargetSymbol", func(b *pipeline.Builder) *pipeline.Builder { return b.WithTargetSymbol("Article") }},
	}
	for _, tc := range cases {
		t.Run(tc.name+" returns the receiver", func(t *testing.T) {
			t.Parallel()
			b := pipeline.New()
			if got := tc.set(b); got != b {
				t.Fatalf("%s should return the receiver", tc.name)
			}
		})
	}
}

// TestBuilder_WithPluginTagOutput pins the per-(plugin, tag) routing
// block — `plugins[*].output.tags.<tag>.*` in `.eidos.yaml`, layer 3
// of the six-layer precedence merge, and its most specific
// refinement. Nothing in the workspace called
// [pipeline.Builder.WithPluginTagOutput] from a test, so
// `applyPerTagOverride` never got past its "no override registered"
// early return and every branch inside it survived mutation testing:
//
//   - negating the lazy map init in WithPluginTagOutput assigns into
//     a nil map, so any `.eidos.yaml` carrying a tags block crashes
//     the CLI outright;
//   - negating an `over.<field> != ""` guard drops the configured
//     value AND writes the empty string over the inherited one — an
//     emptied Layout reaches composeTarget's unreachable-default arm
//     and drops every decl for the tag;
//   - negating a `b.output<Field> != ""` guard inverts the documented
//     "CLI overrides win over per-tag" ordering, so a `-p` / `-layout`
//     / output-dir flag loses to the config file it is meant to
//     override.
//
// Each case therefore asserts the whole resolved [pipeline.LayoutPolicy],
// value and attribution together. Asserting the value alone would let
// the ordering mutations live on: they resolve a plausible value while
// stamping the wrong [manifest.Layer] into the manifest's
// `resolved_from` block, which is the only record of why a file
// landed where it did.
func TestBuilder_WithPluginTagOutput(t *testing.T) {
	t.Parallel()

	const (
		plug = "enumgen"
		tag  = "test"
	)

	policyFor := func(t *testing.T, configure func(*pipeline.Builder) *pipeline.Builder) pipeline.LayoutPolicy {
		t.Helper()
		p, err := configure(pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&stubGen{name: plug}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory())).
			Build()
		assertNoError(t, err)
		return p.LayoutPolicyForTag(plug, tag)
	}

	cases := []struct {
		name      string
		configure func(*pipeline.Builder) *pipeline.Builder
		want      pipeline.LayoutPolicy
	}{
		{
			name: "the first tag block on a builder that has none is accepted",
			configure: func(b *pipeline.Builder) *pipeline.Builder {
				return b.WithPluginTagOutput(plug, tag, "", "enums", "")
			},
			want: pipeline.LayoutPolicy{
				Layout:      pipeline.LayoutAlongsideSource,
				LayoutFrom:  manifest.LayerFramework,
				Package:     "enums",
				PackageFrom: manifest.LayerPerPlugin,
				DirFrom:     manifest.LayerFramework,
			},
		},
		{
			name: "a tag layout overrides the layout the plugin inherited",
			configure: func(b *pipeline.Builder) *pipeline.Builder {
				return b.
					WithProjectOutput(pipeline.LayoutAlongsideSource, "", "").
					WithPluginTagOutput(plug, tag, pipeline.LayoutCentralised, "", "")
			},
			want: pipeline.LayoutPolicy{
				Layout:      pipeline.LayoutCentralised,
				LayoutFrom:  manifest.LayerPerPlugin,
				PackageFrom: manifest.LayerFramework,
				DirFrom:     manifest.LayerFramework,
			},
		},
		{
			name: "a tag block that names no layout leaves the inherited layout untouched",
			configure: func(b *pipeline.Builder) *pipeline.Builder {
				return b.
					WithProjectOutput(pipeline.LayoutCentralised, "", "").
					WithPluginTagOutput(plug, tag, "", "enums", "")
			},
			want: pipeline.LayoutPolicy{
				Layout:      pipeline.LayoutCentralised,
				LayoutFrom:  manifest.LayerProject,
				Package:     "enums",
				PackageFrom: manifest.LayerPerPlugin,
				DirFrom:     manifest.LayerFramework,
			},
		},
		{
			name: "a tag package overrides the package the plugin inherited",
			configure: func(b *pipeline.Builder) *pipeline.Builder {
				return b.
					WithPluginOutput(plug, "", "gen", "").
					WithPluginTagOutput(plug, tag, "", "enums", "")
			},
			want: pipeline.LayoutPolicy{
				Layout:      pipeline.LayoutAlongsideSource,
				LayoutFrom:  manifest.LayerFramework,
				Package:     "enums",
				PackageFrom: manifest.LayerPerPlugin,
				DirFrom:     manifest.LayerFramework,
			},
		},
		{
			name: "a tag block that names no package leaves the inherited package untouched",
			configure: func(b *pipeline.Builder) *pipeline.Builder {
				return b.
					WithPluginOutput(plug, "", "gen", "").
					WithPluginTagOutput(plug, tag, pipeline.LayoutCentralised, "", "")
			},
			want: pipeline.LayoutPolicy{
				Layout:      pipeline.LayoutCentralised,
				LayoutFrom:  manifest.LayerPerPlugin,
				Package:     "gen",
				PackageFrom: manifest.LayerPerPlugin,
				DirFrom:     manifest.LayerFramework,
			},
		},
		{
			name: "a tag dir overrides the dir the plugin inherited",
			configure: func(b *pipeline.Builder) *pipeline.Builder {
				return b.
					WithPluginOutput(plug, "", "", "internal/gen").
					WithPluginTagOutput(plug, tag, "", "", "internal/enums")
			},
			want: pipeline.LayoutPolicy{
				Layout:      pipeline.LayoutAlongsideSource,
				LayoutFrom:  manifest.LayerFramework,
				PackageFrom: manifest.LayerFramework,
				Dir:         "internal/enums",
				DirFrom:     manifest.LayerPerPlugin,
			},
		},
		{
			name: "a tag block that names no dir leaves the inherited dir untouched",
			configure: func(b *pipeline.Builder) *pipeline.Builder {
				return b.
					WithPluginOutput(plug, "", "", "internal/gen").
					WithPluginTagOutput(plug, tag, pipeline.LayoutCentralised, "", "")
			},
			want: pipeline.LayoutPolicy{
				Layout:      pipeline.LayoutCentralised,
				LayoutFrom:  manifest.LayerPerPlugin,
				PackageFrom: manifest.LayerFramework,
				Dir:         "internal/gen",
				DirFrom:     manifest.LayerPerPlugin,
			},
		},
		{
			name: "a layout on the command line beats the tag block",
			configure: func(b *pipeline.Builder) *pipeline.Builder {
				return b.
					WithPluginTagOutput(plug, tag, pipeline.LayoutCentralised, "", "").
					WithOutputLayout(pipeline.LayoutAlongsideSource)
			},
			want: pipeline.LayoutPolicy{
				Layout:      pipeline.LayoutAlongsideSource,
				LayoutFrom:  manifest.LayerCLI,
				PackageFrom: manifest.LayerFramework,
				DirFrom:     manifest.LayerFramework,
			},
		},
		{
			name: "a package on the command line beats the tag block",
			configure: func(b *pipeline.Builder) *pipeline.Builder {
				return b.
					WithPluginTagOutput(plug, tag, "", "enums", "").
					WithOutputPackage("cli-pkg")
			},
			want: pipeline.LayoutPolicy{
				Layout:      pipeline.LayoutAlongsideSource,
				LayoutFrom:  manifest.LayerFramework,
				Package:     "cli-pkg",
				PackageFrom: manifest.LayerCLI,
				DirFrom:     manifest.LayerFramework,
			},
		},
		{
			name: "a dir on the command line beats the tag block",
			configure: func(b *pipeline.Builder) *pipeline.Builder {
				return b.
					WithPluginTagOutput(plug, tag, "", "", "internal/enums").
					WithOutputDir("internal/cli")
			},
			want: pipeline.LayoutPolicy{
				Layout:      pipeline.LayoutAlongsideSource,
				LayoutFrom:  manifest.LayerFramework,
				PackageFrom: manifest.LayerFramework,
				Dir:         "internal/cli",
				DirFrom:     manifest.LayerCLI,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := policyFor(t, tc.configure); got != tc.want {
				t.Fatalf("LayoutPolicyForTag(%q, %q) = %+v, want %+v", plug, tag, got, tc.want)
			}
		})
	}
}

// dualRolePlugin implements two roles on one type — the composition
// plugin/doc.go documents and cli's registerPlugin performs for
// every role a plugin implements.
type dualRolePlugin struct{ name string }

// Name returns the configured plugin identifier.
func (p *dualRolePlugin) Name() string { return p.name }

// Annotate satisfies [plugin.Annotator]; the fixture stamps nothing.
func (*dualRolePlugin) Annotate(*plugin.AnnotatorContext) error { return nil }

// Generate satisfies [plugin.Generator]; the fixture emits nothing.
func (*dualRolePlugin) Generate(*plugin.GeneratorContext) error { return nil }

// TestBuilder_DuplicatePluginNames pins that the duplicate-name
// check distinguishes one plugin registered under several roles from
// two different plugins claiming one name.
//
// The check keyed on the name alone, so a dual-role plugin — the
// shape plugin/doc.go promises and cli/build.go's registerPlugin
// performs unconditionally — failed Build with ErrDuplicatePlugin.
// Every CLI-driven run with such a plugin was rejected on the
// documented composition.
func TestBuilder_DuplicatePluginNames(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, wire func(*pipeline.Builder)) error {
		t.Helper()
		b := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory())
		wire(b)
		_, err := b.Build()
		return err
	}

	t.Run("one instance under two roles builds", func(t *testing.T) {
		t.Parallel()
		d := &dualRolePlugin{name: "dual"}
		if err := build(t, func(b *pipeline.Builder) {
			b.WithAnnotator(d).WithGenerator(d)
		}); err != nil {
			t.Fatalf("dual-role registration failed: %v", err)
		}
	})

	t.Run("two instances sharing a name are rejected", func(t *testing.T) {
		t.Parallel()
		// The check's actual purpose. Exempting by identity must not
		// exempt by name, or two unrelated plugins could both claim
		// one identity in the manifest and the Plugins: header.
		err := build(t, func(b *pipeline.Builder) {
			b.WithAnnotator(&dualRolePlugin{name: "same"})
			b.WithGenerator(&dualRolePlugin{name: "same"})
		})
		if !errors.Is(err, pipeline.ErrDuplicatePlugin) {
			t.Fatalf("Build error = %v, want ErrDuplicatePlugin", err)
		}
	})

	t.Run("one instance registered twice in one role is rejected", func(t *testing.T) {
		t.Parallel()
		// Identity alone would permit this, and it would run Generate
		// twice — a mistake, not a composition, so the exemption is
		// scoped across roles rather than within one.
		d := &dualRolePlugin{name: "twice"}
		err := build(t, func(b *pipeline.Builder) {
			b.WithGenerator(d).WithGenerator(d)
		})
		if !errors.Is(err, pipeline.ErrDuplicatePlugin) {
			t.Fatalf("Build error = %v, want ErrDuplicatePlugin", err)
		}
	})
}

// dualRoleProvider is the realistic dual-role shape: a plugin is
// dual-role precisely because it owns a directive and generates from
// its own stamp, so it also implements DirectiveProvider and
// TemplateProvider. Neither path is reachable from a role-only
// fixture, and both were where the second registration surfaced.
type dualRoleProvider struct{ name string }

// Name returns the configured plugin identifier.
func (p *dualRoleProvider) Name() string { return p.name }

// Annotate satisfies [plugin.Annotator].
func (*dualRoleProvider) Annotate(*plugin.AnnotatorContext) error { return nil }

// Generate satisfies [plugin.Generator].
func (*dualRoleProvider) Generate(*plugin.GeneratorContext) error { return nil }

// Directives declares one schema, which a second registration of the
// same instance would reject as already registered.
func (p *dualRoleProvider) Directives() []directive.Schema {
	return []directive.Schema{directive.NewSchema(directive.Name(p.name)).Build()}
}

// Templates ships one template tree.
func (*dualRoleProvider) Templates(string) (fs.FS, bool) {
	return fstest.MapFS{
		"dual.x.tmpl": &fstest.MapFile{Data: []byte(`{{define "dual.x"}}{{end}}`)},
	}, true
}

// TemplateFuncs contributes one helper, which a second registration
// of the same instance would reject as colliding with itself.
func (*dualRoleProvider) TemplateFuncs(string) template.FuncMap {
	return template.FuncMap{"dualHelper": func() string { return "" }}
}

// TemplateOverrides replaces nothing.
func (*dualRoleProvider) TemplateOverrides(string) template.FuncMap { return nil }

// TestBuilder_DualRoleProviderBuilds pins that a plugin registered
// under two roles is counted once by every consumer of a flattened
// plugin list, not once per registration.
//
// Fixing the duplicate-name check alone was not enough: three other
// sites flatten the role slices, and two turn the second appearance
// into a Build error. Both name the plugin as colliding with itself,
// which is the tell.
func TestBuilder_DualRoleProviderBuilds(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T) error {
		t.Helper()
		d := &dualRoleProvider{name: "dualprov"}
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithAnnotator(d).
			WithGenerator(d).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		return err
	}

	t.Run("the directive registry accepts a single registration", func(t *testing.T) {
		t.Parallel()
		if err := build(t); errors.Is(err, pipeline.ErrDuplicateDirective) {
			t.Fatalf("directive registered twice for one plugin: %v", err)
		}
	})

	t.Run("the funcmap merge accepts a single registration", func(t *testing.T) {
		t.Parallel()
		if err := build(t); errors.Is(err, pipeline.ErrTemplateFuncCollision) {
			t.Fatalf("funcmap entry collided with its own plugin: %v", err)
		}
	})

	t.Run("Build succeeds", func(t *testing.T) {
		t.Parallel()
		// The two checks above name the specific failures; this one
		// catches any other site that learns the same wrong
		// assumption later.
		if err := build(t); err != nil {
			t.Fatalf("dual-role provider failed Build: %v", err)
		}
	})
}

// unhashablePlugin is a value-typed plugin carrying a slice, so its
// dynamic type is neither a valid map key nor an == operand. Several
// in-tree cli fixtures have exactly this shape.
type unhashablePlugin struct {
	name string
	body []byte
}

// Name returns the configured plugin identifier.
func (p unhashablePlugin) Name() string { return p.name }

// Generate satisfies [plugin.Generator].
func (unhashablePlugin) Generate(*plugin.GeneratorContext) error { return nil }

// Annotate satisfies [plugin.Annotator], so two instances of this
// type can occupy different role slices under one name — the shape
// that forces an identity comparison between two values Go cannot
// compare.
func (unhashablePlugin) Annotate(*plugin.AnnotatorContext) error { return nil }

// TestBuilder_UnhashablePluginDoesNotPanic pins that plugin identity
// comparison degrades rather than crashing.
//
// Deduplicating a flattened plugin list by instance needs a map key
// or an == operand, and Go panics at run time on either for a struct
// value holding a slice. A recovered panic surfaces as
// ExitInternalError from the CLI, which reads as a framework bug for
// a plugin shape the framework never said was illegal.
func TestBuilder_UnhashablePluginDoesNotPanic(t *testing.T) {
	t.Parallel()

	t.Run("a value-typed plugin carrying a slice builds", func(t *testing.T) {
		t.Parallel()
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(unhashablePlugin{name: "unhashable", body: []byte("x")}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
	})

	t.Run("two unhashable plugins sharing a name are still rejected", func(t *testing.T) {
		t.Parallel()
		// Two distinct values of the same non-comparable type, one
		// per role. Go compares interface types before values, so
		// only a matching type reaches the value comparison that
		// panics — a differing-type fixture proves nothing here.
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithAnnotator(unhashablePlugin{name: "same", body: []byte("a")}).
			WithGenerator(unhashablePlugin{name: "same", body: []byte("b")}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		if !errors.Is(err, pipeline.ErrDuplicatePlugin) {
			t.Fatalf("Build error = %v, want ErrDuplicatePlugin", err)
		}
	})
}
