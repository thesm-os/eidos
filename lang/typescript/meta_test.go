// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript_test

import (
	"errors"
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

func TestDecoratorsKey(t *testing.T) {
	t.Parallel()

	t.Run("round-trips a decorator list through the directive path", func(t *testing.T) {
		t.Parallel()
		// The parser exists for the directive-override path, where a
		// value arrives as text and the key has to decode it.
		s := &node.Struct{}
		raw := `[{"name":"Entity","args":"({ name: 'users' })"},{"name":"Index"}]`

		err := typescript.MetaDecorators.SetDirectiveFromString(
			s.EnsureMeta(), raw, position.Pos{},
		)
		if err != nil {
			t.Fatalf("SetDirectiveFromString: %v", err)
		}

		got := typescript.Decorators(s)
		if len(got) != 2 {
			t.Fatalf("decorators = %d, want 2", len(got))
		}
		if got[0].Name != "Entity" || got[0].Args != "({ name: 'users' })" {
			t.Fatalf("first = %+v", got[0])
		}
		if got[1].Name != "Index" || got[1].Args != "" {
			t.Fatalf("second = %+v, want a bare decorator", got[1])
		}
	})

	t.Run("an empty value decodes to no decorators", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{}
		if err := typescript.MetaDecorators.SetDirectiveFromString(
			s.EnsureMeta(), "", position.Pos{},
		); err != nil {
			t.Fatalf("SetDirectiveFromString(empty): %v", err)
		}
		if got := typescript.Decorators(s); len(got) != 0 {
			t.Fatalf("decorators = %+v, want none", got)
		}
	})

	t.Run("malformed input is rejected rather than silently dropped", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{}
		err := typescript.MetaDecorators.SetDirectiveFromString(
			s.EnsureMeta(), "not json", position.Pos{},
		)
		if err == nil {
			t.Fatal("malformed decorator list accepted")
		}
	})

	t.Run("a programmatic set survives a bag round trip", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{}
		want := []typescript.Decorator{{Name: "A", Args: "(1)"}}
		typescript.MetaDecorators.Set(s.EnsureMeta(), want, "test")

		got, ok := typescript.MetaDecorators.Get(s.Meta())
		if !ok || len(got) != 1 || got[0].Args != "(1)" {
			t.Fatalf("Get = %+v (present: %v)", got, ok)
		}
	})
}

func TestOverloadsKey(t *testing.T) {
	t.Parallel()

	t.Run("round-trips an overload list", func(t *testing.T) {
		t.Parallel()
		fn := &node.Function{}
		raw := `[{"text":"function f(a: string): void"},{"text":"function f(a: number): void"}]`

		err := typescript.MetaOverloads.SetDirectiveFromString(
			fn.EnsureMeta(), raw, position.Pos{},
		)
		if err != nil {
			t.Fatalf("SetDirectiveFromString: %v", err)
		}

		got, _ := typescript.MetaOverloads.Get(fn.Meta())
		if len(got) != 2 {
			t.Fatalf("overloads = %d, want 2", len(got))
		}
		if got[0].Text != "function f(a: string): void" {
			t.Fatalf("first = %q", got[0].Text)
		}
	})

	t.Run("an empty value decodes to no overloads", func(t *testing.T) {
		t.Parallel()
		fn := &node.Function{}
		if err := typescript.MetaOverloads.SetDirectiveFromString(
			fn.EnsureMeta(), "", position.Pos{},
		); err != nil {
			t.Fatalf("SetDirectiveFromString(empty): %v", err)
		}
		if got, _ := typescript.MetaOverloads.Get(fn.Meta()); len(got) != 0 {
			t.Fatalf("overloads = %+v, want none", got)
		}
	})

	t.Run("malformed input is rejected", func(t *testing.T) {
		t.Parallel()
		fn := &node.Function{}
		err := typescript.MetaOverloads.SetDirectiveFromString(
			fn.EnsureMeta(), "{oops}", position.Pos{},
		)
		if err == nil {
			t.Fatal("malformed overload list accepted")
		}
	})
}

func TestKeyRegistration(t *testing.T) {
	t.Parallel()

	t.Run("every ts key resolves through the shared registry", func(t *testing.T) {
		t.Parallel()
		// A key is interned by name, and the directive-override step
		// finds it by that name. One declared but unreachable would
		// accept a `+gen:meta` write nothing could read back.
		names := []string{
			"ts.decorators", "ts.overloads", "ts.exported", "ts.optional",
			"ts.readonly", "ts.visibility", "ts.heritage", "ts.typeText",
			"ts.reExport", "ts.definiteAssignment",
		}
		for _, name := range names {
			if _, err := meta.Lookup(name); err != nil {
				t.Errorf("Lookup(%q): %v", name, err)
			}
		}
	})

	t.Run("an unregistered name reports the sentinel", func(t *testing.T) {
		t.Parallel()
		_, err := meta.Lookup("ts.notAKey")
		if !errors.Is(err, meta.ErrUnregisteredKey) {
			t.Fatalf("Lookup(unknown) = %v, want ErrUnregisteredKey", err)
		}
	})
}
