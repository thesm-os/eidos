// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package enum_test

import (
	"testing"

	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	enumplugin "go.thesmos.sh/eidos/plugins/generator/enum"
	"go.thesmos.sh/eidos/sdk"
)

// TestConformance runs the language-neutral framework conformance
// suites against the enum plugin.
//
// The fixtures here stay language-neutral: an empty store and a
// package declaring nothing exercise the per-package dispatch without
// leaning on any target language's spelling. The fixtures that drive
// Go's own classification live in enum_go_test.go.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, enumplugin.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(
			t,
			enumplugin.New(),
			[]plugintest.GeneratorFixture{
				{
					Name: "empty store",
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return gofixture.New().Build()
					},
				},
				{
					Name: "package with no annotated enums",
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return gofixture.New().
							Package("blog", "example.com/blog").
							Build()
					},
				},
			},
		)
	})

	t.Run("options round-trip", func(t *testing.T) {
		t.Parallel()
		plugintest.RunOptionsSuite(t, enumplugin.New(), plugintest.OptionsFixture{
			Valid:      map[string]string{"parse-word": "Parse"},
			UnknownKey: "no_such_key",
		})
	})
}
