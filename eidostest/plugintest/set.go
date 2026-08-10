// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
)

// RunSetSuite asserts the contracts that hold over a whole plugin set
// rather than over one plugin, plus [RunSuite] over each member.
//
// # What a set can be wrong about that a member cannot
//
// Every conformance suite this package ships is single-plugin, and
// every property they check is one a plugin can satisfy while the set
// it ships in does not: two plugins with one name, two declaring the
// same directive schema, two providing one capability in one priority
// bucket, two whose emit majors do not intersect. A consumer
// assembling a binary's plugin slice has no way to ask whether that
// slice is coherent short of building the binary and running it.
//
// # Asserted by building, not by re-deriving
//
// The checks are the pipeline's own. Name uniqueness, capability
// resolution, directive-registry construction, emit-version
// compatibility and output well-formedness are all decided at
// [pipeline.Builder.Build], and re-deriving any of them here would put
// a second implementation of each beside the first — which is the
// failure mode this whole vocabulary exists to remove. What this adds
// is the ability to ask the question without a frontend, a backend, a
// store or a filesystem.
//
// # Role presence is deliberately not asserted
//
// A consumer's set is legitimately partial: a bundle of generators is
// the common case, and the binary supplies the frontend and backend.
// So a role the set does not fill is filled here by a stub, and only
// roles the set *does* fill are its own. A set carrying two backends
// still fails, because that is a real contradiction rather than an
// absence.
//
// # An unprovided Requires is not a fault
//
// Worth stating because it is the check a reader expects here and
// will not find. The pipeline resolves capabilities within a priority
// bucket and ignores a Requires naming a capability nothing in that
// bucket provides — deliberately, and documented as such: a plugin
// may legitimately require something an earlier bucket supplied, or
// something optional it degrades without. Asserting closure here
// would fail sets the pipeline runs correctly, so this reports
// exactly what the pipeline rejects and nothing it tolerates.
//
// # Not a substitute for a real run
//
// A set that builds can still generate nothing useful, and a member
// implementing no role at all is registered nowhere — so it is the
// per-member [RunSuite] above, not the build, that catches one. This
// answers the structural question — does this slice cohere — which is
// the one that today has no answer short of shipping.
func RunSetSuite(t *testing.T, ps ...plugin.Plugin) {
	t.Helper()

	if len(ps) == 0 {
		t.Fatal("plugintest: RunSetSuite over an empty set would assert nothing, " +
			"and would pass having done so")
		return
	}

	for _, p := range ps {
		t.Run("member/"+p.Name(), func(t *testing.T) {
			RunSuite(t, p)
		})
	}

	t.Run("the set builds a pipeline", func(t *testing.T) {
		assertSetBuilds(t, ps)
	})
}

// assertSetBuilds registers the set on a real builder, fills the roles
// it does not, and reports whatever Build rejects.
//
// Build returns every structural problem joined rather than the first,
// so one report names all of them — which is the difference between
// fixing a set in one pass and in as many passes as it has faults.
func assertSetBuilds(tb testing.TB, ps []plugin.Plugin) {
	tb.Helper()

	b := pipeline.New().WithPlugins(ps...)
	for _, filler := range absentRoleFillers(ps) {
		b = filler(b)
	}
	if _, err := b.Build(); err != nil {
		tb.Errorf("plugintest: the plugin set does not build: %v", err)
	}
}

// absentRoleFillers returns the registrations needed to make the set
// buildable without asserting anything about roles it does not claim.
//
// Only the frontend and the backend, because those are the two the
// builder's role counts require. A set with no generator and no
// annotator builds fine and generates nothing, which is a coherent
// configuration this has no business rejecting.
func absentRoleFillers(ps []plugin.Plugin) []func(*pipeline.Builder) *pipeline.Builder {
	var hasFrontend, hasBackend bool
	for _, p := range ps {
		if _, ok := p.(plugin.Frontend); ok {
			hasFrontend = true
		}
		if _, ok := p.(plugin.Backend); ok {
			hasBackend = true
		}
	}
	var out []func(*pipeline.Builder) *pipeline.Builder
	if !hasFrontend {
		out = append(out, func(b *pipeline.Builder) *pipeline.Builder {
			return b.WithFrontend(&setFrontend{})
		})
	}
	if !hasBackend {
		out = append(out, func(b *pipeline.Builder) *pipeline.Builder {
			return b.WithBackend(&setBackend{})
		})
	}
	return out
}

// setFrontend fills the frontend role for a set that declares none. It
// loads nothing: the suite asserts that the set coheres, and a
// frontend producing fixture nodes would make the result depend on
// what those nodes happened to be.
type setFrontend struct{}

// Name returns the filler's identifier, spelled so a build error
// naming it reads as coming from the harness rather than from the
// caller's own set.
func (*setFrontend) Name() string { return "plugintest.set-frontend" }

// Load returns without producing nodes.
func (*setFrontend) Load(_ *plugin.FrontendContext) error { return nil }

// setBackend fills the backend role for a set that declares none. It
// renders nothing, for the same reason [setFrontend] loads nothing.
type setBackend struct{}

// Name returns the filler's identifier.
func (*setBackend) Name() string { return "plugintest.set-backend" }

// Language returns the conformance language, so a set whose members
// declare templates for it is registered against a backend that
// claims it rather than one that claims nothing.
func (*setBackend) Language() string { return ConformanceLanguage }

// Render returns without writing.
func (*setBackend) Render(_ *plugin.BackendContext) error { return nil }

// EmitVersions declares the majors the backend accepts.
//
// The major the emit model is at, because a filler must not
// be the reason a set fails its version check: the assertion worth
// making is that the *members* agree with each other.
func (*setBackend) EmitVersions() []string { return []string{emit.Major()} }
