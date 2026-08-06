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

	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/priority"
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
	failed bool
}

// newFakeT returns a fresh fake TB.
func newFakeT() *fakeT { return &fakeT{} }

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

// joinFake renders every message a fake TB recorded, for inclusion in
// a meta-test failure. Reported in full rather than truncated: a
// fixture that broke an unrelated check is diagnosed from which
// message it produced, not from the fact that it produced one.
func joinFake(f *fakeT) string {
	return strings.Join(append(slices.Clone(f.errs), f.fatals...), "\n")
}
