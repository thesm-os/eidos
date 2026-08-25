// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"os"
	"path/filepath"
	"testing"

	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/lang/protobuf/frontend"
	"go.thesmos.sh/eidos/plugin"
)

// TestPluginShape pins the protobuf frontend's public-contract
// surface so a rename / drop accident surfaces at PR review time.
func TestPluginShape(t *testing.T) {
	t.Parallel()

	t.Run("Name returns the documented identifier", func(t *testing.T) {
		t.Parallel()
		if got := frontend.New().Name(); got != frontend.FrontendName {
			t.Fatalf("Name = %q, want %q", got, frontend.FrontendName)
		}
	})

	t.Run("FrontendName is the literal string \"protobuf\"", func(t *testing.T) {
		t.Parallel()
		if frontend.FrontendName != "protobuf" {
			t.Fatalf("FrontendName = %q, want %q", frontend.FrontendName, "protobuf")
		}
	})

	t.Run("Frontend implements plugin.Frontend", func(t *testing.T) {
		t.Parallel()
		var _ plugin.Frontend = frontend.New()
	})

	t.Run("Frontend implements plugin.Versioned", func(t *testing.T) {
		t.Parallel()
		var _ plugin.Versioned = frontend.New()
	})

	t.Run("Frontend implements plugin.OptionsProvider", func(t *testing.T) {
		t.Parallel()
		var _ plugin.OptionsProvider = frontend.New()
	})

	t.Run("Version is non-empty so it contributes to the cache key", func(t *testing.T) {
		t.Parallel()
		if frontend.New().Version() == "" {
			t.Fatalf(
				"Version() returned empty; a non-empty version is required so it contributes to the cache key",
			)
		}
	})
}

// TestOptionsSchema pins the documented frontend options surface.
// Each option's default and accepted value forms must round-trip
// through the reflected schema so misconfigured pipelines surface
// positioned diagnostics at Build time.
func TestOptionsSchema(t *testing.T) {
	t.Parallel()

	t.Run("OptionsSchema returns a schema with the documented keys", func(t *testing.T) {
		t.Parallel()
		schema := frontend.New().OptionsSchema()
		wantKeys := []string{"dir", "import_paths", "include_well_known"}
		for _, key := range wantKeys {
			if !schema.HasField(key) {
				t.Fatalf("schema missing option %q", key)
			}
		}
	})

	t.Run("default option values match the documented defaults", func(t *testing.T) {
		t.Parallel()
		f := frontend.New()
		assertNoError(t, f.SetOptions(opt.New(f.OptionsSchema(), nil)))
		if got := f.Dir(); got != "" {
			t.Fatalf("dir default = %q, want empty", got)
		}
		if got := f.ImportPaths(); len(got) != 0 {
			t.Fatalf("import_paths default = %v, want empty slice", got)
		}
		if got := f.IncludeWellKnown(); !got {
			t.Fatalf("include_well_known default = false, want true")
		}
	})

	t.Run("import_paths parses a comma-separated list", func(t *testing.T) {
		t.Parallel()
		f := frontend.New()
		assertNoError(t, f.SetOptions(opt.New(f.OptionsSchema(), map[string]string{
			"import_paths": "/a,/b,/c",
		})))
		got := f.ImportPaths()
		want := []string{"/a", "/b", "/c"}
		if len(got) != len(want) {
			t.Fatalf("import_paths len = %d, want %d (%v)", len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("import_paths[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("include_well_known accepts true / false", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			raw  string
			want bool
		}{{"true", true}, {"false", false}} {
			f := frontend.New()
			assertNoError(t, f.SetOptions(opt.New(f.OptionsSchema(), map[string]string{
				"include_well_known": tc.raw,
			})))
			if got := f.IncludeWellKnown(); got != tc.want {
				t.Fatalf("include_well_known(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		}
	})
}

// assertNoError fails the test if err is non-nil. Mirrors the
// helper used across the rest of the package's tests; lives here
// in the per-file test until helpers_test.go lands with the
// shared fixtures.
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestConformance runs the framework's plugin-conformance suite
// against this package's plugin. The suite pins the standard
// framework contracts (stable Name, role-interface compliance,
// deterministic capability ordering, unique directive schema
// names, non-empty Versioned version) plus the per-role
// frontend contracts (empty-pattern panic recovery, determinism
// across two runs of the same source fixture).
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, frontend.New())
	})

	t.Run("frontend contracts", func(t *testing.T) {
		t.Parallel()
		// testdata fixtures live alongside this package; use
		// absolute paths so the suite stays immune to cwd pivots
		// other tests perform.
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("os.Getwd: %v", err)
		}
		plugintest.RunFrontendSuite(
			t,
			frontend.New(),
			[]plugintest.FrontendFixture{
				{
					Name:    "simple proto fixture",
					Pattern: "./...",
					Options: map[string]string{
						"dir": filepath.Join(cwd, "testdata", "simple"),
					},
				},
				{
					Name:    "messages proto fixture",
					Pattern: "./...",
					Options: map[string]string{
						"dir": filepath.Join(cwd, "testdata", "messages"),
					},
				},
			},
		)
	})
}
