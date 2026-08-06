// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

// Package-level meta keys for the predicate tests, declared once so
// the global registry never sees re-registration on -count > 1.
var (
	keyShapeDetected = meta.NewKey("store.predicates.shape", meta.BoolParser)
	keyMockOptions   = meta.NewKey("store.predicates.options", meta.StringParser)
)

func TestWithDirective(t *testing.T) {
	t.Parallel()

	t.Run("matches values whose Kind exposes HasDirective and the directive is attached", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		r := store.NewReader(s)
		got := r.Structs().Where(store.WithDirective[*node.Struct]("repo")).Slice()
		if len(got) != 1 || got[0].Name != "User" {
			t.Fatalf("WithDirective filter mismatch: %+v", got)
		}
	})

	t.Run("does not match when the directive is absent", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		r := store.NewReader(s)
		got := r.Structs().Where(store.WithDirective[*node.Struct]("missing")).Slice()
		if len(got) != 0 {
			t.Fatalf("WithDirective(missing) should return no matches; got %+v", got)
		}
	})
}

func TestWithMeta(t *testing.T) {
	t.Parallel()

	t.Run("matches values with the named key set", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		user, _ := s.Nodes().Structs().ByQName("github.com/example/users.User")
		keyShapeDetected.Set(user.EnsureMeta(), true, "test")
		r := store.NewReader(s)
		got := r.Structs().Where(store.WithMeta[*node.Struct](keyShapeDetected.Name())).Slice()
		if len(got) != 1 || got[0].Name != "User" {
			t.Fatalf("WithMeta filter mismatch: %+v", got)
		}
	})

	t.Run("does not match when the key is absent", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		r := store.NewReader(s)
		got := r.Structs().Where(store.WithMeta[*node.Struct]("never.set.key")).Slice()
		if len(got) != 0 {
			t.Fatalf("WithMeta on unset key should return no matches; got %+v", got)
		}
	})
}

func TestWithMetaKey(t *testing.T) {
	t.Parallel()

	t.Run("matches values where the typed key resolves", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		user, _ := s.Nodes().Structs().ByQName("github.com/example/users.User")
		keyShapeDetected.Set(user.EnsureMeta(), true, "test")
		r := store.NewReader(s)
		got := r.Structs().Where(store.WithMetaKey[*node.Struct](keyShapeDetected)).Slice()
		if len(got) != 1 || got[0].Name != "User" {
			t.Fatalf("WithMetaKey filter mismatch: %+v", got)
		}
	})
}

func TestMetaEq(t *testing.T) {
	t.Parallel()

	t.Run("matches values where the key resolves to the supplied value", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		user, _ := s.Nodes().Structs().ByQName("github.com/example/users.User")
		keyMockOptions.Set(user.EnsureMeta(), "exhaustive", "test")
		r := store.NewReader(s)
		got := r.Structs().Where(store.MetaEq[*node.Struct](keyMockOptions, "exhaustive")).Slice()
		if len(got) != 1 || got[0].Name != "User" {
			t.Fatalf("MetaEq filter mismatch: %+v", got)
		}
	})

	t.Run("does not match values whose value differs", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		user, _ := s.Nodes().Structs().ByQName("github.com/example/users.User")
		keyMockOptions.Set(user.EnsureMeta(), "exhaustive", "test")
		r := store.NewReader(s)
		got := r.Structs().Where(store.MetaEq[*node.Struct](keyMockOptions, "minimal")).Slice()
		if len(got) != 0 {
			t.Fatalf("MetaEq mismatch should not match; got %+v", got)
		}
	})

	t.Run("does not match when the key is unset", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		r := store.NewReader(s)
		got := r.Structs().Where(store.MetaEq[*node.Struct](keyMockOptions, "exhaustive")).Slice()
		if len(got) != 0 {
			t.Fatalf("MetaEq on unset key should not match; got %+v", got)
		}
	})
}

func TestAnd(t *testing.T) {
	t.Parallel()

	always := func(int) bool { return true }
	never := func(int) bool { return false }

	t.Run("empty preds match everything", func(t *testing.T) {
		t.Parallel()
		if !store.And[int]()(0) {
			t.Fatalf("And() with no preds should match")
		}
	})

	t.Run("all true matches", func(t *testing.T) {
		t.Parallel()
		nonOne := func(i int) bool { return i != 1 }
		if !store.And(always, nonOne)(0) {
			t.Fatalf("And of all-true preds should match")
		}
	})

	t.Run("any false rejects", func(t *testing.T) {
		t.Parallel()
		if store.And(always, never)(0) {
			t.Fatalf("And with a false pred should not match")
		}
	})
}

func TestOr(t *testing.T) {
	t.Parallel()

	always := func(int) bool { return true }
	never := func(int) bool { return false }

	t.Run("empty preds match nothing", func(t *testing.T) {
		t.Parallel()
		if store.Or[int]()(0) {
			t.Fatalf("Or() with no preds should not match")
		}
	})

	t.Run("any true matches", func(t *testing.T) {
		t.Parallel()
		if !store.Or(never, always)(0) {
			t.Fatalf("Or with a true pred should match")
		}
	})

	t.Run("all false rejects", func(t *testing.T) {
		t.Parallel()
		isOne := func(i int) bool { return i == 1 }
		if store.Or(never, isOne)(0) {
			t.Fatalf("Or with all-false preds should not match")
		}
	})
}

func TestNot(t *testing.T) {
	t.Parallel()

	t.Run("inverts the wrapped predicate", func(t *testing.T) {
		t.Parallel()
		always := func(int) bool { return true }
		if store.Not(always)(0) {
			t.Fatalf("Not(always-true) should reject")
		}
	})
}

func TestPredicateComposition(t *testing.T) {
	t.Parallel()

	t.Run("And of WithDirective and MetaEq narrows to the intersection", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		user, _ := s.Nodes().Structs().ByQName("github.com/example/users.User")
		keyMockOptions.Set(user.EnsureMeta(), "exhaustive", "test")
		r := store.NewReader(s)
		got := r.Structs().Where(store.And(
			store.WithDirective[*node.Struct]("repo"),
			store.MetaEq[*node.Struct](keyMockOptions, "exhaustive"),
		)).Slice()
		if len(got) != 1 || got[0].Name != "User" {
			t.Fatalf("And composition mismatch: %+v", got)
		}
	})
}
