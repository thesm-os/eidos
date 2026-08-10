// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import "go.thesmos.sh/eidos/core/diag"

// Diagnostics — how a plugin reports a problem instead of
// panicking, failing silently, or emitting something wrong.

// Sink is the run-wide diagnostic collector every phase context
// carries as its Diag field. A plugin does not emit through it
// directly: call `ctx.Diag.For(Name)` once and emit through the
// returned [PluginSink], so every diagnostic is attributed
// without the collector guessing from the call stack.
type Sink = diag.Sink

// PluginSink is a [Sink] view that stamps a fixed plugin name on
// everything it emits. Safe to share across goroutines for the
// span of one plugin invocation, which is what lets a generator
// fan out over declarations and still report coherently.
type PluginSink = diag.PluginSink

// Diag is one positioned diagnostic. Named by plugins that
// inspect a captured run rather than by ones that only emit —
// emission goes through [PluginSink]'s Infof / Warnf / Errorf.
type Diag = diag.Diag

// Severity ranks a diagnostic. The rank is load-bearing, not
// decoration: an Error blocks output for the whole run, so a
// generator that reports a per-declaration problem at Error
// stops every other declaration from being written.
type Severity = diag.Severity

// The severity levels, prefixed.
//
// The bare spellings would collide — `Internal` is already the
// emit-side ref constructor, and a bare `Error` in a Go package
// reads as an error type rather than a level. Both halves of that
// confusion compile, which is exactly the kind of mistake the
// prefix is here to make impossible.
const (
	// SeverityInfo is informational, hidden unless asked for.
	SeverityInfo = diag.Info

	// SeverityWarn flags something probably wrong that still
	// produces output.
	SeverityWarn = diag.Warn

	// SeverityError blocks output for the run. Reserve it for a
	// problem that makes the generated result wrong, not merely
	// incomplete.
	SeverityError = diag.Error

	// SeverityInternal reports a framework or plugin bug and maps
	// to its own CLI exit code, so an operator can route the
	// report to whoever can fix it. Plugin code emits Error; the
	// pipeline emits this on a recovered panic.
	SeverityInternal = diag.Internal
)

// NewSink returns a capturing diagnostic sink — [diag.New].
//
// A plugin never makes one: the pipeline hands it a [Sink] on every
// phase context. A plugin's *tests* do, and the type was reachable
// here while the constructor was not, so a test asserting on what a
// plugin reported imported `core/diag` for one call and reached past
// the façade this package's own doc tells plugin authors to stay
// inside.
//
// [eidostest/plugintest.Annotate] and its siblings drive a plugin and
// return the sink already, which is the shorter path for a test whose
// subject is what the plugin reported. This is for a test assembling
// a context itself.
//
//nolint:gochecknoglobals // alias re-export of a stable factory.
var NewSink = diag.New
