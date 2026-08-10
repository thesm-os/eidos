// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	builderplugin "go.thesmos.sh/eidos/plugins/generator/builder"
	"go.thesmos.sh/eidos/sdk"
)

// TestConformance runs the language-neutral framework
// conformance suites against the builder plugin. The
// framework contracts pin the static surface (stable Name,
// role implementation, deterministic Outputs across the
// language dispatch); the generator suite confirms the
// plugin's [Generate] pass is panic-safe, leaves source
// nodes untouched, and produces a deterministic emit
// projection across two runs over the same fixture.
//
// Generator fixtures here intentionally stay language-
// neutral — the empty store and an annotated package
// without any structures exercise the dispatch + iteration
// machinery without leaning on any target-language's field
// shape. Per-language fixtures that drive Go-specific
// classification live in builder_go_test.go.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, builderplugin.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(
			t,
			builderplugin.New(),
			[]plugintest.GeneratorFixture{
				{
					Name: "empty store",
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return storefixture.New().Build()
					},
				},
				{
					Name: "package with no annotated structs",
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return storefixture.New().
							Package("blog", "example.com/blog").
							Build()
					},
				},
			},
		)
	})

	t.Run("options round-trip", func(t *testing.T) {
		t.Parallel()
		plugintest.RunOptionsSuite(t, builderplugin.New(), plugintest.OptionsFixture{
			Valid:      map[string]string{"suffix": "Builder"},
			UnknownKey: "no_such_key",
		})
	})
}
