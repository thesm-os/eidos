// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest_test

import (
	"fmt"
	"sort"
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/plugin"
)

// TestRunBackendSuite_PassesForWellFormedBackend pins the
// happy path: a backend that renders each emit struct's name
// into a stable byte stream passes every contract — does not
// panic, surfaces no Error diagnostics, and produces
// byte-identical output across two runs of the same fixture.
func TestRunBackendSuite_PassesForWellFormedBackend(t *testing.T) {
	t.Parallel()
	plugintest.RunBackendSuite(
		t,
		&namingBackend{name: "namer", lang: "stub"},
		[]plugintest.BackendFixture{
			{
				Name: "single-struct package",
				BuildEmitPackages: func(t *testing.T) []*emit.Package {
					t.Helper()
					return []*emit.Package{{
						Name: "pkg",
						Path: "example.com/pkg",
						Structs: []*emit.Struct{{
							Name:    "User",
							Package: "pkg",
							Target: emit.Target{
								Dir:      "pkg",
								Filename: "user.txt",
								Package:  "pkg",
							},
						}},
					}}
				},
			},
			{
				Name: "two-struct package",
				BuildEmitPackages: func(t *testing.T) []*emit.Package {
					t.Helper()
					return []*emit.Package{{
						Name: "pkg",
						Path: "example.com/pkg",
						Structs: []*emit.Struct{
							{
								Name:    "User",
								Package: "pkg",
								Target: emit.Target{
									Dir:      "pkg",
									Filename: "user.txt",
									Package:  "pkg",
								},
							},
							{
								Name:    "Order",
								Package: "pkg",
								Target: emit.Target{
									Dir:      "pkg",
									Filename: "order.txt",
									Package:  "pkg",
								},
							},
						},
					}}
				},
			},
		},
	)
}

// TestRunBackendSuite_RejectsPanickingBackend covers the
// empty-emit panic rejection.
func TestRunBackendSuite_RejectsPanickingBackend(t *testing.T) {
	t.Parallel()
	b := &panickingBackend{name: "panicky", lang: "stub"}
	fake := newFakeT()
	plugintest.AssertRenderEmptyEmitDoesNotPanic(fake, b)
	assertFakeMentions(t, fake, "Render panicked on an empty emit graph")
}

// TestRunBackendSuite_RejectsErrorDiagnostic pins the
// no-error-diagnostics contract. A backend that surfaces a
// [diag.SeverityError] diagnostic on a fixture the author
// claims is valid trips the suite.
func TestRunBackendSuite_RejectsErrorDiagnostic(t *testing.T) {
	t.Parallel()
	b := &errorDiagnosticBackend{name: "diag-error", lang: "stub"}
	fx := plugintest.BackendFixture{
		Name: "single-struct package",
		BuildEmitPackages: func(t *testing.T) []*emit.Package {
			t.Helper()
			return []*emit.Package{{
				Name: "pkg",
				Path: "example.com/pkg",
				Structs: []*emit.Struct{{
					Name: "User",
					Target: emit.Target{
						Dir:      "pkg",
						Filename: "user.txt",
						Package:  "pkg",
					},
				}},
			}}
		},
	}
	pkgs := fx.BuildEmitPackages(t)
	fake := newFakeT()
	plugintest.AssertRenderCarriesNoErrors(fake, b, fx, pkgs)
	assertFakeMentions(t, fake, "Error-severity diagnostics")
}

// TestRunBackendSuite_RejectsNonByteStableBackend pins the
// byte-stability contract: a backend whose output embeds a
// per-call counter trips the comparison.
func TestRunBackendSuite_RejectsNonByteStableBackend(t *testing.T) {
	t.Parallel()
	b := &counterBackend{name: "counter", lang: "stub"}
	fx := plugintest.BackendFixture{
		Name: "single-struct package",
		BuildEmitPackages: func(t *testing.T) []*emit.Package {
			t.Helper()
			return []*emit.Package{{
				Name: "pkg",
				Path: "example.com/pkg",
				Structs: []*emit.Struct{{
					Name: "User",
					Target: emit.Target{
						Dir:      "pkg",
						Filename: "user.txt",
						Package:  "pkg",
					},
				}},
			}}
		},
	}
	pkgsA := fx.BuildEmitPackages(t)
	pkgsB := fx.BuildEmitPackages(t)
	fake := newFakeT()
	plugintest.AssertRenderIsByteStable(fake, b, fx, pkgsA, pkgsB)
	assertFakeMentions(t, fake, "byte-stable")
}

// TestRunBackendSuite_FailsOnDuplicateFixtureName pins the
// fixture-name uniqueness contract.
func TestRunBackendSuite_FailsOnDuplicateFixtureName(t *testing.T) {
	t.Parallel()
	fixtures := []plugintest.BackendFixture{
		{Name: "dup", BuildEmitPackages: func(_ *testing.T) []*emit.Package { return nil }},
		{Name: "dup", BuildEmitPackages: func(_ *testing.T) []*emit.Package { return nil }},
	}
	fake := newFakeT()
	captureFatal(func() {
		plugintest.AssertBackendFixtureNamesUnique(fake, fixtures)
	})
	assertFakeMentions(t, fake, "duplicate fixture Name")
}

// namingBackend renders each emit struct's name to its
// target's full path. Deterministic by construction.
type namingBackend struct {
	name string
	lang string
}

// Name returns the configured identifier.
func (b *namingBackend) Name() string { return b.name }

// Language returns the configured target-language identifier.
func (b *namingBackend) Language() string { return b.lang }

// Render walks every emit struct in deterministic order and
// writes "<name>\n" to its target. Structs are sorted by qname
// so the rendering order matches across two runs.
func (*namingBackend) Render(ctx *plugin.BackendContext) error {
	structs := ctx.Store.Emit().Structs().Items()
	type rendered struct {
		target emit.Target
		body   []byte
	}
	out := make([]rendered, 0, len(structs))
	for _, s := range structs {
		out = append(out, rendered{
			target: s.Target,
			body:   []byte(s.QName() + "\n"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].target.JoinPath() < out[j].target.JoinPath() })
	for _, r := range out {
		if err := ctx.Sink.Write(r.target, r.body); err != nil {
			return fmt.Errorf("namingBackend: write %v: %w", r.target, err)
		}
	}
	return nil
}

// panickingBackend panics in Render.
type panickingBackend struct {
	name string
	lang string
}

// Name returns the configured identifier.
func (b *panickingBackend) Name() string { return b.name }

// Language returns the configured target-language identifier.
func (b *panickingBackend) Language() string { return b.lang }

// Render panics with a sentinel message.
func (*panickingBackend) Render(_ *plugin.BackendContext) error {
	panic("plugintest test: panickingBackend panicking on purpose") //nolint:forbidigo
}

// errorDiagnosticBackend writes valid output and surfaces an
// Error-severity diagnostic to verify the suite's
// no-error-diagnostics contract.
type errorDiagnosticBackend struct {
	name string
	lang string
}

// Name returns the configured identifier.
func (b *errorDiagnosticBackend) Name() string { return b.name }

// Language returns the configured target-language identifier.
func (b *errorDiagnosticBackend) Language() string { return b.lang }

// Render emits a sentinel error diagnostic and returns nil.
func (*errorDiagnosticBackend) Render(ctx *plugin.BackendContext) error {
	ctx.Diag.Errorf(position.Synthetic("plugintest-test"), "plugintest test: deliberate error diagnostic")
	return nil
}

// counterBackend emits content whose bytes vary across calls
// — used to drive the byte-stability rejection path.
type counterBackend struct {
	name  string
	lang  string
	count int
}

// Name returns the configured identifier.
func (b *counterBackend) Name() string { return b.name }

// Language returns the configured target-language identifier.
func (b *counterBackend) Language() string { return b.lang }

// Render writes a counter-derived body to each emit struct's
// target.
func (b *counterBackend) Render(ctx *plugin.BackendContext) error {
	b.count++
	for _, s := range ctx.Store.Emit().Structs().Items() {
		body := fmt.Appendf(nil, "%s-%d\n", s.QName(), b.count)
		if err := ctx.Sink.Write(s.Target, body); err != nil {
			return fmt.Errorf("counterBackend: write: %w", err)
		}
	}
	return nil
}
