// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package frontendtest is the abstract frontend-author harness.
// Tests pass a [plugin.Frontend] instance + a source-fixture
// path; the harness drives the frontend through a pipeline
// (optionally with annotators / generators / a backend) and
// surfaces the resulting store / sink / diagnostics for
// assertions.
//
// The package supersedes language-specific harnesses (the
// previous demopipe and protopipe wrappers): a caller hands over
// `frontend_golang.New()`, `protobuf.New()` or its own frontend
// and the harness's surface stays neutral — it knows no source
// language of its own.
//
// Two entry points: [Run] builds a full pipeline and returns the
// rendered outputs; [LoadDirect] invokes the frontend's Load
// surface only and returns the produced node graph + diagnostics
// without the pipeline-build invariants noise. Use Run for tests
// that exercise the full plugin chain; use LoadDirect for tests
// that assert on the frontend's source-mapping in isolation.
package frontendtest

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
	"go.thesmos.sh/eidos/store"
)

// DemoFixture returns the absolute path of the framework's
// shared Go-source fixture under [eidostest/demoproject]. Tests
// driving the Go frontend through this harness use it as the
// SourceDir value:
//
//	frontendtest.Run(t, frontendtest.RunOptions{
//	    Frontend:  frontend_golang.New(),
//	    SourceDir: frontendtest.DemoFixture(t),
//	    ...
//	})
//
// The function resolves through [runtime.Caller] so the result
// is stable regardless of the test's working directory. Stable
// and present are different properties, so it also stats the
// result and fails the test when the fixture is not there.
//
// Absent is the normal case outside an eidos checkout. The
// fixture tree carries its own go.mod, and the module zipper
// skips every directory holding one, so eidostest/testdata is
// missing from the published eidostest module in full. A test
// resolving this helper from the module cache therefore stops
// here rather than handing the pipeline a path to nothing and
// asserting over an empty store. Downstream authors point
// [RunOptions.SourceDir] at their own testdata directory
// instead.
func DemoFixture(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("frontendtest.DemoFixture: runtime.Caller failed")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "testdata", "demoproject")
	if err := sourceDirError(dir); err != nil {
		t.Fatalf("frontendtest.DemoFixture: %v; the shared fixture is not shipped in the "+
			"published eidostest module — it is a nested Go module, so the module zip "+
			"excludes eidostest/testdata, and this helper resolves only inside an eidos "+
			"checkout: set RunOptions.SourceDir to your own testdata directory instead", err)
	}
	return dir
}

// sourceDirError reports why dir is unusable as a source fixture,
// or nil when dir is an existing directory. [DemoFixture] and
// [Run] call it before a frontend ever sees the value: a frontend
// handed a path that is not there reports it as a diagnostic, and
// a diagnostic no test inspects is indistinguishable from a
// passing run over an empty store.
//
// The message carries no prefix of its own; each caller names the
// entry point that rejected the value and what to pass instead.
//
// One stat syscall; no allocation on the nil path.
func sourceDirError(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("source directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source directory %q is not a directory", dir)
	}
	return nil
}

// RunOptions configures one [Run] or [LoadDirect] call.
type RunOptions struct {
	// Frontend is the frontend instance the harness drives.
	// Required.
	Frontend plugin.Frontend

	// SourceDir is the absolute path of the source fixture the
	// frontend loads from. Required, and for [Run] it must name an
	// existing directory; the harness sets the frontend's `dir`
	// option to this value unless overridden via [FrontendOptions]
	// or [PluginOptions].
	SourceDir string

	// Pattern overrides the default `./...` recursive sweep the
	// harness passes to the pipeline. Empty falls back to the
	// canonical recursive form.
	Pattern string

	// FrontendOptions extends or overrides the harness's default
	// frontend options. The harness always sets `dir` from
	// [SourceDir]; entries here add to or override that, with
	// entries in [PluginOptions] keyed by the frontend's name
	// winning over the [FrontendOptions] map.
	FrontendOptions map[string]string

	// Annotators register on the pipeline in slice order.
	Annotators []plugin.Annotator

	// Generators register on the pipeline in slice order.
	Generators []plugin.Generator

	// Backend registers as the pipeline's backend. Optional; when
	// nil [Run] uses a no-op backend that satisfies the
	// pipeline's build invariants without emitting anything. Use
	// nil for tests that assert on the produced node graph or
	// emit graph without driving a render pass.
	Backend plugin.Backend

	// Sink captures the backend's output when [Backend] is set.
	// Defaults to a fresh in-memory sink when nil.
	Sink sink.Sink

	// PluginOptions carries per-plugin configuration keyed by
	// plugin name. Entries are forwarded to the builder verbatim
	// except for the frontend's entry, which is merged into the
	// frontend's option map per the precedence rule documented on
	// [RunOptions.FrontendOptions].
	PluginOptions map[string]map[string]string

	// Cache is the cache instance the pipeline uses. Defaults to
	// [cache.NewNone] when nil.
	Cache cache.Cache

	// Layout selects the routing layout. Empty defers to the
	// framework default.
	Layout string

	// OutputPackage pins [emit.Target.Package] for every emitted
	// decl in scope and supplies the shared package name
	// centralised layouts require. Empty leaves the per-decl
	// default in place.
	OutputPackage string

	// OutputDir sets the rendered output directory under
	// centralised layout. Ignored under alongside-source.
	OutputDir string

	// TargetSymbol restricts the run to source decls whose
	// unqualified Name equals the value, or whose QName ends with
	// `.<value>`. Empty disables scoping.
	TargetSymbol string

	// Command pins the literal text the backend stamps into the
	// `Command:` header line of every rendered file. Empty
	// leaves the framework's `os.Args`-derived value in place;
	// tests asserting against committed baselines across processes
	// set this to a stable string.
	Command string

	// ManifestPath, when set, instructs the pipeline to write
	// the per-run manifest at this absolute path. Empty leaves
	// the pipeline's manifest unconfigured (no manifest written).
	ManifestPath string

	// SourceRoot pins [pipeline.Builder.WithSourceRoot] so the
	// rendered `Source:` header line stays stable across machines
	// regardless of the user's checkout location. Empty defers
	// to the framework's default.
	SourceRoot string
}

// Result captures the outcome of a [Run] or [LoadDirect] call.
type Result struct {
	// Sink is the destination sink that captured backend output.
	// Nil when [RunOptions.Backend] is nil and no sink was
	// supplied (the no-op backend writes nothing).
	Sink sink.Sink

	// Diag is the diagnostic sink the pipeline shared across
	// every phase. HasErrors / Diagnostics give per-test
	// visibility.
	Diag *diag.Sink

	// Store is the in-memory store the pipeline populated. Nil
	// when the pipeline never reached the Run path (e.g. Build
	// failed); tests should check for nil before dereferencing.
	Store *store.Store

	// RunErr is the error returned by [pipeline.Pipeline.Run].
	// Typically [pipeline.ErrRunHadErrors] when any plugin
	// emitted an Error diagnostic; nil otherwise.
	RunErr error
}

// Run builds and runs a pipeline driven by opts.Frontend against
// the supplied source dir and returns the captured outputs.
// Build failures surface as a fatal test error — the harness is
// for tests that assume the configured plugin set is valid.
//
// When [RunOptions.Backend] is nil, the harness registers a
// minimal no-op backend that satisfies the pipeline's build
// invariants without emitting anything; the returned Sink is
// the caller's (or a default empty one).
//
// A [RunOptions.SourceDir] that is not an existing directory is a
// fatal test error, raised before the pipeline is built. Left
// through, the frontend turns the same path into Error
// diagnostics and Run returns them in [Result], where a test that
// asserts nothing about the store passes over an empty one.
func Run(t *testing.T, opts RunOptions) Result {
	t.Helper()
	if opts.Frontend == nil {
		t.Fatalf("frontendtest.Run: opts.Frontend is required")
	}
	if opts.SourceDir == "" {
		t.Fatalf("frontendtest.Run: opts.SourceDir is required")
	}
	if err := sourceDirError(opts.SourceDir); err != nil {
		t.Fatalf("frontendtest.Run: %v; set opts.SourceDir to a directory the frontend "+
			"can load, such as your own testdata tree", err)
	}
	if opts.Sink == nil {
		opts.Sink = sink.NewMemory()
	}
	if opts.Cache == nil {
		opts.Cache = cache.NewNone()
	}
	pattern := opts.Pattern
	if pattern == "" {
		pattern = "./..."
	}
	backend := opts.Backend
	if backend == nil {
		backend = &noopBackend{}
	}

	feOpts := frontendOptionsFor(opts)

	b := pipeline.New().
		WithFrontend(opts.Frontend).
		WithBackend(backend).
		WithSink(opts.Sink).
		WithCache(opts.Cache).
		WithPluginOptions(opts.Frontend.Name(), feOpts)
	if opts.Layout != "" {
		b = b.WithOutputLayout(opts.Layout)
	}
	if opts.OutputPackage != "" {
		b = b.WithOutputPackage(opts.OutputPackage)
	}
	if opts.OutputDir != "" {
		b = b.WithOutputDir(opts.OutputDir)
	}
	if opts.TargetSymbol != "" {
		b = b.WithTargetSymbol(opts.TargetSymbol)
	}
	if opts.Command != "" {
		b = b.WithCommand(opts.Command)
	}
	if opts.ManifestPath != "" {
		b = b.WithManifestPath(opts.ManifestPath)
	}
	if opts.SourceRoot != "" {
		b = b.WithSourceRoot(opts.SourceRoot)
	}
	for _, a := range opts.Annotators {
		b = b.WithAnnotator(a)
	}
	for _, g := range opts.Generators {
		b = b.WithGenerator(g)
	}
	for name, kv := range opts.PluginOptions {
		if name == opts.Frontend.Name() {
			continue
		}
		b = b.WithPluginOptions(name, kv)
	}

	p, err := b.Build()
	if err != nil {
		t.Fatalf("frontendtest.Run: pipeline.Build: %v", err)
	}
	runErr := p.Run(t.Context(), pattern)
	return Result{
		Sink:   opts.Sink,
		Diag:   p.Diag(),
		Store:  p.Store(),
		RunErr: runErr,
	}
}

// LoadDirect drives opts.Frontend's [plugin.Frontend.Load]
// surface against the source dir without building the pipeline.
// Tests covering frontend-only diagnostics (parse-error
// rejection, source-mapping shape) call LoadDirect to skip the
// pipeline-build invariants; tests exercising annotator /
// generator interaction call [Run] instead.
//
// The returned [diag.Sink] captures every diagnostic the
// frontend produced; [Result.Store] carries the produced node
// graph. [Result.RunErr] is always nil — LoadDirect calls the
// frontend's Load method directly rather than driving a
// pipeline, so there's no pipeline run error to surface.
//
// Unlike [Run], LoadDirect does not stat [RunOptions.SourceDir].
// It hands the value to the frontend, and the frontend's own Load
// error is what fails the test — so a frontend that never reads
// the filesystem accepts a path to nothing here.
func LoadDirect(t *testing.T, opts RunOptions) Result {
	t.Helper()
	if opts.Frontend == nil {
		t.Fatalf("frontendtest.LoadDirect: opts.Frontend is required")
	}
	if opts.SourceDir == "" {
		t.Fatalf("frontendtest.LoadDirect: opts.SourceDir is required")
	}
	if opts.Sink == nil {
		opts.Sink = sink.NewMemory()
	}
	if opts.Cache == nil {
		opts.Cache = cache.NewNone()
	}
	pattern := opts.Pattern
	if pattern == "" {
		pattern = "./..."
	}
	d := diag.New()
	s := store.New()
	provider, ok := any(opts.Frontend).(plugin.OptionsProvider)
	if ok {
		if err := provider.SetOptions(opt.New(provider.OptionsSchema(), frontendOptionsFor(opts))); err != nil {
			t.Fatalf("frontendtest.LoadDirect: SetOptions: %v", err)
		}
	}
	ctx := &plugin.FrontendContext{
		Store:    s,
		Diag:     d,
		Registry: directive.NewRegistry(),
		Parser:   directive.DefaultParser(),
		Cache:    opts.Cache,
		Pattern:  pattern,
	}
	if err := opts.Frontend.Load(ctx); err != nil {
		t.Fatalf("frontendtest.LoadDirect: Load: %v", err)
	}
	return Result{Sink: opts.Sink, Diag: d, Store: s}
}

// frontendOptionsFor merges the harness's default options
// (`dir = <SourceDir>`) with caller-supplied overrides. The
// merge precedence (last writer wins) is:
//
//  1. The harness's `dir = <SourceDir>` default.
//  2. Caller-supplied [RunOptions.FrontendOptions].
//  3. Caller-supplied [RunOptions.PluginOptions] keyed by the
//     frontend's plugin name — the generic per-plugin override
//     path wins over the frontend-specific shortcut.
//
// The returned map is a fresh allocation; mutating it does not
// affect the caller's [RunOptions].
func frontendOptionsFor(opts RunOptions) map[string]string {
	out := map[string]string{"dir": opts.SourceDir}
	maps.Copy(out, opts.FrontendOptions)
	maps.Copy(out, opts.PluginOptions[opts.Frontend.Name()])
	return out
}

// noopBackend satisfies the pipeline's build invariants without
// emitting anything. Used internally by [Run] when the caller
// supplies no backend.
type noopBackend struct{}

// Name returns the no-op backend's identifier.
func (*noopBackend) Name() string { return "frontendtest.noop" }

// Language returns the framework-neutral language identifier the
// no-op backend declares.
func (*noopBackend) Language() string { return "noop" }

// Render is a no-op — the pipeline runs frontend / annotator /
// generator phases and the no-op backend writes nothing.
func (*noopBackend) Render(_ *plugin.BackendContext) error { return nil }
