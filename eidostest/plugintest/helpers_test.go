// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest_test

import (
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"testing"
	"text/template"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/priority"
	"go.thesmos.sh/eidos/store"
)

// fakeT is a [testing.TB] adapter used by tests that need to
// assert against the test-failure side of the conformance suites
// without failing the surrounding `go test` invocation. fakeT
// records errors and fatals into in-memory slices; [Helper] is a
// no-op so file:line attribution stays at the call site.
type fakeT struct {
	testing.TB
	errs   []string
	fatals []string
	skips  []string
	logs   []string
	failed bool
}

// newFakeT returns a fresh fake TB. Its embedded [testing.TB] is
// nil, so it answers only the four methods below; checks that reach
// for anything else on the interface need [newFakeTIn].
func newFakeT() *fakeT { return &fakeT{} }

// newFakeTIn returns a fake TB that delegates everything it does not
// override to tb. The frontend suite's cache checks call
// [testing.TB.TempDir], which the zero fakeT cannot answer — and the
// delegated directory is cleaned up with tb rather than leaking into
// the temp root.
func newFakeTIn(tb testing.TB) *fakeT {
	tb.Helper()
	return &fakeT{TB: tb}
}

// Errorf records the formatted message and marks the fake as
// failed without aborting the test.
func (f *fakeT) Errorf(format string, args ...any) {
	f.errs = append(f.errs, fmt.Sprintf(format, args...))
	f.failed = true
}

// Fatalf records the formatted message and panics with the
// sentinel [fatalSentinel] so callers can recover and continue
// asserting in the surrounding real test. Mirrors how
// [testing.TB] short-circuits on Fatal in production.
func (f *fakeT) Fatalf(format string, args ...any) {
	f.fatals = append(f.fatals, fmt.Sprintf(format, args...))
	f.failed = true
	panic(fatalSentinel{})
}

// Helper is a no-op; fakeT does not adjust file:line reporting.
func (*fakeT) Helper() {}

// Skipf records the formatted message and panics with the sentinel
// [fatalSentinel], mirroring how [testing.T.Skipf] short-circuits the
// rest of the check. Recorded rather than ignored so a test can assert
// that a check reported "examined nothing" instead of quietly passing.
func (f *fakeT) Skipf(format string, args ...any) {
	f.skips = append(f.skips, fmt.Sprintf(format, args...))
	panic(fatalSentinel{})
}

// Logf records the formatted message. Recorded rather than discarded
// so a test can assert a check reported how much it examined.
func (f *fakeT) Logf(format string, args ...any) {
	f.logs = append(f.logs, fmt.Sprintf(format, args...))
}

// Failed reports whether any error or fatal has been recorded.
func (f *fakeT) Failed() bool { return f.failed }

// fatalSentinel is the panic payload [fakeT.Fatalf] uses so
// callers can recover deterministically without conflating with
// real test panics.
type fatalSentinel struct{}

// captureFatal runs fn and reports whether it called
// [fakeT.Fatalf] during execution. The fake's recorded messages
// remain available for assertion after captureFatal returns.
func captureFatal(fn func()) (called bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(fatalSentinel); ok {
				called = true
				return
			}
			panic(r) //nolint:forbidigo
		}
	}()
	fn()
	return false
}

// flappingNamePlugin satisfies [plugin.Plugin] but returns a
// different Name on every call. Used to drive the stability
// rejection path of [assertStableName].
type flappingNamePlugin struct{ count int }

// Name increments a counter and returns a different identifier
// each call so the stability check rejects it.
func (p *flappingNamePlugin) Name() string {
	p.count++
	return fmt.Sprintf("flap-%d", p.count)
}

// Generate satisfies [plugin.Generator] so [flappingNamePlugin]
// clears the role probe and the test stays focused on the
// Name-stability rejection.
func (*flappingNamePlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// flappingProvidesPlugin satisfies [plugin.CapabilityProvider]
// but returns a different Provides slice on every call. Used to
// drive the stability rejection of
// [assertCapabilityProviderStability].
type flappingProvidesPlugin struct{ count int }

// Name returns a stable identifier so the stability check is
// not the source of failure.
func (*flappingProvidesPlugin) Name() string { return "flapping-provides" }

// Generate satisfies [plugin.Generator] so [flappingProvidesPlugin]
// clears the role probe.
func (*flappingProvidesPlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// Priority returns [priority.GeneratorFoundation] verbatim.
func (*flappingProvidesPlugin) Priority() priority.Priority { return priority.GeneratorFoundation }

// Provides increments a counter and returns a different slice
// each call.
func (p *flappingProvidesPlugin) Provides() []string {
	p.count++
	return []string{fmt.Sprintf("cap.%d", p.count)}
}

// Requires returns nil.
func (*flappingProvidesPlugin) Requires() []string { return nil }

// flappingVersionPlugin satisfies [plugin.Versioned] but returns
// a different Version on every call.
type flappingVersionPlugin struct{ count int }

// Name returns a stable identifier.
func (*flappingVersionPlugin) Name() string { return "flapping-version" }

// Generate satisfies [plugin.Generator] so the role probe
// clears.
func (*flappingVersionPlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// Version increments a counter and returns a different value
// each call.
func (p *flappingVersionPlugin) Version() string {
	p.count++
	return fmt.Sprintf("v%d", p.count)
}

// flappingEmitVersionsPlugin satisfies [plugin.EmitVersioned]
// but returns a different EmitVersions slice on every call.
type flappingEmitVersionsPlugin struct{ count int }

// Name returns a stable identifier.
func (*flappingEmitVersionsPlugin) Name() string { return "flapping-emit-versions" }

// Generate satisfies [plugin.Generator] so the role probe
// clears.
func (*flappingEmitVersionsPlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// EmitVersions increments a counter and returns a different
// slice each call.
func (p *flappingEmitVersionsPlugin) EmitVersions() []string {
	p.count++
	return []string{strconv.Itoa(p.count)}
}

// flappingNodesOnlyPlugin satisfies [plugin.NodesOnly] but
// returns a different declaration on every call.
type flappingNodesOnlyPlugin struct{ count int }

// Name returns a stable identifier.
func (*flappingNodesOnlyPlugin) Name() string { return "flapping-nodes-only" }

// Generate satisfies [plugin.Generator] so the role probe
// clears.
func (*flappingNodesOnlyPlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// NodesOnly toggles between true and false on every call.
func (p *flappingNodesOnlyPlugin) NodesOnly() bool {
	p.count++
	return p.count%2 == 0
}

// flappingSuffixPlugin satisfies [plugin.FilenameProvider] but
// returns a different Outputs slice on every call for the "go"
// language.
type flappingSuffixPlugin struct{ count int }

// Name returns a stable identifier.
func (*flappingSuffixPlugin) Name() string { return "flapping-suffix" }

// Generate satisfies [plugin.Generator] so the role probe
// clears.
func (*flappingSuffixPlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// Outputs increments a counter and returns a slice whose primary
// Suffix differs on every call so the Outputs-stability check
// rejects.
func (p *flappingSuffixPlugin) Outputs(_ string) []plugin.Output {
	p.count++
	return []plugin.Output{{Suffix: fmt.Sprintf("_v%d.go", p.count)}}
}

// malformedOutputsPlugin satisfies [plugin.FilenameProvider] with
// a caller-supplied Outputs slice. Used to drive the
// Outputs-shape conformance check against deliberately malformed
// configurations.
type malformedOutputsPlugin struct {
	outputs []plugin.Output
}

// Name returns a stable identifier.
func (*malformedOutputsPlugin) Name() string { return "malformed-outputs" }

// Generate satisfies [plugin.Generator] so the role probe clears.
func (*malformedOutputsPlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// Outputs returns the configured slice unchanged for every
// language so the shape check exercises the rules across the
// language matrix the suite probes.
func (p *malformedOutputsPlugin) Outputs(_ string) []plugin.Output { return p.outputs }

// templateProviderPlugin satisfies [plugin.TemplateProvider] with a
// caller-supplied hook behind each of the three methods.
//
// One configurable fixture rather than a type per failure mode is
// deliberate: the template contract has five distinguishable
// rejection paths (flapping ok flag, ok-with-nil-filesystem,
// filesystem-with-false-flag, flapping funcmap names, a name
// declared as both an extension and an override) and a bespoke
// struct for each would bury the difference between them in
// boilerplate. The hook signature is the interface method's own, so
// each test reads as the misbehaviour it encodes.
//
// A nil hook returns the zero value — which is exactly the
// "contributes nothing for this language" shape the check must
// accept in silence, so the zero fixture doubles as the negative
// control.
type templateProviderPlugin struct {
	name      string
	templates func(lang string) (fs.FS, bool)
	funcs     func(lang string) template.FuncMap
	overrides func(lang string) template.FuncMap
}

// Name returns the configured identifier.
func (p *templateProviderPlugin) Name() string { return p.name }

// Generate satisfies [plugin.Generator] so the role probe clears
// and the test stays scoped to the template contract.
func (*templateProviderPlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// Templates delegates to the configured hook, reporting "nothing
// for this language" when none is set.
func (p *templateProviderPlugin) Templates(lang string) (fs.FS, bool) {
	if p.templates == nil {
		return nil, false
	}
	return p.templates(lang)
}

// TemplateFuncs delegates to the configured hook, returning no
// registrations when none is set.
func (p *templateProviderPlugin) TemplateFuncs(lang string) template.FuncMap {
	if p.funcs == nil {
		return nil
	}
	return p.funcs(lang)
}

// TemplateOverrides delegates to the configured hook, returning no
// overrides when none is set.
func (p *templateProviderPlugin) TemplateOverrides(lang string) template.FuncMap {
	if p.overrides == nil {
		return nil
	}
	return p.overrides(lang)
}

// emptyInputComplainer reports one positioned Error-severity
// diagnostic when — and only when — the store it is handed holds no
// structs, and says nothing otherwise.
//
// It is the surgical fixture for the empty-store probes: a plugin
// that complains about every input would fail the per-fixture
// diagnostic check too, and could not distinguish a probe that reads
// its own sink from one that inherited a fixture's. The position is
// real so only the severity check can fire.
//
// One type serving both roles rather than two: the contract, the
// failure and the rationale are identical on the generator and
// annotator sides, and splitting them would only duplicate the
// complaint.
type emptyInputComplainer struct{ name string }

// Name returns the configured identifier.
func (p *emptyInputComplainer) Name() string { return p.name }

// Generate complains when the store carries nothing to generate from.
func (p *emptyInputComplainer) Generate(ctx *plugin.GeneratorContext) error {
	p.complainWhenEmpty(ctx.Store, ctx.Diag)
	return nil
}

// Annotate complains when the store carries nothing to stamp.
func (p *emptyInputComplainer) Annotate(ctx *plugin.AnnotatorContext) error {
	p.complainWhenEmpty(ctx.Store, ctx.Diag)
	return nil
}

// complainWhenEmpty emits the sentinel diagnostic against an empty
// store and nothing against a populated one.
func (p *emptyInputComplainer) complainWhenEmpty(s *store.Store, d *diag.Sink) {
	if s.Nodes().Structs().Len() > 0 {
		return
	}
	d.For(p.name).Errorf(position.Synthetic("plugintest-test"), "plugintest test: nothing to work with")
}

// positionedWarner emits one positioned Warn-severity diagnostic per
// invocation. It is the negative control for the positioned-
// diagnostic check: a check that rejected any diagnostic rather than
// any positionless one would make the fixture waiver mandatory for
// every plugin that reports anything at all.
type positionedWarner struct{ name string }

// Name returns the configured identifier.
func (p *positionedWarner) Name() string { return p.name }

// Generate emits a warning carrying a real position.
func (p *positionedWarner) Generate(ctx *plugin.GeneratorContext) error {
	ctx.Diag.For(p.name).Warnf(
		position.At("fixture.go", 3, 1),
		"plugintest test: deliberate positioned warning",
	)
	return nil
}

// joinFake renders every message a fake TB recorded, for inclusion in
// a meta-test failure. Reported in full rather than truncated: a
// fixture that broke an unrelated check is diagnosed from which
// message it produced, not from the fact that it produced one.
func joinFake(f *fakeT) string {
	return strings.Join(append(slices.Clone(f.errs), f.fatals...), "\n")
}
