// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipelinetest_test

import (
	"fmt"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// loadCtx builds a [plugin.FrontendContext] referencing s plus a
// discard diagnostic sink. Tests call frontends' Load via this
// helper without rebuilding the context boilerplate per call.
func loadCtx(s *store.Store, pattern string) *plugin.FrontendContext {
	return &plugin.FrontendContext{Store: s, Diag: diag.Discard(), Pattern: pattern}
}

// fakeT is a [testing.TB] adapter used by tests that need to assert
// against the test-failure side of [pipelinetest.Pipeline] without
// failing the surrounding go-test invocation. fakeT records errors
// and fatals into in-memory slices; Helper is a no-op.
type fakeT struct {
	testing.TB
	errs   []string
	fatals []string
	failed bool
}

// newFakeT returns a fresh fake TB.
func newFakeT() *fakeT { return &fakeT{} }

// Errorf records the formatted message and marks the fake as failed
// without aborting the test.
func (f *fakeT) Errorf(format string, args ...any) {
	f.errs = append(f.errs, fmt.Sprintf(format, args...))
	f.failed = true
}

// Fatalf records the formatted message and panics with sentinel
// [fatalSentinel] so callers can recover and continue asserting in
// the surrounding real test. Mirrors how testing.TB short-circuits
// on Fatal in production.
func (f *fakeT) Fatalf(format string, args ...any) {
	f.fatals = append(f.fatals, fmt.Sprintf(format, args...))
	f.failed = true
	panic(fatalSentinel{})
}

// Helper is a no-op; fakeT does not adjust file:line reporting.
func (*fakeT) Helper() {}

// Failed reports whether any error or fatal has been recorded.
func (f *fakeT) Failed() bool { return f.failed }

// fatalSentinel is the panic payload [fakeT.Fatalf] uses so callers
// can recover deterministically without conflating with real test
// panics.
type fatalSentinel struct{}

// captureFatal runs fn and reports whether it called [fakeT.Fatalf]
// during execution. The fake's recorded messages remain available
// for assertion after captureFatal returns.
func captureFatal(fn func()) (called bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(fatalSentinel); ok {
				called = true
				return
			}
			panic(r)
		}
	}()
	fn()
	return false
}

// exampleTB adapts the package's [testing.TB]-driven harness to an
// [Example] function, which the testing package gives no TB of its
// own.
//
// Failures print to stdout rather than being recorded, and that is
// the point: an example is verified by its `// Output:` block, so a
// harness regression that would otherwise be swallowed shows up as
// extra lines the block does not expect and the example fails. A
// silent fake would turn the example into a compile check only.
//
// Fatalf panics with [fatalSentinel] after printing. [Builder.Build]
// and [Pipeline.Run] call it on unrecoverable failures and expect
// not to return; continuing with a nil pipeline would report a
// confusing nil dereference instead of the real cause.
type exampleTB struct{ testing.TB }

// Errorf prints the failure and lets the example continue, matching
// the non-fatal semantics of the real [testing.TB].
func (*exampleTB) Errorf(format string, args ...any) {
	fmt.Printf("FAILED: "+format+"\n", args...)
}

// Fatalf prints the failure and unwinds; see [exampleTB].
func (*exampleTB) Fatalf(format string, args ...any) {
	fmt.Printf("FATAL: "+format+"\n", args...)
	panic(fatalSentinel{}) //nolint:forbidigo // test-only harness adapter; mirrors testing.TB's abort semantics
}

// Helper is a no-op; exampleTB does not adjust file:line reporting.
func (*exampleTB) Helper() {}

// stubBackend is a backend whose Render writes a fixed set of
// (target → body) entries to ctx.Sink. Tests use it to seed captured
// output without going through a real templating backend.
type stubBackend struct {
	name   string
	lang   string
	writes map[emit.Target][]byte
}

// Name returns the configured plugin name.
func (b *stubBackend) Name() string { return b.name }

// Language returns the configured target-language identifier.
func (b *stubBackend) Language() string { return b.lang }

// Render writes every configured (target, body) entry to ctx.Sink.
// Map iteration order is undefined, but [sink.Memory] tolerates that
// because its assertions consume by target, not by call order.
func (b *stubBackend) Render(ctx *plugin.BackendContext) error {
	for tgt, body := range b.writes {
		if err := ctx.Sink.Write(tgt, body); err != nil {
			return err
		}
	}
	return nil
}

// stubGenerator is a generator that does nothing on Generate. Used
// when a test only needs a generator phase present but inert.
type stubGenerator struct{ name string }

// Name returns the configured plugin name.
func (g *stubGenerator) Name() string { return g.name }

// Generate is a no-op.
func (*stubGenerator) Generate(_ *plugin.GeneratorContext) error { return nil }

// stubAnnotator is an annotator that does nothing on Annotate. Used
// when a test only needs an annotator phase present but inert.
type stubAnnotator struct{ name string }

// Name returns the configured plugin name.
func (a *stubAnnotator) Name() string { return a.name }

// Annotate is a no-op.
func (*stubAnnotator) Annotate(_ *plugin.AnnotatorContext) error { return nil }

// erroringFrontend is a frontend whose Load returns a fixed error.
// Used to trigger the ErrRunHadErrors branch of [Pipeline.Run] via
// an emitted error diagnostic, not via the run-level fatal path.
type erroringFrontend struct {
	name string
	err  error
}

// Name returns the configured plugin name.
func (f *erroringFrontend) Name() string { return f.name }

// Load returns the configured error verbatim.
func (f *erroringFrontend) Load(_ *plugin.FrontendContext) error { return f.err }
