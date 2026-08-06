// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest_test

import (
	"testing"

	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/store"
)

// ExampleRunSuite shows how a plugin author wires the conformance
// suites into their own test: the universal framework checks first,
// then one per-role suite for each role the plugin implements, each
// carrying the fixtures that describe realistic input.
//
// The suites register subtests, so they take the enclosing test's
// `*testing.T` — which an [Example] function is never given. The
// body below is therefore written as the helper a plugin author
// calls from their own `func TestMyPlugin(t *testing.T)`, and the
// example never invokes it.
//
// There is deliberately no `// Output:` block. Without one Go
// compiles and type-checks the example but does not run it, and the
// compile check is the whole point: the equivalent snippet in the
// package docblock is prose, no build ever reads it, and snippets in
// this module have drifted from the real signatures before. Whether
// the suites behave correctly when run is settled elsewhere — see
// TestRunSuite_PassesForWellFormedPlugin and its per-role siblings,
// which drive them against real plugins.
func ExampleRunSuite() {
	assertConformance := func(t *testing.T) {
		t.Helper()

		// A real test constructs its own plugin here; the shipped
		// fixture stands in so the example has something concrete.
		p := plugintest.NewFixturePlugin()

		// Universal: appropriate for every plugin regardless of role.
		plugintest.RunSuite(t, p)

		// Per-role: invoked only for the roles the plugin implements.
		// Each fixture's BuildStore must return a freshly-populated
		// store, because the determinism check builds two and
		// compares the emit graphs they produce.
		plugintest.RunGeneratorSuite(t, p, []plugintest.GeneratorFixture{{
			Name: "one annotated struct",
			BuildStore: func(t *testing.T) *store.Store {
				t.Helper()
				return storefixture.New().
					Struct("User", func(s *storefixture.StructBuilder) {
						s.Directive(storefixture.Directive("fixture"))
						s.Field("ID", storefixture.Named("string"), nil)
					}).
					Build()
			},
		}})
	}

	// An Example has no *testing.T to hand over; see the docblock.
	_ = assertConformance
}
