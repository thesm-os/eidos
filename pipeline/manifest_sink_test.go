// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/manifest"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
)

// The recordingSink is unexported, so its observable behavior is
// reached through pipeline.Run wired with a manifest path. The
// tests below drive that path end-to-end: run the pipeline against
// a backend that performs sink.Writes, then read the written
// manifest and verify its contents.

// TestManifest_MultiOutputAttribution pins the multi-output
// manifest attribution: a tagged emit decl produces a manifest
// entry whose Plugins list carries the [emit.BaseEmit.OutputTag]
// in the contributing plugin's attribution. Primary-output
// entries marshal as bare strings (preserving byte-stable parity
// with pre-multi-output manifests); tagged entries marshal as
// objects with `output_tag`. Both flavours round-trip through
// [manifest.Read] so consumers can pair name + output_tag
// structurally without parsing the human-readable form.
func TestManifest_MultiOutputAttribution(t *testing.T) {
	t.Parallel()

	t.Run("tagged decl surfaces output_tag in the manifest's Plugins list", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestPath := filepath.Join(root, ".eidos", "manifest.json")
		mem := sink.NewMemory()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "a/x.go"}},
			Name:     "X", Package: "example.com/a",
		}
		fe := &nodePackageFE{name: "fe", pkg: &node.Package{
			Name: "a", Path: "example.com/a",
			Structs: []*node.Struct{origin},
		}}
		primary := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, SetByName: "enum"},
			Name:     "X", Package: "a",
		}
		tagged := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, SetByName: "enum", OutputTagName: "test"},
			Name:     "XTest", Package: "a",
		}
		gen := &layoutGen{
			name: "enum",
			outputs: []plugin.Output{
				{Suffix: "_enum.go"},
				{Tag: "test", Suffix: "_enum_test.go"},
			},
			pkg: &emit.Package{
				Name: "a", Path: "example.com/a",
				Structs: []*emit.Struct{primary, tagged},
			},
		}
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				ctx.Reader.EmitStructs().Each(func(s *emit.Struct) {
					_ = ctx.Sink.Write(s.Target, []byte("body-"+s.Name))
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithGenerator(gen).
			WithBackend(be).
			WithSink(mem).
			WithManifestPath(manifestPath).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "a"))

		m, err := manifest.Read(manifestPath)
		assertNoError(t, err)
		if len(m.Outputs) != 2 {
			t.Fatalf("manifest should record 2 outputs; got %d", len(m.Outputs))
		}

		byFilename := map[string]manifest.Output{}
		for _, o := range m.Outputs {
			byFilename[o.Target.Filename] = o
		}
		primaryOut, ok := byFilename["x_enum.go"]
		if !ok {
			t.Fatalf("manifest missing primary output entry; got %v", byFilename)
		}
		if len(primaryOut.Plugins) != 1 || primaryOut.Plugins[0].Name != "enum" {
			t.Errorf("primary Plugins = %+v, want one attribution for enum", primaryOut.Plugins)
		}
		if got := primaryOut.Plugins[0].OutputTag; got != "" {
			t.Errorf("primary attribution OutputTag = %q, want empty", got)
		}
		taggedOut, ok := byFilename["x_enum_test.go"]
		if !ok {
			t.Fatalf("manifest missing tagged output entry; got %v", byFilename)
		}
		if len(taggedOut.Plugins) != 1 || taggedOut.Plugins[0].Name != "enum" {
			t.Errorf("tagged Plugins = %+v, want one attribution for enum", taggedOut.Plugins)
		}
		if got, want := taggedOut.Plugins[0].OutputTag, "test"; got != want {
			t.Errorf("tagged attribution OutputTag = %q, want %q", got, want)
		}
	})

	t.Run("primary attribution serialises as a bare string on disk", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestPath := filepath.Join(root, ".eidos", "manifest.json")
		mem := sink.NewMemory()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "a/x.go"}},
			Name:     "X", Package: "example.com/a",
		}
		fe := &nodePackageFE{name: "fe", pkg: &node.Package{
			Name: "a", Path: "example.com/a",
			Structs: []*node.Struct{origin},
		}}
		primary := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, SetByName: "enum"},
			Name:     "X", Package: "a",
		}
		gen := &layoutGen{
			name:    "enum",
			outputs: []plugin.Output{{Suffix: "_enum.go"}},
			pkg: &emit.Package{
				Name: "a", Path: "example.com/a",
				Structs: []*emit.Struct{primary},
			},
		}
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				ctx.Reader.EmitStructs().Each(func(s *emit.Struct) {
					_ = ctx.Sink.Write(s.Target, []byte("body"))
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithGenerator(gen).
			WithBackend(be).
			WithSink(mem).
			WithManifestPath(manifestPath).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "a"))

		body, err := os.ReadFile(manifestPath)
		assertNoError(t, err)
		// The bare-string form for the plugin element (`"enum"`,
		// not `{"name":"enum"}`) is the byte-stable shape for
		// primary-output entries — any drift toward object form
		// would break manifests written before the M4 output_tag
		// surface landed. The exact whitespace around the array
		// follows the json.MarshalIndent contract; we assert on
		// the indented element form the encoder uses.
		if !strings.Contains(string(body), "\"enum\"\n") {
			t.Errorf("primary attribution should serialise as bare string; got:\n%s", body)
		}
		if strings.Contains(string(body), `"name": "enum"`) {
			t.Errorf("primary attribution leaked into object form; got:\n%s", body)
		}
	})
}

func TestManifest_RecordsBackendWrites(t *testing.T) {
	t.Parallel()

	t.Run("written manifest lists every sink.Write call with its content hash", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestPath := filepath.Join(root, ".eidos", "manifest.json")
		mem := sink.NewMemory()
		// Two source structs in distinct source files so Layout
		// composes distinct Targets the backend can write to —
		// the recording sink keys captures by Target so two writes
		// against the routed Targets produce two manifest entries.
		srcX := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "a/x.go"}},
			Name:     "X", Package: "example.com/a",
		}
		srcY := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "a/y.go"}},
			Name:     "Y", Package: "example.com/a",
		}
		fe := &nodePackageFE{name: "fe", pkg: &node.Package{
			Name: "a", Path: "example.com/a",
			Structs: []*node.Struct{srcX, srcY},
		}}
		gen := &recGen{name: "gen", suffix: "_gen.go", generate: func(ctx *plugin.GeneratorContext) {
			_ = ctx.Store.Emit().AddPackage(&emit.Package{
				Name: "a", Path: "example.com/a",
				Structs: []*emit.Struct{
					{BaseEmit: emit.BaseEmit{OriginNode: srcX, SetByName: "gen"}, Name: "X", Package: "a"},
					{BaseEmit: emit.BaseEmit{OriginNode: srcY, SetByName: "gen"}, Name: "Y", Package: "a"},
				},
			})
		}}
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				ctx.Reader.EmitStructs().Each(func(s *emit.Struct) {
					_ = ctx.Sink.Write(s.Target, []byte("hello-"+s.Name))
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithGenerator(gen).
			WithBackend(be).
			WithSink(mem).
			WithManifestPath(manifestPath).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "a"))

		// Manifest exists and parses.
		body, err := os.ReadFile(manifestPath)
		assertNoError(t, err)
		if len(body) == 0 {
			t.Fatalf("manifest should be non-empty")
		}
		m, err := manifest.Read(manifestPath)
		assertNoError(t, err)
		if len(m.Outputs) != 2 {
			t.Fatalf("manifest should record 2 outputs; got %d", len(m.Outputs))
		}
		seen := map[emit.Target]string{}
		for _, o := range m.Outputs {
			seen[o.Target] = o.Hash
		}
		x := seen[emit.Target{
			Dir: "a", Filename: "x_gen.go", Package: "a", ImportPath: "example.com/a",
		}]
		y := seen[emit.Target{
			Dir: "a", Filename: "y_gen.go", Package: "a", ImportPath: "example.com/a",
		}]
		if x == "" || y == "" {
			t.Fatalf("manifest should hash both routed targets; got %+v", seen)
		}
	})

	t.Run("no manifest path → no manifest written", func(t *testing.T) {
		t.Parallel()
		mem := sink.NewMemory()
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				_ = ctx.Sink.Write(emit.Target{Dir: "a", Filename: "x.go", Package: "x"}, []byte("hello-x"))
			},
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(be).
			WithSink(mem).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		// Nothing to assert positively for "no manifest" beyond
		// the pipeline completing without writes to a path the
		// test never configured.
	})

	t.Run("inner-sink Write errors propagate; nothing is captured for that target", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestPath := filepath.Join(root, "manifest.json")
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				// One successful write + one failing write attempt.
				// The pipeline's outer Sink is a Multi[good, fail],
				// but we don't have Multi-with-fail at hand; use a
				// failing wrapper instead.
				_ = ctx.Sink.Write(emit.Target{Dir: "a", Filename: "ok.go", Package: "x"}, []byte("ok-payload"))
			},
		}
		// Inner sink that fails the first write so recordingSink's
		// error path is exercised.
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(be).
			WithSink(&failingSink{err: errFailingSink}).
			WithManifestPath(manifestPath).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		// Manifest was still written but has no captured entries
		// (the only write attempt errored).
		m, err := manifest.Read(manifestPath)
		assertNoError(t, err)
		if len(m.Outputs) != 0 {
			t.Fatalf("expected zero outputs (failing inner sink); got %+v", m.Outputs)
		}
	})

	t.Run("per-output Plugins lists only the contributing plugin when entities are attributed", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestPath := filepath.Join(root, "manifest.json")
		// The source struct lives in package "x" at file "a/x.go"
		// so the Layout phase composes Target.Dir from the source
		// directory and Target.Filename from the basename plus
		// recGen's default "_gen.go" suffix.
		src := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "a/x.go"}},
			Name:     "X", Package: "example.com/x",
		}
		composedTarget := emit.Target{
			Dir: "a", Filename: "x_gen.go", Package: "x", ImportPath: "example.com/x",
		}
		// A generator that adds an emit struct stamped with the
		// plugin's SetBy via the builder. The recording sink walks
		// the store at manifest-write time and reads SetBy to
		// attribute outputs.
		gen := &recGen{name: "attrgen", generate: func(ctx *plugin.GeneratorContext) {
			c := builder.For("attrgen", emit.Target{})
			pkg := c.Package("x", "example.com/x")
			pkg.Struct("X", func(b *builder.StructBuilder) {
				b.Origin(src)
			})
			out, err := pkg.Build()
			if err != nil {
				t.Fatalf("pkg.Build: %v", err)
			}
			if err := ctx.Store.Emit().AddPackage(out); err != nil {
				t.Fatalf("AddPackage: %v", err)
			}
		}}
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				ctx.Reader.EmitStructs().Each(func(s *emit.Struct) {
					_ = ctx.Sink.Write(s.Target, []byte("body"))
				})
			},
		}
		fe := &nodePackageFE{name: "fe", pkg: &node.Package{
			Name: "x", Path: "example.com/x",
			Structs: []*node.Struct{src},
		}}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithGenerator(gen).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			WithManifestPath(manifestPath).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		_ = composedTarget // documents the expected composition
		m, err := manifest.Read(manifestPath)
		assertNoError(t, err)
		if len(m.Outputs) != 1 {
			t.Fatalf("expected 1 output; got %d", len(m.Outputs))
		}
		got := m.Outputs[0].Plugins
		if len(got) != 1 || got[0].Name != "attrgen" {
			t.Fatalf("Plugins = %v, want [attrgen] (only contributing plugin)", got)
		}
	})

	t.Run("Plugins aggregates entity SetBy, slot Provenance, and method-host recursion", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestPath := filepath.Join(root, "manifest.json")
		// Three host source decls so the Layout phase has Origins to
		// route every emit kind through. All three share a source
		// file and source package so the Layout phase composes a
		// single (Dir, Filename, Package) for every routable decl;
		// the resulting outputs compose into one manifest entry.
		srcStruct := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "a/x.go"}},
			Name:     "S", Package: "example.com/x",
		}
		srcIface := &node.Interface{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "a/x.go"}},
			Name:     "I", Package: "example.com/x",
		}
		srcAlias := &node.Alias{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "a/x.go"}},
			Name:     "N", Package: "example.com/x",
		}
		fe := &nodePackageFE{name: "fe", pkg: &node.Package{
			Name: "x", Path: "example.com/x",
			Structs: []*node.Struct{srcStruct}, Interfaces: []*node.Interface{srcIface},
			Aliases: []*node.Alias{srcAlias},
		}}
		// One generator hosts every emit entity but the slot
		// contributions and the per-method prebody come from
		// independent Contexts so manifest attribution must aggregate
		// every distinct SetBy.
		gen := &recGen{name: "host", generate: func(ctx *plugin.GeneratorContext) {
			host := builder.For("host", emit.Target{})
			fieldgen := builder.For("fieldgen", emit.Target{})
			methgen := builder.For("methgen", emit.Target{})
			weaver := builder.For("weaver", emit.Target{})
			pkg := host.Package("x", "example.com/x")
			pkg.Struct("S", func(b *builder.StructBuilder) {
				b.Origin(srcStruct).Method("M", func(mb *builder.MethodBuilder) {
					_ = weaver.AppendPrebody(mb.Node(), builder.RawStmt("_ = 0"))
				})
				_ = fieldgen.AppendField(b.Node(), &emit.Field{Name: "F", Type: emit.External("io", "Reader")})
			})
			pkg.Interface("I", func(b *builder.InterfaceBuilder) {
				b.Origin(srcIface)
				_ = methgen.AppendMethod(b.Node(), &emit.Method{Name: "Do"})
			})
			pkg.NamedType("N", emit.External("io", "Reader"), func(b *builder.AliasBuilder) {
				b.Origin(srcAlias).Method("M", func(*builder.MethodBuilder) {})
			})
			out, err := pkg.Build()
			if err != nil {
				t.Fatalf("pkg.Build: %v", err)
			}
			if err := ctx.Store.Emit().AddPackage(out); err != nil {
				t.Fatalf("AddPackage: %v", err)
			}
		}}
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				// Write to one of the routed targets — the struct's
				// composed target — so the manifest captures a
				// single output whose Plugins list aggregates every
				// contributor.
				ctx.Reader.EmitStructs().Each(func(s *emit.Struct) {
					_ = ctx.Sink.Write(s.Target, []byte("body"))
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithGenerator(gen).
			WithGenerator(&recGen{name: "fieldgen"}).
			WithGenerator(&recGen{name: "methgen"}).
			WithGenerator(&recGen{name: "weaver"}).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			WithManifestPath(manifestPath).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		m, err := manifest.Read(manifestPath)
		assertNoError(t, err)
		if len(m.Outputs) != 1 {
			t.Fatalf("expected 1 output; got %d", len(m.Outputs))
		}
		got := m.Outputs[0].Plugins
		// Order follows the pipeline's plugin-registration order;
		// every distinct SetBy across entity, slot, and method-host
		// recursion appears exactly once.
		want := map[string]bool{"host": true, "fieldgen": true, "methgen": true, "weaver": true}
		if len(got) != len(want) {
			t.Fatalf("Plugins = %v, want every contributor (%v)", got, want)
		}
		for _, attr := range got {
			if !want[attr.Name] {
				t.Fatalf("Plugins lists %q which did not contribute; got %v", attr.Name, got)
			}
			delete(want, attr.Name)
		}
		if len(want) != 0 {
			t.Fatalf("Plugins missing contributors %v; got %v", want, m.Outputs[0].Plugins)
		}
	})

	t.Run("ResolvedLayout records per-field precedence-layer attribution", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestPath := filepath.Join(root, "manifest.json")
		nodePkg := &node.Package{Name: "x", Path: "example.com/x"}
		// The struct's +gen:out directive pins Filename to a
		// directive-resolved value; the CLI -p override pins
		// Package; the source position pins Dir under the
		// alongside-source default. The manifest's ResolvedFrom
		// block names the precedence layer that supplied each
		// field.
		src := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "a/x.go"},
				DirectiveList: []*directive.Directive{
					{Name: pipeline.OutDirective, Args: []string{"pinned.go"}},
				},
			},
			Name: "X", Package: "example.com/x",
		}
		nodePkg.Structs = []*node.Struct{src}
		gen := &recGen{name: "rg", generate: func(ctx *plugin.GeneratorContext) {
			c := builder.For("rg", emit.Target{})
			pkg := c.Package("x", "example.com/x")
			pkg.Struct("X", func(b *builder.StructBuilder) {
				b.Origin(src)
			})
			out, err := pkg.Build()
			if err != nil {
				t.Fatalf("pkg.Build: %v", err)
			}
			if err := ctx.Store.Emit().AddPackage(out); err != nil {
				t.Fatalf("AddPackage: %v", err)
			}
		}}
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				ctx.Reader.EmitStructs().Each(func(s *emit.Struct) {
					_ = ctx.Sink.Write(s.Target, []byte("body"))
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(gen).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			WithManifestPath(manifestPath).
			WithOutputPackage("generated").
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		m, err := manifest.Read(manifestPath)
		assertNoError(t, err)
		if len(m.Outputs) != 1 {
			t.Fatalf("expected 1 output; got %d", len(m.Outputs))
		}
		rl := m.Outputs[0].ResolvedLayout
		if rl == nil {
			t.Fatalf("ResolvedLayout missing on the manifest output")
		}
		if got, want := rl.Layout, pipeline.LayoutAlongsideSource; got != want {
			t.Errorf("Layout = %q, want %q", got, want)
		}
		if got, want := rl.Dir, "a"; got != want {
			t.Errorf("Dir = %q, want %q", got, want)
		}
		if got, want := rl.Filename, "pinned.go"; got != want {
			t.Errorf("Filename = %q, want %q", got, want)
		}
		if got, want := rl.Package, "generated"; got != want {
			t.Errorf("Package = %q, want %q", got, want)
		}
		wantFrom := map[string]manifest.Layer{
			"layout":   manifest.LayerFramework,
			"dir":      manifest.LayerFramework,
			"filename": manifest.LayerDirective,
			"package":  manifest.LayerCLI,
		}
		for field, want := range wantFrom {
			if got := rl.ResolvedFrom[field]; got != want {
				t.Errorf("ResolvedFrom[%q] = %q, want %q", field, got, want)
			}
		}
	})

	t.Run("centralised layout stamps cli on every policy-driven field", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestPath := filepath.Join(root, "manifest.json")
		src := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "a/x.go"}},
			Name:     "X", Package: "example.com/x",
		}
		fe := &nodePackageFE{name: "fe", pkg: &node.Package{
			Name: "x", Path: "example.com/x",
			Structs: []*node.Struct{src},
		}}
		gen := &recGen{name: "rg", suffix: "_gen.go", generate: func(ctx *plugin.GeneratorContext) {
			c := builder.For("rg", emit.Target{})
			pkg := c.Package("x", "example.com/x")
			pkg.Struct("X", func(b *builder.StructBuilder) {
				b.Origin(src)
			})
			out, err := pkg.Build()
			if err != nil {
				t.Fatalf("pkg.Build: %v", err)
			}
			if err := ctx.Store.Emit().AddPackage(out); err != nil {
				t.Fatalf("AddPackage: %v", err)
			}
		}}
		be := &recBE{name: "be", lang: "stub", render: func(ctx *plugin.BackendContext) {
			ctx.Reader.EmitStructs().Each(func(s *emit.Struct) {
				_ = ctx.Sink.Write(s.Target, []byte("body"))
			})
		}}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithGenerator(gen).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			WithManifestPath(manifestPath).
			WithOutputLayout(pipeline.LayoutCentralised).
			WithOutputPackage("gen").
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		m, err := manifest.Read(manifestPath)
		assertNoError(t, err)
		if len(m.Outputs) != 1 {
			t.Fatalf("expected 1 output; got %d", len(m.Outputs))
		}
		rl := m.Outputs[0].ResolvedLayout
		if rl == nil {
			t.Fatalf("ResolvedLayout missing")
		}
		if rl.Layout != pipeline.LayoutCentralised {
			t.Errorf("Layout = %q, want centralised", rl.Layout)
		}
		wantFrom := map[string]manifest.Layer{
			"layout":   manifest.LayerCLI,
			"dir":      manifest.LayerCLI,
			"package":  manifest.LayerCLI,
			"filename": manifest.LayerPluginSuffix,
		}
		for field, want := range wantFrom {
			if got := rl.ResolvedFrom[field]; got != want {
				t.Errorf("ResolvedFrom[%q] = %q, want %q", field, got, want)
			}
		}
	})

	t.Run("ResolvedLayout round-trips through manifest IO", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestPath := filepath.Join(root, "manifest.json")
		assertNoError(t, manifest.Write(manifestPath, &manifest.Manifest{
			Version: manifest.Version,
			RunID:   "test-run",
			Outputs: []manifest.Output{{
				Target: emit.Target{Dir: "a", Filename: "x.go", Package: "p"},
				Hash:   "sha256:abc",
				ResolvedLayout: &manifest.ResolvedLayout{
					Layout: pipeline.LayoutCentralised,
					Dir:    "a", Filename: "x.go", Package: "p",
					ResolvedFrom: map[string]manifest.Layer{
						"layout":   manifest.LayerCLI,
						"dir":      manifest.LayerCLI,
						"filename": manifest.LayerPluginSuffix,
						"package":  manifest.LayerCLI,
					},
				},
			}},
		}))
		m, err := manifest.Read(manifestPath)
		assertNoError(t, err)
		rl := m.Outputs[0].ResolvedLayout
		if rl == nil {
			t.Fatalf("ResolvedLayout lost on round-trip")
		}
		if rl.Layout != pipeline.LayoutCentralised {
			t.Errorf("Layout round-trip = %q", rl.Layout)
		}
		if got, want := rl.ResolvedFrom["filename"], manifest.LayerPluginSuffix; got != want {
			t.Errorf("ResolvedFrom[filename] = %q, want %q", got, want)
		}
	})

	t.Run("backend writing an unrouted Target during a routed run emits Internal", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestPath := filepath.Join(root, "manifest.json")
		// A normal routed Target via the generator, plus a
		// synthetic write directly through ctx.Sink to a Target
		// the Layout phase never composed — the framework-bug
		// pattern asManifest catches with an Internal diagnostic.
		src := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "a/x.go"}},
			Name:     "X", Package: "example.com/x",
		}
		fe := &nodePackageFE{name: "fe", pkg: &node.Package{
			Name: "x", Path: "example.com/x",
			Structs: []*node.Struct{src},
		}}
		gen := &recGen{name: "rg", suffix: "_gen.go", generate: func(ctx *plugin.GeneratorContext) {
			c := builder.For("rg", emit.Target{})
			pkg := c.Package("x", "example.com/x")
			pkg.Struct("X", func(b *builder.StructBuilder) { b.Origin(src) })
			out, err := pkg.Build()
			if err != nil {
				t.Fatalf("pkg.Build: %v", err)
			}
			if err := ctx.Store.Emit().AddPackage(out); err != nil {
				t.Fatalf("AddPackage: %v", err)
			}
		}}
		be := &recBE{name: "be", lang: "stub", render: func(ctx *plugin.BackendContext) {
			// One routed write (matches the composed Target) ...
			ctx.Reader.EmitStructs().Each(func(s *emit.Struct) {
				_ = ctx.Sink.Write(s.Target, []byte("ok"))
			})
			// ... plus one synthetic write to a Target Layout
			// never composed. The recording sink captures both;
			// asManifest surfaces the second as an Internal diag.
			_ = ctx.Sink.Write(
				emit.Target{Dir: "rogue", Filename: "z.go", Package: "z"},
				[]byte("rogue"),
			)
		}}
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(fe).
			WithGenerator(gen).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			WithManifestPath(manifestPath).
			WithDiag(d).
			Build()
		assertNoError(t, err)
		runErr := p.Run(t.Context(), "x")
		if !errors.Is(runErr, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run = %v, want ErrRunHadErrors", runErr)
		}
		var sawInternal bool
		for _, dg := range d.Diagnostics() {
			match := dg.Severity == diag.Internal &&
				strings.Contains(dg.Message, "without Layout-composed attribution")
			if match {
				sawInternal = true
				break
			}
		}
		if !sawInternal {
			t.Fatalf("expected Internal diagnostic; got %+v", d.Diagnostics())
		}
	})

	t.Run("manifest write failure emits a Warn diagnostic", func(t *testing.T) {
		t.Parallel()
		// Point the manifest at a path whose parent is a regular
		// file so MkdirAll inside manifest.Write fails.
		root := t.TempDir()
		conflict := filepath.Join(root, "block")
		assertNoError(t, os.WriteFile(conflict, nil, 0o600))
		manifestPath := filepath.Join(conflict, ".eidos", "manifest.json")

		mem := sink.NewMemory()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(mem).
			WithManifestPath(manifestPath).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		// Manifest write failure surfaces as Warn (not Error) so the
		// run still returns nil.
		if p.Diag().Count(diag.Warn) == 0 {
			t.Fatalf("expected a Warn diagnostic about manifest-write failure")
		}
	})

	t.Run("Outputs slice sort is stable across runs", func(t *testing.T) {
		t.Parallel()
		// Captured Targets used to iterate in Go map-iteration order
		// — reshuffling the manifest's Outputs every run. The fix
		// sorts Targets by (Dir, Filename, Package, ImportPath) so
		// two runs against identical input produce identical
		// Outputs ordering.
		root := t.TempDir()
		manifestPathA := filepath.Join(root, "a", "manifest.json")
		manifestPathB := filepath.Join(root, "b", "manifest.json")
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				_ = ctx.Sink.Write(emit.Target{Dir: "a", Filename: "x.go", Package: "x"}, []byte("body-x"))
				_ = ctx.Sink.Write(emit.Target{Dir: "a", Filename: "y.go", Package: "x"}, []byte("body-y"))
				_ = ctx.Sink.Write(emit.Target{Dir: "b", Filename: "z.go", Package: "z"}, []byte("body-z"))
			},
		}
		run := func(t *testing.T, path string) *manifest.Manifest {
			t.Helper()
			p, err := pipeline.New().
				WithFrontend(&stubFE{name: "fe"}).
				WithBackend(be).
				WithSink(sink.NewMemory()).
				WithManifestPath(path).
				Build()
			assertNoError(t, err)
			assertNoError(t, p.Run(t.Context()))
			m, err := manifest.Read(path)
			assertNoError(t, err)
			return m
		}
		first := run(t, manifestPathA)
		second := run(t, manifestPathB)
		if len(first.Outputs) != len(second.Outputs) {
			t.Fatalf("Outputs len mismatch: %d vs %d", len(first.Outputs), len(second.Outputs))
		}
		for i := range first.Outputs {
			if first.Outputs[i].Target != second.Outputs[i].Target {
				t.Fatalf(
					"Outputs[%d] Target mismatch: %+v vs %+v (reshuffle)",
					i, first.Outputs[i].Target, second.Outputs[i].Target,
				)
			}
		}
	})

	t.Run(
		"re-run against identical input leaves the manifest's bytes untouched",
		func(t *testing.T) {
			t.Parallel()
			// The pipeline stamps RunID with the wall-clock — every
			// run would otherwise produce a fresh timestamp and
			// dirty the manifest in version control even when no
			// rendered output changed. The fix skips the write
			// when an existing manifest's content (Outputs +
			// Brand + Version) matches what we'd produce now.
			root := t.TempDir()
			manifestPath := filepath.Join(root, ".eidos", "manifest.json")
			be := &recBE{
				name: "be", lang: "stub",
				render: func(ctx *plugin.BackendContext) {
					_ = ctx.Sink.Write(
						emit.Target{Dir: "a", Filename: "x.go", Package: "x"},
						[]byte("hello"),
					)
				},
			}
			run := func(t *testing.T) {
				t.Helper()
				p, err := pipeline.New().
					WithFrontend(&stubFE{name: "fe"}).
					WithBackend(be).
					WithSink(sink.NewMemory()).
					WithManifestPath(manifestPath).
					Build()
				assertNoError(t, err)
				assertNoError(t, p.Run(t.Context()))
			}
			run(t)
			first, err := os.ReadFile(manifestPath)
			assertNoError(t, err)
			beforeRerun, err := os.Stat(manifestPath)
			assertNoError(t, err)
			run(t)
			second, err := os.ReadFile(manifestPath)
			assertNoError(t, err)
			afterRerun, err := os.Stat(manifestPath)
			assertNoError(t, err)
			// Byte equality alone cannot observe the skip this test
			// is named for. RunID is stamped at one-second
			// resolution, so two runs inside the same second produce
			// identical bytes whether the write was skipped or
			// performed — and sleeping a second to force the
			// difference is not worth it in a unit test. manifest.Write
			// publishes through a temp file and a rename, so a write
			// that did happen swaps the inode underneath the path:
			// os.SameFile is what distinguishes "skipped the write"
			// from "rewrote the same content".
			if !os.SameFile(beforeRerun, afterRerun) {
				t.Fatalf(
					"re-run replaced the manifest file instead of skipping the write "+
						"(inode changed at %s)",
					manifestPath,
				)
			}
			if !bytes.Equal(first, second) {
				t.Fatalf(
					"re-run dirtied the manifest:\nfirst=%s\nsecond=%s",
					first,
					second,
				)
			}
		},
	)
}

// TestManifest_OutputOrderTiebreaks pins the third and fourth keys of
// the Target sort comparator that fixes the manifest's Outputs order —
// Package, then ImportPath, after Dir and Filename.
//
// Every existing ordering test uses targets that already differ on Dir
// or Filename, so the comparator returned before it ever reached the
// Package tiebreak and negating that comparison survived mutation
// testing. Negated, the comparator reports two Targets that share a
// Dir and Filename but differ in Package as unordered, and reports two
// that agree on Package as ordered without consulting ImportPath. The
// relation stops being a total order, slices.SortFunc is not stable,
// and the surviving order falls out of map iteration — so the
// manifest's bytes reshuffle on every run against unchanged input,
// destroying the stable-bytes property the sort exists to provide.
//
// Both collisions are reachable in production: the _test.go package
// shift and a per-plugin package override can each hand two plugins
// the same Dir and Filename under different Packages, and a
// centralised layout can route two source packages to one directory
// under different ImportPaths.
//
// The run repeats because the mutation does not produce one specific
// wrong order — it produces an arbitrary one. A single run could draw
// the sorted order by luck; independent runs drawing it every time
// cannot.
func TestManifest_OutputOrderTiebreaks(t *testing.T) {
	t.Parallel()

	// Five targets whose first two sort keys collide, so the whole
	// ordering rests on the Package and ImportPath tiebreaks. The
	// backend writes them in an order that is not the sorted one, so
	// a comparator that declines to order them leaves them wrong.
	want := []emit.Target{
		{Dir: "internal/gen", Filename: "x_gen.go", Package: "alpha", ImportPath: "example.com/one"},
		{Dir: "internal/gen", Filename: "x_gen.go", Package: "alpha", ImportPath: "example.com/two"},
		{Dir: "internal/gen", Filename: "x_gen.go", Package: "beta", ImportPath: "example.com/one"},
		{Dir: "internal/gen", Filename: "x_gen.go", Package: "gamma", ImportPath: "example.com/one"},
		{Dir: "internal/z", Filename: "x_gen.go", Package: "alpha", ImportPath: "example.com/one"},
	}
	written := make([]emit.Target, len(want))
	copy(written, want)
	slices.Reverse(written)

	be := &recBE{
		name: "be", lang: "stub",
		render: func(ctx *plugin.BackendContext) {
			for _, target := range written {
				_ = ctx.Sink.Write(target, []byte("body"))
			}
		},
	}

	// manifestOrder runs one pipeline and returns the Targets in the
	// order the manifest recorded them. Dry-run mode keeps the prior
	// manifest off disk, so LastManifest is the freshly assembled one
	// and not the output of the separate merge-time re-sort.
	manifestOrder := func(t *testing.T) []emit.Target {
		t.Helper()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			WithManifestPath(filepath.Join(t.TempDir(), "manifest.json")).
			WithDryRun(true).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		m := p.LastManifest()
		if m == nil {
			t.Fatalf("LastManifest = nil; the run assembled no manifest")
		}
		got := make([]emit.Target, 0, len(m.Outputs))
		for _, o := range m.Outputs {
			got = append(got, o.Target)
		}
		return got
	}

	t.Run("outputs sharing a dir and filename order by package then import path", func(t *testing.T) {
		t.Parallel()
		const runs = 24
		for i := range runs {
			if got := manifestOrder(t); !slices.Equal(got, want) {
				t.Fatalf("run %d ordered the manifest as\n  %+v\nwant\n  %+v", i, got, want)
			}
		}
	})
}
