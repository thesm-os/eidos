// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sentinel_test

import (
	"testing"

	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	sentinelplugin "go.thesmos.sh/eidos/plugins/generator/sentinel"
	"go.thesmos.sh/eidos/sdk"
)

// TestConformance runs the language-neutral framework conformance
// suites against the sentinel plugin.
//
// The fixtures here stay language-neutral: an empty store and a
// package declaring nothing exercise the per-package dispatch without
// leaning on any target language's error protocol. The fixtures that
// drive Go's own live in sentinel_go_test.go.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, sentinelplugin.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(
			t,
			sentinelplugin.New(),
			[]plugintest.GeneratorFixture{
				{
					Name: "empty store",
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return storefixture.New().Build()
					},
				},
				{
					Name: "package with no annotation",
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
		plugintest.RunOptionsSuite(t, sentinelplugin.New(), plugintest.OptionsFixture{
			Valid:      map[string]string{"prefix": "app"},
			UnknownKey: "no_such_key",
		})
	})
}
