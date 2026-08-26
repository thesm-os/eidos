// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"errors"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/typescript"
)

func TestRenderExpr(t *testing.T) {
	t.Parallel()

	t.Run("re-quotes a string literal", func(t *testing.T) {
		t.Parallel()
		// The generator wrote it in whatever quoting its own language
		// uses, and this file has one quote style.
		s := exprState(t)
		got, err := s.renderExpr(emit.NewLiteralString("hi"))
		if err != nil {
			t.Fatalf("renderExpr: %v", err)
		}
		if got != "'hi'" {
			t.Fatalf("renderExpr = %q, want 'hi'", got)
		}
	})

	t.Run("passes numeric and boolean literals through", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		for _, tc := range []struct {
			expr *emit.Expr
			want string
		}{
			{emit.NewLiteralInt(42), "42"},
			{emit.NewLiteralUint(7), "7"},
			{emit.NewLiteralBool(true), "true"},
			{emit.NewLiteralRaw("1 + 2"), "1 + 2"},
		} {
			got, err := s.renderExpr(tc.expr)
			if err != nil {
				t.Errorf("renderExpr: %v", err)
				continue
			}
			if got != tc.want {
				t.Errorf("renderExpr = %q, want %q", got, tc.want)
			}
		}
	})

	t.Run("nil renders as null", func(t *testing.T) {
		t.Parallel()
		// TypeScript has two absent values and strictNullChecks makes
		// the difference load-bearing; null is the one a value
		// crossing JSON becomes.
		s := exprState(t)
		got, _ := s.renderExpr(emit.NewLiteralNil())
		if got != typescript.TypeNull {
			t.Fatalf("renderExpr(nil literal) = %q, want null", got)
		}
	})

	t.Run("an identifier is made bindable", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		got, _ := s.renderExpr(&emit.Expr{ExprKind: emit.ExprIdent, Name: "class"})
		if got != "class_" {
			t.Fatalf("renderExpr = %q, want the reserved word suffixed", got)
		}
	})

	t.Run("a parenthesised expression keeps its parentheses", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		got, _ := s.renderExpr(&emit.Expr{
			ExprKind: emit.ExprParen,
			Receiver: emit.NewLiteralInt(1),
		})
		if got != "(1)" {
			t.Fatalf("renderExpr = %q, want (1)", got)
		}
	})

	t.Run("an external reference registers its import", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		got, _ := s.renderExpr(&emit.Expr{
			ExprKind: emit.ExprExternal, Pkg: "./consts", Name: "MAX",
		})
		if got != "MAX" {
			t.Fatalf("renderExpr = %q, want MAX", got)
		}
		if s.imports.Len() != 1 {
			t.Fatal("the external reference registered no import")
		}
	})

	t.Run("nil renders as nothing", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		if got, err := s.renderExpr(nil); got != "" || err != nil {
			t.Fatalf("renderExpr(nil) = %q, %v", got, err)
		}
	})

	t.Run("a runtime expression is refused rather than guessed at", func(t *testing.T) {
		t.Parallel()
		// This backend renders type declarations. A generator emitting
		// a call into TypeScript is emitting runtime code, and an
		// error naming the shape beats a plausible mis-rendering.
		s := exprState(t)
		_, err := s.renderExpr(&emit.Expr{ExprKind: emit.ExprCall})
		if !errors.Is(err, ErrUnsupportedExpr) {
			t.Fatalf("renderExpr(call) = %v, want ErrUnsupportedExpr", err)
		}
	})
}

func TestLiteralKinds(t *testing.T) {
	t.Parallel()

	t.Run("a rune literal is quoted like a string", func(t *testing.T) {
		t.Parallel()
		// TypeScript has no character type; a rune is a one-character
		// string.
		s := exprState(t)
		got, err := s.renderExpr(emit.NewLiteralRune("x"))
		if err != nil {
			t.Fatalf("renderExpr: %v", err)
		}
		if got != "'x'" {
			t.Fatalf("renderExpr = %q, want 'x'", got)
		}
	})

	t.Run("a float literal passes through", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		got, _ := s.renderExpr(emit.NewLiteralFloat(1.5))
		if got != "1.5" {
			t.Fatalf("renderExpr = %q", got)
		}
	})

	t.Run("an unknown literal kind is refused", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		_, err := s.renderExpr(&emit.Expr{
			ExprKind: emit.ExprLiteral,
			LitKind:  emit.LiteralKind(99),
		})
		if !errors.Is(err, ErrUnsupportedExpr) {
			t.Fatalf("renderExpr = %v, want ErrUnsupportedExpr", err)
		}
	})
}
