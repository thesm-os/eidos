// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cli_test

import (
	"errors"
	"path/filepath"
	"testing"

	"go.thesmos.sh/eidos/cli"
)

// TestNewEnv_RequiresBrand covers the documented contract that
// [cli.NewEnv] rejects an empty brand — the command surface can't
// operate without a brand to anchor state paths to.
func TestNewEnv_RequiresBrand(t *testing.T) {
	t.Parallel()

	t.Run("empty brand returns a *ConfigError", func(t *testing.T) {
		t.Parallel()
		_, err := cli.NewEnv("")
		if _, ok := errors.AsType[*cli.ConfigError](err); !ok {
			t.Fatalf("expected *ConfigError; got %T %v", err, err)
		}
	})

	t.Run("non-empty brand returns a populated *Env", func(t *testing.T) {
		t.Parallel()
		env, err := cli.NewEnv("eidos")
		if err != nil {
			t.Fatalf("NewEnv returned %v", err)
		}
		if env.Brand != "eidos" {
			t.Fatalf("Brand = %q, want %q", env.Brand, "eidos")
		}
		if env.Workdir == "" {
			t.Fatalf("Workdir should be populated from os.Getwd")
		}
		if env.Stdin == nil || env.Stdout == nil || env.Stderr == nil {
			t.Fatalf("IO surfaces should be wired to os.Stdin/Stdout/Stderr")
		}
		if env.Diag == nil {
			t.Fatalf("Diag should be a fresh sink")
		}
	})
}

// TestEnv_BrandDerivedPaths covers the brand-derived default paths
// the cli uses when the config file leaves them blank: state dir,
// manifest path, cache dir, config-file basename. The contract is
// `.<Brand>/...` under the working directory.
func TestEnv_BrandDerivedPaths(t *testing.T) {
	t.Parallel()

	t.Run("StateDir / ManifestPath / CacheDir derive from Brand and Workdir", func(t *testing.T) {
		t.Parallel()
		workdir := filepath.Join("opt", "project")
		env := &cli.Env{Brand: "acmegen", Workdir: workdir}
		if got, want := env.StateDir(), filepath.Join(workdir, ".acmegen"); got != want {
			t.Fatalf("StateDir = %q, want %q", got, want)
		}
		if got, want := env.ManifestPath(), filepath.Join(workdir, ".acmegen", "manifest.json"); got != want {
			t.Fatalf("ManifestPath = %q, want %q", got, want)
		}
		if got, want := env.CacheDir(), filepath.Join(workdir, ".acmegen", "cache"); got != want {
			t.Fatalf("CacheDir = %q, want %q", got, want)
		}
	})

	t.Run("ConfigFileName uses the leading-dot convention", func(t *testing.T) {
		t.Parallel()
		env := &cli.Env{Brand: "eidos"}
		if got := env.ConfigFileName(); got != ".eidos.yaml" {
			t.Fatalf("ConfigFileName = %q, want .eidos.yaml", got)
		}
		env.Brand = "acmegen"
		if got := env.ConfigFileName(); got != ".acmegen.yaml" {
			t.Fatalf("ConfigFileName = %q, want .acmegen.yaml", got)
		}
	})
}
