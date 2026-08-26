// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/typescript/typescripttest/tsfixture"
	"go.thesmos.sh/eidos/sdk"
)

func TestDirective(t *testing.T) {
	t.Parallel()

	t.Run("the options populate what a parser would", func(t *testing.T) {
		t.Parallel()
		d := tsfixture.Directive("repo",
			tsfixture.Negated(),
			tsfixture.Arg("first"),
			tsfixture.Arg("second"),
			tsfixture.KV("mode", "strict"),
			tsfixture.At(position.Pos{File: "user.ts", Line: 3}),
		)
		if d.Name != "repo" || !d.Negated {
			t.Fatalf("directive = %+v", d)
		}
		if len(d.Args) != 2 || d.Args[0] != "first" {
			t.Errorf("args = %v", d.Args)
		}
		if d.KV["mode"] != "strict" {
			t.Errorf("kv = %v", d.KV)
		}
		if d.Pos.File != "user.ts" || d.Pos.Line != 3 {
			t.Errorf("pos = %+v", d.Pos)
		}
	})

	t.Run("the default is the positive form with an empty map", func(t *testing.T) {
		t.Parallel()
		// An empty map rather than nil, so a caller reading a key it
		// did not set gets the zero value instead of a panic.
		d := tsfixture.Directive("repo")
		if d.Negated || d.KV == nil {
			t.Fatalf("directive = %+v", d)
		}
	})
}

func TestRouteTo(t *testing.T) {
	t.Parallel()

	t.Run("a directory gains its separator", func(t *testing.T) {
		t.Parallel()
		// Layout reads the value as a path, and a trailing separator is
		// its only way to say "this is a directory, keep the filename
		// you composed". Without one the output lands in a file
		// literally called `validation`, which nothing imports and
		// nothing diagnoses.
		for _, spelled := range []string{"validation", "validation/"} {
			d := tsfixture.RouteTo("repo", spelled, "validation")
			if d.Args[0] != "validation/" {
				t.Errorf("RouteTo(%q) routed to %q", spelled, d.Args[0])
			}
		}
	})

	t.Run("an empty directory stays empty", func(t *testing.T) {
		t.Parallel()
		// "Same directory" is a legitimate routing and must stay
		// expressible; a bare separator would name the project root.
		if got := tsfixture.RouteTo("repo", "", "x").Args[0]; got != "" {
			t.Fatalf("routed to %q, want the origin's own directory", got)
		}
	})

	t.Run("the plugin and the directory identity are recorded", func(t *testing.T) {
		t.Parallel()
		d := tsfixture.RouteTo("repo", "gen/api", "api")
		if d.Name != sdk.OutDirective {
			t.Errorf("name = %q, want the out directive", d.Name)
		}
		if d.KV["plugin"] != "repo" || d.KV["pkg"] != "api" {
			t.Errorf("kv = %v", d.KV)
		}
	})
}
