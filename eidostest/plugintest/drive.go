// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"errors"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// Drivers for a targeted test of one plugin.
//
// The conformance suites answer "is this a well-formed plugin". They
// do not answer "what did it do to this store", which is the question
// a plugin's own tests are mostly about — and answering it meant
// building a [plugin.AnnotatorContext] or [plugin.GeneratorContext] by
// hand, reaching past the façade for [store.NewReader] and [diag]
// while `sdk/doc.go` tells plugin authors to route exactly those
// through `eidostest`.
//
// The bodies already existed here, unexported, behind the suites.

// Annotate runs a's Annotate against s and returns what it reported.
//
// A panic fails tb naming the plugin rather than taking the test
// binary down, which is the difference between one red test and a run
// with no results. A returned error fails tb too: a test asserting on
// the error path holds the plugin directly, and treating an error as
// an ordinary outcome here would let a plugin that did nothing pass
// every assertion made about the store afterwards.
//
// The returned sink is a capturing one, so a caller reads
// [diag.Sink.Diagnostics] to assert on positions and messages.
func Annotate(tb testing.TB, a plugin.Annotator, s *store.Store) *diag.Sink {
	tb.Helper()
	d := diag.Capture()
	reportDriveFailure(tb, "Annotate", nameOf(a), runAnnotateRecovering(a, s, d))
	return d
}

// Generate runs g's Generate against s and returns what it reported.
//
// The generator side of [Annotate], with the same failure contract. A
// generator's own tests then assert on the emit store the run
// populated and on the diagnostics it wrote, which together are what
// the plugin is for.
func Generate(tb testing.TB, g plugin.Generator, s *store.Store) *diag.Sink {
	tb.Helper()
	d := diag.Capture()
	reportDriveFailure(tb, "Generate", nameOf(g),
		runGenerateWithReader(g, s, store.NewReader(s), d))
	return d
}

// GenerateWithReader is [Generate] with a caller-supplied reader, so a
// test can inspect what the generator read.
//
// The reads a generator records are its cache key. A plugin that stops
// recording one keeps producing correct output until a warm cache
// serves a stale file, and the reader is the only place that is
// visible before it happens.
func GenerateWithReader(
	tb testing.TB, g plugin.Generator, s *store.Store, r *store.Reader,
) *diag.Sink {
	tb.Helper()
	d := diag.Capture()
	reportDriveFailure(tb, "Generate", nameOf(g), runGenerateWithReader(g, s, r, d))
	return d
}

// reportDriveFailure turns a driver's error into a test failure that
// says which plugin, which hook, and which of the two failure kinds.
//
// The two are not the same defect: a returned error is the plugin
// declining and is usually the test's own fixture being wrong, while a
// panic is the plugin being wrong and is worth naming as such.
func reportDriveFailure(tb testing.TB, hook, name string, err error) {
	tb.Helper()
	switch {
	case err == nil:
		return
	case errors.Is(err, ErrProbePanicked):
		tb.Fatalf("plugintest: %s panicked in %s: %v", name, hook, err)
	default:
		tb.Fatalf("plugintest: %s returned an error from %s: %v\n"+
			"a test asserting on the error path drives the plugin directly, "+
			"because an error here means nothing downstream of it ran", name, hook, err)
	}
}

// nameOf returns a plugin's declared name, or a placeholder when it
// declares none — so a failure message never reads "" panicked.
func nameOf(p plugin.Plugin) string {
	if p == nil {
		return "<nil plugin>"
	}
	if n := p.Name(); n != "" {
		return n
	}
	return "<unnamed plugin>"
}
