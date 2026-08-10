// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/plugin"
)

// requireSetRejected asserts the fake recorded a rejection naming want.
func requireSetRejected(t *testing.T, f *fakeT, want string) {
	t.Helper()
	if !f.failed {
		t.Fatalf("the set was accepted; expected a rejection naming %q", want)
	}
	if got := joinFake(f); !strings.Contains(got, want) {
		t.Fatalf("rejection does not name %q:\n%s", want, got)
	}
}

// requireSetAccepted asserts the fake recorded nothing.
func requireSetAccepted(t *testing.T, f *fakeT) {
	t.Helper()
	if f.failed {
		t.Fatalf("the set was rejected:\n%s", joinFake(f))
	}
}

// TestRunSetSuite covers the entry point over sets that hold, since
// the passing path is the one a consumer runs. Rejections are driven
// through [plugintest.AssertSetBuilds] against a recording fake,
// because RunSetSuite takes a *testing.T and one cannot be fabricated
// outside the harness.
func TestRunSetSuite(t *testing.T) {
	t.Parallel()

	t.Run("a generator-only set passes", func(t *testing.T) {
		t.Parallel()
		// The common consumer shape: a bundle of generators, with the
		// binary supplying the frontend and backend. Asserting role
		// presence would fail this, which is why it does not.
		plugintest.RunSetSuite(t, gen("alpha"), gen("beta"))
	})

	t.Run("a set carrying its own backend is not given a second", func(t *testing.T) {
		t.Parallel()
		// Two backends is what the builder rejects, so a filler added
		// unconditionally would fail every set that brought one.
		plugintest.RunSetSuite(t, &setTestBackend{})
	})
}

func TestAssertSetBuilds(t *testing.T) {
	t.Parallel()

	t.Run("accepts a set whose members do not collide", func(t *testing.T) {
		t.Parallel()
		f := newFakeT()
		plugintest.AssertSetBuilds(f, []plugin.Plugin{gen("alpha"), gen("beta")})
		requireSetAccepted(t, f)
	})

	t.Run("reports two members sharing one name", func(t *testing.T) {
		t.Parallel()
		// A property no single-plugin suite can check: each member is
		// individually conformant, and the set is not.
		f := newFakeT()
		plugintest.AssertSetBuilds(f, []plugin.Plugin{gen("dup"), gen("dup")})
		requireSetRejected(t, f, "dup")
	})

	t.Run("tolerates a Requires nothing in the set provides", func(t *testing.T) {
		t.Parallel()
		// The check a reader expects here and will not find. The
		// pipeline resolves capabilities within a priority bucket and
		// documents that it ignores a Requires naming one nothing in
		// that bucket provides — a plugin may require something an
		// earlier bucket supplied, or something optional. Asserting
		// closure would fail sets that run correctly.
		needy := gen("needy")
		needy.CapabilityRequires = []string{"nobody.provides.this"}

		f := newFakeT()
		plugintest.AssertSetBuilds(f, []plugin.Plugin{needy})
		requireSetAccepted(t, f)
	})

	t.Run("reports two members providing one capability", func(t *testing.T) {
		t.Parallel()
		// The capability fault the pipeline does reject: two providers
		// of one name in one bucket, which leaves the order between
		// them undecidable.
		a := gen("a")
		a.CapabilityProvides = []string{"shape.facts"}
		b := gen("b")
		b.CapabilityProvides = []string{"shape.facts"}

		f := newFakeT()
		plugintest.AssertSetBuilds(f, []plugin.Plugin{a, b})
		requireSetRejected(t, f, "shape.facts")
	})

	t.Run("accepts a requirement another member provides", func(t *testing.T) {
		t.Parallel()
		// The half that must not over-report: a capability satisfied
		// inside the set is not a fault, and a check keyed on the
		// member alone would say it was.
		provider := gen("provider")
		provider.CapabilityProvides = []string{"shape.facts"}
		consumer := gen("consumer")
		consumer.CapabilityRequires = []string{"shape.facts"}

		f := newFakeT()
		plugintest.AssertSetBuilds(f, []plugin.Plugin{provider, consumer})
		requireSetAccepted(t, f)
	})

	t.Run("reports two members declaring one directive schema", func(t *testing.T) {
		t.Parallel()
		a := gen("a")
		a.DirectiveSchemas = []directive.Schema{directive.NewSchema("shared").On("Struct").Build()}
		b := gen("b")
		b.DirectiveSchemas = []directive.Schema{directive.NewSchema("shared").On("Struct").Build()}

		f := newFakeT()
		plugintest.AssertSetBuilds(f, []plugin.Plugin{a, b})
		requireSetRejected(t, f, "shared")
	})

	t.Run("reports a set carrying two backends", func(t *testing.T) {
		t.Parallel()
		// The one absence that is a contradiction rather than a gap,
		// and therefore the one the filler must not paper over.
		f := newFakeT()
		plugintest.AssertSetBuilds(f, []plugin.Plugin{&setTestBackend{}, &secondTestBackend{}})
		if !f.failed {
			t.Fatal("a set carrying two backends was accepted")
		}
	})
}

// gen returns a role-implementing member with the supplied name and
// no directive schemas or capability requirements, so a set assembled
// from several differs only in what a case varies.
//
// NewFixturePlugin ships two schemas and a requirement of its own, so
// two unmodified copies collide on both counts — which would make
// every case below fail for a reason it is not about.
func gen(name string) *plugintest.FixturePlugin {
	p := plugintest.NewFixturePlugin()
	p.PluginName = name
	p.DirectiveSchemas = nil
	p.CapabilityProvides = nil
	p.CapabilityRequires = nil
	return p
}

// setTestBackend is a conformant backend, used to prove the filler
// stands down for a role the set already fills.
type setTestBackend struct{}

func (*setTestBackend) Name() string                          { return "set-test-backend" }
func (*setTestBackend) Language() string                      { return plugintest.ConformanceLanguage }
func (*setTestBackend) Render(_ *plugin.BackendContext) error { return nil }
func (*setTestBackend) EmitVersions() []string                { return []string{"1"} }

// secondTestBackend is a distinctly-named second backend, so the
// two-backend rejection is not confused with a duplicate name.
type secondTestBackend struct{ setTestBackend }

// Name returns the second backend's own identifier.
func (*secondTestBackend) Name() string { return "second-test-backend" }
