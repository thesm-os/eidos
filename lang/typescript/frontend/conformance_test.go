// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"path/filepath"
	"testing"

	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/lang/typescript/frontend"
)

// TestFrontendConformance runs the framework's plugin and frontend
// conformance suites.
//
// These are the contracts the pipeline relies on rather than
// behaviours this frontend chose: that Load is deterministic across
// two runs, that it re-parses when the composition fingerprint
// changes rather than replaying a cached graph, that every
// diagnostic carries a position, and that a declared-handled fixture
// produces no Error-severity output. A failure here means the
// frontend would either crash a pipeline or make its output
// non-reproducible.
func TestFrontendConformance(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("testdata/project")
	if err != nil {
		t.Fatalf("resolve fixture root: %v", err)
	}

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, frontend.New())
	})

	t.Run("frontend contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunFrontendSuite(t, frontend.New(), []plugintest.FrontendFixture{
			{
				Name:    "recursive",
				Pattern: "./...",
				Options: map[string]string{"dir": root},
			},
			{
				Name:    "single-directory",
				Pattern: "./src",
				Options: map[string]string{"dir": root},
			},
			{
				Name:    "single-file",
				Pattern: "./src/types.ts",
				Options: map[string]string{"dir": root},
			},
			{
				Name:    "glob",
				Pattern: "./src/*.ts",
				Options: map[string]string{"dir": root},
			},
		})
	})
}
