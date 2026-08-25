// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
)

func TestDirective(t *testing.T) {
	t.Parallel()

	t.Run("produces a positive directive with no args by default", func(t *testing.T) {
		t.Parallel()
		d := gofixture.Directive("repo")
		if d.Name != "repo" {
			t.Fatalf("name wrong: %q", d.Name)
		}
		if d.Negated {
			t.Fatalf("default directive should not be negated")
		}
		if len(d.Args) != 0 {
			t.Fatalf("default Args should be empty: %+v", d.Args)
		}
		if d.KV == nil {
			t.Fatalf("KV should be initialised, not nil")
		}
	})

	t.Run("applies options left-to-right", func(t *testing.T) {
		t.Parallel()
		pos := position.At("user.go", 7, 1)
		d := gofixture.Directive(
			"mock",
			gofixture.Negated(),
			gofixture.Arg("UserRepo"),
			gofixture.KV("target", "first"),
			gofixture.KV("target", "second"),
			gofixture.At(pos),
		)
		if !d.Negated {
			t.Fatalf("Negated() should mark the directive negated")
		}
		if len(d.Args) != 1 || d.Args[0] != "UserRepo" {
			t.Fatalf("Args wrong: %+v", d.Args)
		}
		if d.KV["target"] != "second" {
			t.Fatalf("later KV should overwrite earlier; got %q", d.KV["target"])
		}
		if !d.Pos.Equal(pos) {
			t.Fatalf("position wrong: got %v, want %v", d.Pos, pos)
		}
	})
}

func TestRouteTo(t *testing.T) {
	t.Parallel()

	// The assertion the helper exists for. Layout reads the value as
	// a path and treats a slashless one as a filename, so the routed
	// package silently becomes a file named after the directory —
	// with no diagnostic. Nothing else in the suite would catch a
	// refactor that dropped the separator.
	t.Run("terminates the directory with a separator", func(t *testing.T) {
		t.Parallel()
		d := gofixture.RouteTo("validategen", "validation", "validation")
		if len(d.Args) != 1 || d.Args[0] != "validation/" {
			t.Fatalf("Args = %q, want [\"validation/\"]; a slashless value routes to a "+
				"file called `validation`, not a directory", d.Args)
		}
	})

	t.Run("accepts a directory already carrying the separator", func(t *testing.T) {
		t.Parallel()
		d := gofixture.RouteTo("validategen", "validation/", "validation")
		if len(d.Args) != 1 || d.Args[0] != "validation/" {
			t.Fatalf("Args = %q, want [\"validation/\"] with no doubled separator", d.Args)
		}
	})

	t.Run("leaves an empty directory empty", func(t *testing.T) {
		t.Parallel()
		d := gofixture.RouteTo("validategen", "", "validation")
		if len(d.Args) != 1 || d.Args[0] != "" {
			t.Fatalf("Args = %q, want [\"\"]; an empty dir means the origin's own "+
				"directory, not the module root", d.Args)
		}
	})

	t.Run("scopes the routing to one plugin and names the package", func(t *testing.T) {
		t.Parallel()
		d := gofixture.RouteTo("validategen", "validation", "vpkg")
		if d.Name != "out" {
			t.Fatalf("name = %q, want the canonical out directive", d.Name)
		}
		if d.Negated {
			t.Fatalf("a routing directive is never the negated form")
		}
		if d.KV["plugin"] != "validategen" {
			t.Fatalf("plugin = %q, want validategen; an unscoped route moves every "+
				"plugin's output", d.KV["plugin"])
		}
		if d.KV["pkg"] != "vpkg" {
			t.Fatalf("pkg = %q, want vpkg", d.KV["pkg"])
		}
	})
}
