// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"sync"

	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/plugin"
)

// FrontendName is the [plugin.Plugin.Name] this frontend reports.
// Consumers reference it when supplying options via
// [pipeline.Builder.WithPluginOptions] or when reading the
// frontend's diagnostics out of [diag.Sink.Diagnostics].
const FrontendName = "golang"

// Frontend is the Go-language frontend. The zero value is unusable;
// construct via [New].
//
// Frontend instances are safe for concurrent use. Load can be
// invoked from multiple goroutines (one per pattern, dispatched by
// [pipeline.Pipeline] when [pipeline.Builder.WithParallel] enables
// frontend parallelism); the underlying [golang.org/x/tools/go/packages]
// loader is concurrent-safe, and writes to the shared [store.Store]
// serialize on per-package locks.
type Frontend struct {
	mu   sync.Mutex
	opts Options
}

// New returns a Go frontend ready for registration on a
// [pipeline.Builder]. The default [Options] (returned by
// [defaultOptions]) apply until the pipeline calls
// [Frontend.SetOptions] — the documented "default true" booleans
// (SkipCgoFiles, SkipGeneratedFiles) therefore hold even when no
// option overrides reach the frontend.
func New() *Frontend {
	return &Frontend{opts: defaultOptions()}
}

// defaultOptions returns the [Options] value the frontend uses when
// no overrides are configured. Mirrors the `default=…` tags on the
// [Options] struct one-for-one; the small amount of duplication is
// the trade-off for keeping [New] panic-free and side-effect-free.
func defaultOptions() Options {
	return Options{
		IncludeTests:       false,
		BuildTags:          "",
		SkipCgoFiles:       true,
		SkipGeneratedFiles: true,
		Dir:                "",
	}
}

// Name returns [FrontendName].
func (*Frontend) Name() string { return FrontendName }

// Version returns [FrontendVersion]. Composed into the cache key
// for every package the frontend produces; bumping the constant
// invalidates frontend-stage cache entries.
func (*Frontend) Version() string { return FrontendVersion }

// EmitVersions reports the emit major versions this frontend is
// compatible with. The frontend never produces emit values but
// participates in the pipeline's emit-compatibility check so users
// see a positioned diagnostic if they pair the frontend with an
// incompatible emit major.
func (*Frontend) EmitVersions() []string {
	out := make([]string, len(supportedEmitVersions))
	copy(out, supportedEmitVersions)
	return out
}

// OptionsSchema returns the reflected [opt.Schema] describing the
// frontend's configurable options.
func (*Frontend) OptionsSchema() opt.Schema { return optionsSchema }

// SetOptions decodes the supplied options into the frontend's
// internal Options. Called by the pipeline at Build time; consumers
// don't invoke it directly.
func (f *Frontend) SetOptions(o opt.Options) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return o.Decode(&f.opts)
}

// Load implements [plugin.Frontend]. It snapshots the current
// options under the receiver's mutex and delegates to [loadPattern],
// which expands the supplied [plugin.FrontendContext.Pattern] through
// [golang.org/x/tools/go/packages], converts each package's AST into
// [node] entities, stamps `go.*` metadata, and writes the result
// into the shared [store.Store]. Per-package errors are recorded as
// diagnostics on [plugin.FrontendContext.Diag] rather than aborting
// the run.
func (f *Frontend) Load(ctx *plugin.FrontendContext) error {
	return loadPattern(ctx, f.snapshotOptions())
}

// snapshotOptions returns a copy of the frontend's current Options
// under the receiver's mutex so concurrent Load calls see a stable
// view of configuration even if SetOptions runs in parallel.
func (f *Frontend) snapshotOptions() Options {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opts
}
