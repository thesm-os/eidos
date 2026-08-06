// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipelinetest

import (
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
)

// Builder is the test-tuned analogue of [pipeline.Builder]. It
// forwards every registration call into the underlying pipeline
// builder and seeds shared defaults so that the typical test path is
// "register plugins, Build, Run" — no manual sink or diag wiring.
//
// Defaults applied at New time:
//
//   - A fresh [sink.Memory] is supplied as the destination sink. The
//     captured bytes drive every per-file assertion the [Pipeline]
//     exposes. Tests that need a different sink may call
//     [Builder.WithSink] to override; in that case file-level
//     assertions become unavailable for files routed away from the
//     memory sink.
//   - A fresh [diag.Sink] is supplied so every test starts from a
//     clean diagnostic state.
//   - The cache defaults to [cache.NewNone] — incremental caching
//     is irrelevant for in-process tests and disabling it keeps
//     assertions hermetic.
//   - The rendered "Command:" header is pinned to [DefaultCommand].
//     Left unpinned the pipeline renders os.Args, which under
//     `go test` is the test binary's own flag set — including the
//     per-machine `-test.testlogfile` path and, fatally for golden
//     files, the `-update-golden` flag itself. Callers who want the
//     os.Args rendering override through [Builder.WithCommand].
//
// SourceRoot is deliberately NOT defaulted: [pipeline.Builder]
// reads the empty string as "unset" and falls back to os.Getwd, so
// there is no value that means "no source root". Tests whose
// fixtures carry absolute origin paths pin it themselves through
// [Builder.WithSourceRoot].
type Builder struct {
	t     testing.TB
	inner *pipeline.Builder
	sink  *sink.Memory
	diag  *diag.Sink
}

// DefaultCommand is the "Command:" header line every harness-built
// pipeline stamps unless the caller overrides it. The parenthesised
// spelling matches the framework's own "(library)" sentinel for
// programmatic invocations: a reader of a golden file can tell at a
// glance that no real command line produced it.
const DefaultCommand = "(test)"

// New starts a Builder bound to tb. Build- and run-time failures
// call tb.Fatalf, so callers do not need to thread error handling
// through every helper invocation.
func New(tb testing.TB) *Builder {
	tb.Helper()
	mem := sink.NewMemory()
	d := diag.New()
	return &Builder{
		t: tb,
		inner: pipeline.New().
			WithSink(mem).
			WithDiag(d).
			WithCache(cache.NewNone()).
			WithCommand(DefaultCommand),
		sink: mem,
		diag: d,
	}
}

// WithFrontend registers a frontend on the underlying pipeline.
func (b *Builder) WithFrontend(p plugin.Frontend) *Builder {
	b.inner.WithFrontend(p)
	return b
}

// WithAnnotator registers an annotator on the underlying pipeline.
func (b *Builder) WithAnnotator(p plugin.Annotator) *Builder {
	b.inner.WithAnnotator(p)
	return b
}

// WithGenerator registers a generator on the underlying pipeline.
func (b *Builder) WithGenerator(p plugin.Generator) *Builder {
	b.inner.WithGenerator(p)
	return b
}

// WithBackend registers the pipeline's backend.
func (b *Builder) WithBackend(p plugin.Backend) *Builder {
	b.inner.WithBackend(p)
	return b
}

// WithDirective registers one or more directive schemas with the
// pipeline's directive registry. Mirrors [pipeline.Builder.WithDirective].
func (b *Builder) WithDirective(schemas ...directive.Schema) *Builder {
	b.inner.WithDirective(schemas...)
	return b
}

// WithPluginOptions supplies typed options for the named plugin.
// Mirrors [pipeline.Builder.WithPluginOptions].
func (b *Builder) WithPluginOptions(name string, kv map[string]string) *Builder {
	b.inner.WithPluginOptions(name, kv)
	return b
}

// WithSink replaces the default in-memory sink. After WithSink the
// per-file assertion methods on [Pipeline] cannot observe files
// routed away from the memory sink; tests that need both real
// destinations and assertions should combine the user sink with the
// default memory sink via [sink.NewMulti].
func (b *Builder) WithSink(s sink.Sink) *Builder {
	b.inner.WithSink(s)
	return b
}

// WithCache replaces the default cache.
func (b *Builder) WithCache(c cache.Cache) *Builder {
	b.inner.WithCache(c)
	return b
}

// WithVerbose enables verbose-mode diagnostics for the underlying
// pipeline.
func (b *Builder) WithVerbose(v bool) *Builder {
	b.inner.WithVerbose(v)
	return b
}

// WithParallel opts the underlying pipeline's phases into parallel
// execution.
func (b *Builder) WithParallel(phases ...pipeline.Phase) *Builder {
	b.inner.WithParallel(phases...)
	return b
}

// WithCommand pins the literal text the backend stamps into the
// "Command:" header line, overriding the harness default of
// [DefaultCommand]. Pass the empty string to restore the pipeline's
// production behaviour of rendering os.Args — under `go test` that
// makes the header vary with the flags the binary was invoked
// under, so a golden asserted against it cannot round-trip. Mirrors
// [pipeline.Builder.WithCommand].
func (b *Builder) WithCommand(cmd string) *Builder {
	b.inner.WithCommand(cmd)
	return b
}

// WithSourceRoot sets the base directory the backend renders
// "Source:" header paths relative to. Unset, the underlying builder
// resolves it from os.Getwd — the test binary's package directory,
// which is machine-specific. Tests whose fixture nodes carry
// absolute origin paths pin it here so the rendered header is
// portable. Mirrors [pipeline.Builder.WithSourceRoot]; the empty
// string reads as unset rather than as "no source root".
func (b *Builder) WithSourceRoot(root string) *Builder {
	b.inner.WithSourceRoot(root)
	return b
}

// WithBrand pins the tool identifier stamped into the generated-file
// marker and the provenance footer. Mirrors
// [pipeline.Builder.WithBrand].
func (b *Builder) WithBrand(brand string) *Builder {
	b.inner.WithBrand(brand)
	return b
}

// WithOutputLayout overrides the layout policy for the run. Mirrors
// [pipeline.Builder.WithOutputLayout].
func (b *Builder) WithOutputLayout(layout string) *Builder {
	b.inner.WithOutputLayout(layout)
	return b
}

// WithOutputDir sets the rendered output directory under centralised
// layout. Mirrors [pipeline.Builder.WithOutputDir]; ignored under
// alongside-source layout, which derives the directory from origin.
func (b *Builder) WithOutputDir(dir string) *Builder {
	b.inner.WithOutputDir(dir)
	return b
}

// WithOutputPackage pins [emit.Target.Package] for every emitted
// decl in scope. Mirrors [pipeline.Builder.WithOutputPackage].
func (b *Builder) WithOutputPackage(pkg string) *Builder {
	b.inner.WithOutputPackage(pkg)
	return b
}

// WithOutputFilename pins [emit.Target.Filename] for every emitted
// decl in scope. Mirrors [pipeline.Builder.WithOutputFilename].
//
// The unscoped form is a trap on a multi-output plugin and no
// sentinel guards it: two declared outputs both take the pinned
// name, land on one target, and the second silently overwrites the
// first — one rendered file, no diagnostic, no error. Scope it with
// [Builder.WithTargetSymbol], or reach for the per-(plugin, tag)
// form in [Builder.WithPluginOutputFilename] instead.
func (b *Builder) WithOutputFilename(name string) *Builder {
	b.inner.WithOutputFilename(name)
	return b
}

// WithPluginOutputFilename pins [emit.Target.Filename] for the decls
// the named plugin emits into the output identified by tag; tag is
// empty for the plugin's primary output. Mirrors
// [pipeline.Builder.WithPluginOutputFilename], and is the scoped
// alternative to [Builder.WithOutputFilename] on a multi-output
// plugin.
func (b *Builder) WithPluginOutputFilename(name, tag, path string) *Builder {
	b.inner.WithPluginOutputFilename(name, tag, path)
	return b
}

// WithProjectOutput supplies the project-level routing triple —
// the `output.*` block on `.eidos.yaml`. Mirrors
// [pipeline.Builder.WithProjectOutput].
func (b *Builder) WithProjectOutput(layout, pkg, dir string) *Builder {
	b.inner.WithProjectOutput(layout, pkg, dir)
	return b
}

// WithPluginOutput supplies a per-plugin routing override — the
// `plugins[*].output.*` block on `.eidos.yaml`. Mirrors
// [pipeline.Builder.WithPluginOutput].
func (b *Builder) WithPluginOutput(name, layout, pkg, dir string) *Builder {
	b.inner.WithPluginOutput(name, layout, pkg, dir)
	return b
}

// WithPluginTagOutput supplies a per-(plugin, tag) routing override
// — the `plugins[*].output.tags.<tag>.*` block on `.eidos.yaml`.
// Mirrors [pipeline.Builder.WithPluginTagOutput]. This is the only
// way to express a legitimate shared filename suffix across two of
// a plugin's outputs: the per-tag directory is what keeps them from
// colliding.
func (b *Builder) WithPluginTagOutput(name, tag, layout, pkg, dir string) *Builder {
	b.inner.WithPluginTagOutput(name, tag, layout, pkg, dir)
	return b
}

// WithTargetSymbol narrows the run to source decls whose unqualified
// name equals symbol. Mirrors [pipeline.Builder.WithTargetSymbol].
func (b *Builder) WithTargetSymbol(symbol string) *Builder {
	b.inner.WithTargetSymbol(symbol)
	return b
}

// WithDirectivePrefix overrides the directive prefix the pipeline's
// parser recognises. Mirrors [pipeline.Builder.WithDirectivePrefix];
// the empty string is rejected at Build time.
func (b *Builder) WithDirectivePrefix(prefix string) *Builder {
	b.inner.WithDirectivePrefix(prefix)
	return b
}

// WithManifestPath configures where the run writes its manifest, and
// so is what makes [Pipeline.Manifest] non-nil. Mirrors
// [pipeline.Builder.WithManifestPath].
//
// This option writes to disk from a test. Point it at
// [testing.T.TempDir], or pair it with WithDryRun(true), which
// populates the in-memory manifest and skips the write entirely —
// the combination a test asserting on attribution wants. A
// repo-relative path dirties the working tree from a unit test.
func (b *Builder) WithManifestPath(path string) *Builder {
	b.inner.WithManifestPath(path)
	return b
}

// WithDryRun runs every phase but does not persist the manifest.
// Mirrors [pipeline.Builder.WithDryRun]; the harness's memory sink
// already keeps rendered output off disk, so dry run plus
// [Builder.WithManifestPath] is a fully hermetic way to inspect
// what a run would have recorded.
func (b *Builder) WithDryRun(dryRun bool) *Builder {
	b.inner.WithDryRun(dryRun)
	return b
}

// WithPipelineID pins the identifier stamped on every manifest
// output this pipeline produces. Mirrors
// [pipeline.Builder.WithPipelineID].
func (b *Builder) WithPipelineID(id string) *Builder {
	b.inner.WithPipelineID(id)
	return b
}

// Build finalises the underlying pipeline. On any Build error the
// method calls t.Fatalf — tests rarely need to assert against
// builder validation errors, so the default is to fail loudly.
// Tests that do want the error inspect it via [Builder.BuildErr].
func (b *Builder) Build() *Pipeline {
	b.t.Helper()
	p, err := b.BuildErr()
	if err != nil {
		b.t.Fatalf("testpipe: build failed: %v", err)
	}
	return p
}

// BuildErr finalises the underlying pipeline and returns the
// validation error instead of failing the test, so a composition
// test can assert against [pipeline.ErrInvalidOutputs],
// [pipeline.ErrDuplicateDirective] and the rest of the Build-time
// sentinels without leaving the harness.
//
// The returned Pipeline is nil whenever the error is non-nil —
// callers must check the error before dereferencing. [Builder.Build]
// is the ergonomic default; reach for BuildErr only when the
// validation failure is the behaviour under test.
func (b *Builder) BuildErr() (*Pipeline, error) {
	b.t.Helper()
	inner, err := b.inner.Build()
	if err != nil {
		return nil, err
	}
	return &Pipeline{
		t:     b.t,
		inner: inner,
		sink:  b.sink,
		diag:  b.diag,
	}, nil
}
