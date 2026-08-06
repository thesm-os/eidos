// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"fmt"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
)

// TestChain covers the chain-builder — every chain method extends
// the cursor, and Build returns the accumulated expression. The
// case names describe the final shape produced by the chain.
func TestChain(t *testing.T) {
	t.Parallel()

	t.Run("Sel / Call / Index / TypeAssert build a deep chain", func(t *testing.T) {
		t.Parallel()
		got := builder.Chain(builder.Ident("db")).
			Sel("Conn").
			Call("Query", builder.Str("SELECT *")).
			Index(builder.Int(0)).
			TypeAssert(emit.Builtin("string")).
			Build()
		if got.ExprKind != emit.ExprTypeAssert {
			t.Fatalf("outermost = %v, want ExprTypeAssert", got.ExprKind)
		}
		idx := got.Receiver
		if idx == nil || idx.ExprKind != emit.ExprIndex {
			t.Fatalf("expected ExprIndex one layer in; got %+v", idx)
		}
		call := idx.Receiver
		if call == nil || call.ExprKind != emit.ExprMethodCall {
			t.Fatalf("expected ExprMethodCall two layers in; got %+v", call)
		}
		sel := call.Receiver
		if sel == nil || sel.ExprKind != emit.ExprField {
			t.Fatalf("expected ExprField three layers in; got %+v", sel)
		}
	})

	t.Run("Deref and Addr nest the cursor", func(t *testing.T) {
		t.Parallel()
		// `*&x` round-trip exercises both Chain prefix-mode methods.
		got := builder.Chain(builder.Ident("x")).Addr().Deref().Build()
		if got.ExprKind != emit.ExprDeref {
			t.Fatalf("outer should be Deref; got %v", got.ExprKind)
		}
		inner := got.Receiver
		if inner == nil || inner.ExprKind != emit.ExprAddr {
			t.Fatalf("inner should be Addr; got %+v", inner)
		}
	})
}

// ExampleChain builds the expression `db.Query("SELECT id FROM
// users").Rows[0]` and then walks the result back out.
//
// The walk is the argument for the helper. [emit.Expr] nests
// outermost-first, so the structured equivalent of the chain above
// has to be written inside-out — `emit.NewIndex(emit.NewField(
// emit.NewMethodCall(emit.NewIdent("db"), ...)))` — reversing the
// order the generated code reads. Chain accumulates a cursor left to
// right so the call site matches the emitted source; the unwinding
// printed below is what a reader otherwise has to invert by hand.
func ExampleChain() {
	expr := builder.Chain(builder.Ident("db")).
		Call("Query", builder.Str("SELECT id FROM users")).
		Sel("Rows").
		Index(builder.Int(0)).
		Build()

	// Expr.Name carries the selector for a field access and the
	// method name for a call; an index expression names nothing and
	// keeps its subscript in Expr.IndexExpr.
	for cur := expr; cur != nil; cur = cur.Receiver {
		fmt.Printf("%s %q\n", cur.ExprKind, cur.Name)
	}

	// Output:
	// index ""
	// field "Rows"
	// method_call "Query"
	// ident "db"
}
