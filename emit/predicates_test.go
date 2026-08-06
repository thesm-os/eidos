// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// Package-level keys for the predicate tests. Declared once so the
// global key registry never sees re-registration on -count > 1.
var (
	keyEmitPredicateUntyped = meta.NewKey("emit.predicates.untyped", meta.BoolParser)
	keyEmitPredicateTyped   = meta.NewKey("emit.predicates.typed", meta.BoolParser)
)

func TestWithDirective(t *testing.T) {
	t.Parallel()

	t.Run("matches when the named directive is attached", func(t *testing.T) {
		t.Parallel()
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{
				DirectiveList: []*directive.Directive{directiveAt("mock", position.Pos{})},
			},
		}
		if !emit.WithDirective("mock")(s) {
			t.Fatalf("predicate should match")
		}
	})

	t.Run("does not match when the directive is absent", func(t *testing.T) {
		t.Parallel()
		var s emit.Struct
		if emit.WithDirective("mock")(&s) {
			t.Fatalf("predicate should not match")
		}
	})
}

func TestWithMeta(t *testing.T) {
	t.Parallel()

	t.Run("matches when the named meta key is set", func(t *testing.T) {
		t.Parallel()
		var s emit.Struct
		keyEmitPredicateUntyped.Set(s.EnsureMeta(), true, "test")
		if !emit.WithMeta(keyEmitPredicateUntyped.Name())(&s) {
			t.Fatalf("predicate should match")
		}
	})

	t.Run("does not match when the meta key is absent", func(t *testing.T) {
		t.Parallel()
		var s emit.Struct
		if emit.WithMeta(keyEmitPredicateUntyped.Name())(&s) {
			t.Fatalf("predicate should not match")
		}
	})
}

func TestWithMetaKey(t *testing.T) {
	t.Parallel()

	t.Run("matches when the typed key resolves to a value", func(t *testing.T) {
		t.Parallel()
		var s emit.Struct
		keyEmitPredicateTyped.Set(s.EnsureMeta(), true, "test")
		if !emit.WithMetaKey(keyEmitPredicateTyped)(&s) {
			t.Fatalf("predicate should match")
		}
	})

	t.Run("does not match when the typed key is absent", func(t *testing.T) {
		t.Parallel()
		var s emit.Struct
		if emit.WithMetaKey(keyEmitPredicateTyped)(&s) {
			t.Fatalf("predicate should not match")
		}
	})
}

func TestWithKind(t *testing.T) {
	t.Parallel()

	t.Run("matches when node Kind equals supplied kind", func(t *testing.T) {
		t.Parallel()
		s := &emit.Struct{Name: "X"}
		if !emit.WithKind(emit.KindStruct)(s) {
			t.Fatalf("WithKind should match a struct against KindStruct")
		}
	})

	t.Run("does not match for differing kinds", func(t *testing.T) {
		t.Parallel()
		s := &emit.Struct{Name: "X"}
		if emit.WithKind(emit.KindMethod)(s) {
			t.Fatalf("WithKind should not match a struct against KindMethod")
		}
	})
}

func TestWithOrigin(t *testing.T) {
	t.Parallel()

	t.Run("matches when Origin is non-nil", func(t *testing.T) {
		t.Parallel()
		s := &emit.Struct{BaseEmit: emit.BaseEmit{OriginNode: &node.Struct{Name: "Source"}}}
		if !emit.WithOrigin()(s) {
			t.Fatalf("WithOrigin should match a derived emit value")
		}
	})

	t.Run("does not match for synthetic emit values", func(t *testing.T) {
		t.Parallel()
		s := &emit.Struct{Name: "X"}
		if emit.WithOrigin()(s) {
			t.Fatalf("WithOrigin should not match a synthetic emit value")
		}
	})
}

func TestAnd(t *testing.T) {
	t.Parallel()

	always := func(int) bool { return true }
	never := func(int) bool { return false }

	t.Run("empty preds match everything", func(t *testing.T) {
		t.Parallel()
		if !emit.And[int]()(0) {
			t.Fatalf("And() with no preds should match")
		}
	})

	t.Run("all true matches", func(t *testing.T) {
		t.Parallel()
		nonOne := func(i int) bool { return i != 1 }
		if !emit.And(always, nonOne)(0) {
			t.Fatalf("And of all-true preds should match")
		}
	})

	t.Run("any false rejects", func(t *testing.T) {
		t.Parallel()
		if emit.And(always, never)(0) {
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
		if emit.Or[int]()(0) {
			t.Fatalf("Or() with no preds should not match")
		}
	})

	t.Run("any true matches", func(t *testing.T) {
		t.Parallel()
		if !emit.Or(never, always)(0) {
			t.Fatalf("Or with a true pred should match")
		}
	})

	t.Run("all false rejects", func(t *testing.T) {
		t.Parallel()
		isOne := func(i int) bool { return i == 1 }
		if emit.Or(never, isOne)(0) {
			t.Fatalf("Or with all-false preds should not match")
		}
	})
}

func TestNot(t *testing.T) {
	t.Parallel()

	t.Run("inverts the wrapped predicate", func(t *testing.T) {
		t.Parallel()
		always := func(int) bool { return true }
		if emit.Not(always)(0) {
			t.Fatalf("Not(always-true) should reject")
		}
		never := func(int) bool { return false }
		if !emit.Not(never)(0) {
			t.Fatalf("Not(always-false) should match")
		}
	})
}
