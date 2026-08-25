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
// [pipeline.Builder.WithPluginOptions], when reading the frontend's
// diagnostics out of [diag.Sink.Diagnostics], and when filtering
// store-side nodes by the `frontend = "protobuf"` provenance
// marker stamped on every produced [node.Package].
const FrontendName = "protobuf"

// Frontend is the proto3 frontend. The zero value is unusable;
// construct via [New].
//
// Frontend instances are safe for concurrent use. Load can be
// invoked from multiple goroutines (one per pattern, dispatched by
// the pipeline when frontend-phase parallelism is enabled); the
// underlying protocompile compiler is concurrency-safe per its
// documented contract, and writes to the shared [store.Store]
// serialize on per-package locks.
type Frontend struct {
	mu   sync.Mutex
	opts Options
}

// New returns a protobuf frontend ready for registration on a
// [pipeline.Builder]. The default [Options] (returned by
// [defaultOptions]) apply until the pipeline calls
// [Frontend.SetOptions] — `IncludeWellKnown = true` and an empty
// [Dir] / [ImportPaths] in particular hold under the zero-config
// path.
func New() *Frontend {
	return &Frontend{opts: defaultOptions()}
}

// Name returns [FrontendName].
func (*Frontend) Name() string { return FrontendName }

// Version returns [FrontendVersion]. The string composes into the
// frontend's per-plugin cache key so a version bump invalidates
// the frontend's cached parses without disturbing other plugins'
// caches.
func (*Frontend) Version() string { return FrontendVersion }

// OptionsSchema returns the reflected [opt.Schema] describing the
// frontend's configurable options. The pipeline calls this at
// Build time to validate caller-supplied values; consumers don't
// invoke it directly.
func (*Frontend) OptionsSchema() opt.Schema { return optionsSchema }

// SetOptions decodes the supplied options into the frontend's
// internal [Options]. Called by the pipeline at Build time;
// consumers don't invoke it directly.
//
// Decoding is mutex-guarded so concurrent SetOptions calls from a
// future builder-level parallelism don't race; a typical pipeline
// calls SetOptions exactly once per Frontend instance.
func (f *Frontend) SetOptions(o opt.Options) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return o.Decode(&f.opts)
}

// Load implements [plugin.Frontend]. Calling Load against an
// empty Pattern is a no-op that returns nil; a supplied Pattern
// routes through protocompile's compiler and surfaces parse /
// resolution errors via the frontend's per-plugin diagnostic
// sink (`ctx.Diag.For(FrontendName)`). The resolved descriptor
// set composes into the per-plugin cache key so re-runs against
// unchanged inputs short-circuit through the configured cache.
func (f *Frontend) Load(ctx *plugin.FrontendContext) error {
	opts := f.snapshotOptions()
	return loadPattern(ctx, opts)
}

// Dir returns the configured proto-source search root. Tests
// and consumers read this through the accessor rather than the
// struct field so the public API stays stable while the internal
// options-storage shape can evolve.
func (f *Frontend) Dir() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opts.Dir
}

// ImportPaths returns the configured additional proto-import
// search roots as a typed slice. The underlying option storage
// is a comma-separated string for portability through
// `.eidos.yaml`; ImportPaths splits and trims on each call so
// callers don't deal with parsing.
func (f *Frontend) ImportPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return importPathList(f.opts.ImportPaths)
}

// IncludeWellKnown returns whether protocompile's bundled
// well-known descriptors are registered for the compiler's
// import resolution.
func (f *Frontend) IncludeWellKnown() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opts.IncludeWellKnown
}

// snapshotOptions returns a copy of the current options under the
// receiver's mutex. Load uses this to read a consistent view
// without holding the lock for the duration of a full compile.
func (f *Frontend) snapshotOptions() Options {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opts
}
