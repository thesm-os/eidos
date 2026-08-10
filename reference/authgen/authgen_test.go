// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package authgen_test

import (
	"testing"

	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/reference/authgen"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites against
// the authgen plugin: [plugintest.RunSuite] for the stability, role and
// capability contracts, and [plugintest.RunGeneratorSuite] for the
// per-fixture generator contracts — determinism across two runs, a
// frozen source graph, truthful NodesOnly, declared output tags, and
// tolerance of a partial output-package dispatch.
//
// The generator suite is the half that reaches behaviour. RunSuite
// alone checks what the plugin declares; only the fixtures exercise
// what Generate does with a store.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, authgen.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(t, authgen.New(), []plugintest.GeneratorFixture{
			{
				Name: "empty store",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return store.New()
				},
			},
			{
				// A generator must decline work it was not asked for
				// without emitting, panicking, or touching the source
				// graph — the path a real run takes for most packages.
				Name: "package with nothing this plugin handles",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return storefixture.New().Struct("Plain", nil).Build()
				},
			},
		})
	})
}
