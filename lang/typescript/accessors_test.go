// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// decorated returns a struct carrying the supplied decorators.
func decorated(ds ...typescript.Decorator) *node.Struct {
	s := &node.Struct{Name: "User"}
	if len(ds) > 0 {
		typescript.MetaDecorators.Set(s.EnsureMeta(), ds, "test")
	}
	return s
}

func TestDecorators(t *testing.T) {
	t.Parallel()

	t.Run("returns decorators in the order they were recorded", func(t *testing.T) {
		t.Parallel()
		// Order is part of what a decorator means: expressions
		// evaluate top-down and apply bottom-up, so a list read out of
		// order describes a different composition.
		s := decorated(
			typescript.Decorator{Name: "A"},
			typescript.Decorator{Name: "B"},
		)
		got := typescript.DecoratorNames(s)
		if !slices.Equal(got, []string{"A", "B"}) {
			t.Fatalf("DecoratorNames = %v, want [A B]", got)
		}
	})

	t.Run("an undecorated declaration reports none", func(t *testing.T) {
		t.Parallel()
		if got := typescript.Decorators(decorated()); len(got) != 0 {
			t.Fatalf("Decorators = %+v, want none", got)
		}
		if got := typescript.DecoratorNames(decorated()); len(got) != 0 {
			t.Fatalf("DecoratorNames = %v, want none", got)
		}
	})

	t.Run("nil carries no decorators", func(t *testing.T) {
		t.Parallel()
		if got := typescript.Decorators(nil); got != nil {
			t.Fatalf("Decorators(nil) = %+v, want nil", got)
		}
		if typescript.HasDecorator(nil, "A") {
			t.Fatal("HasDecorator(nil) reported a decorator")
		}
	})
}

func TestDecoratorNamed(t *testing.T) {
	t.Parallel()

	t.Run("returns the matching decorator with its arguments", func(t *testing.T) {
		t.Parallel()
		s := decorated(
			typescript.Decorator{Name: "Entity", Args: "({ name: 'users' })"},
			typescript.Decorator{Name: "Index"},
		)
		got, ok := typescript.DecoratorNamed(s, "Entity")
		if !ok {
			t.Fatal("Entity not found")
		}
		if got.Args != "({ name: 'users' })" {
			t.Fatalf("Args = %q", got.Args)
		}
	})

	t.Run("returns the first when a name repeats", func(t *testing.T) {
		t.Parallel()
		// First rather than only, because a name may legitimately
		// appear more than once — DecoratorsNamed is for the whole set.
		s := decorated(
			typescript.Decorator{Name: "R", Args: "(200)"},
			typescript.Decorator{Name: "R", Args: "(404)"},
		)
		got, ok := typescript.DecoratorNamed(s, "R")
		if !ok || got.Args != "(200)" {
			t.Fatalf("DecoratorNamed = %+v, want the first application", got)
		}
	})

	t.Run("reports absence for an unknown name", func(t *testing.T) {
		t.Parallel()
		s := decorated(typescript.Decorator{Name: "A"})
		if _, ok := typescript.DecoratorNamed(s, "B"); ok {
			t.Fatal("found a decorator that is not there")
		}
		if typescript.HasDecorator(s, "B") {
			t.Fatal("HasDecorator reported a decorator that is not there")
		}
	})

	t.Run("HasDecorator agrees with DecoratorNamed", func(t *testing.T) {
		t.Parallel()
		s := decorated(typescript.Decorator{Name: "A"})
		if !typescript.HasDecorator(s, "A") {
			t.Fatal("HasDecorator missed a present decorator")
		}
	})
}

func TestDecoratorsNamed(t *testing.T) {
	t.Parallel()

	t.Run("returns every application in order", func(t *testing.T) {
		t.Parallel()
		// A route documenting several responses applies the same
		// decorator once per status code; collapsing them would
		// describe an endpoint returning one status.
		s := decorated(
			typescript.Decorator{Name: "R", Args: "(200)"},
			typescript.Decorator{Name: "Other"},
			typescript.Decorator{Name: "R", Args: "(404)"},
		)
		got := typescript.DecoratorsNamed(s, "R")
		if len(got) != 2 {
			t.Fatalf("DecoratorsNamed = %d, want 2", len(got))
		}
		if got[0].Args != "(200)" || got[1].Args != "(404)" {
			t.Fatalf("args = %q, %q; want 200 then 404", got[0].Args, got[1].Args)
		}
	})

	t.Run("returns nothing for an unknown name", func(t *testing.T) {
		t.Parallel()
		s := decorated(typescript.Decorator{Name: "A"})
		if got := typescript.DecoratorsNamed(s, "B"); len(got) != 0 {
			t.Fatalf("DecoratorsNamed = %+v, want none", got)
		}
	})

	t.Run("works on any node kind, not just a struct", func(t *testing.T) {
		t.Parallel()
		// The accessors take contract.Node so they serve a field, a
		// method and a parameter as readily as a declaration.
		f := &node.Field{Name: "name"}
		typescript.MetaDecorators.Set(f.EnsureMeta(),
			[]typescript.Decorator{{Name: "Column", Args: "({})"}}, "test")

		if !typescript.HasDecorator(f, "Column") {
			t.Fatal("a field's decorator was not readable")
		}
	})
}
