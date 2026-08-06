// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// Package-level keys for the predicate tests. Declared once so the
// global key registry never sees re-registration on -count > 1.
var (
	keyPredicateUntyped = meta.NewKey("node.predicates.untyped", meta.BoolParser)
	keyPredicateTyped   = meta.NewKey("node.predicates.typed", meta.BoolParser)
)

func TestWithDirective(t *testing.T) {
	t.Parallel()

	t.Run("matches when the named directive is attached", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{
			BaseNode: node.BaseNode{
				DirectiveList: []*directive.Directive{directiveAt("mock", position.Pos{})},
			},
		}
		if !node.WithDirective("mock")(s) {
			t.Fatalf("predicate should match")
		}
	})

	t.Run("does not match when the directive is absent", func(t *testing.T) {
		t.Parallel()
		var s node.Struct
		if node.WithDirective("mock")(&s) {
			t.Fatalf("predicate should not match")
		}
	})
}

func TestWithMeta(t *testing.T) {
	t.Parallel()

	t.Run("matches when the named meta key is set", func(t *testing.T) {
		t.Parallel()
		var s node.Struct
		keyPredicateUntyped.Set(s.EnsureMeta(), true, "test")
		if !node.WithMeta(keyPredicateUntyped.Name())(&s) {
			t.Fatalf("predicate should match")
		}
	})

	t.Run("does not match when the meta key is absent", func(t *testing.T) {
		t.Parallel()
		var s node.Struct
		if node.WithMeta(keyPredicateUntyped.Name())(&s) {
			t.Fatalf("predicate should not match")
		}
	})
}

func TestWithMetaKey(t *testing.T) {
	t.Parallel()

	t.Run("matches when the typed key resolves to a value", func(t *testing.T) {
		t.Parallel()
		var s node.Struct
		keyPredicateTyped.Set(s.EnsureMeta(), true, "test")
		if !node.WithMetaKey(keyPredicateTyped)(&s) {
			t.Fatalf("predicate should match")
		}
	})

	t.Run("does not match when the typed key is absent", func(t *testing.T) {
		t.Parallel()
		var s node.Struct
		if node.WithMetaKey(keyPredicateTyped)(&s) {
			t.Fatalf("predicate should not match")
		}
	})
}

func TestAnd(t *testing.T) {
	t.Parallel()

	always := func(int) bool { return true }
	never := func(int) bool { return false }

	t.Run("empty preds match", func(t *testing.T) {
		t.Parallel()
		if !node.And[int]()(0) {
			t.Fatalf("And() with no preds should match")
		}
	})

	t.Run("all true matches", func(t *testing.T) {
		t.Parallel()
		nonOne := func(i int) bool { return i != 1 }
		if !node.And(always, nonOne)(0) {
			t.Fatalf("And of all-true preds should match")
		}
	})

	t.Run("any false rejects", func(t *testing.T) {
		t.Parallel()
		if node.And(always, never)(0) {
			t.Fatalf("And with a false pred should not match")
		}
	})
}

func TestOr(t *testing.T) {
	t.Parallel()

	always := func(int) bool { return true }
	never := func(int) bool { return false }

	t.Run("empty preds do not match", func(t *testing.T) {
		t.Parallel()
		if node.Or[int]()(0) {
			t.Fatalf("Or() with no preds should not match")
		}
	})

	t.Run("any true matches", func(t *testing.T) {
		t.Parallel()
		if !node.Or(never, always)(0) {
			t.Fatalf("Or with a true pred should match")
		}
	})

	t.Run("all false rejects", func(t *testing.T) {
		t.Parallel()
		isOne := func(i int) bool { return i == 1 }
		if node.Or(never, isOne)(0) {
			t.Fatalf("Or with all-false preds should not match")
		}
	})
}

func TestNot(t *testing.T) {
	t.Parallel()

	t.Run("inverts the wrapped predicate", func(t *testing.T) {
		t.Parallel()
		always := func(int) bool { return true }
		if node.Not(always)(0) {
			t.Fatalf("Not(always-true) should reject")
		}
		never := func(int) bool { return false }
		if !node.Not(never)(0) {
			t.Fatalf("Not(always-false) should match")
		}
	})
}
