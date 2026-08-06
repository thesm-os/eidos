// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"errors"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/backend/golang"
	"go.thesmos.sh/eidos/emit"
)

// TestErrUnsupportedStmt covers the sentinel surfaced when
// renderStmt encounters an out-of-range StmtKind value. Every
// documented variant is wired, so the only path to this sentinel
// today is an invented discriminator value or a future variant
// not yet handled.
func TestErrUnsupportedStmt(t *testing.T) {
	t.Parallel()

	t.Run("ErrUnsupportedStmt is exported and satisfies errors.Is", func(t *testing.T) {
		t.Parallel()
		if golang.ErrUnsupportedStmt == nil {
			t.Fatalf("ErrUnsupportedStmt must be exported and non-nil")
		}
		if !errors.Is(golang.ErrUnsupportedStmt, golang.ErrUnsupportedStmt) {
			t.Fatalf("ErrUnsupportedStmt must satisfy errors.Is reflexivity")
		}
	})
}

// TestRenderStmt_Variants covers every documented [emit.StmtKind]
// variant via the public render path. Each variant is exercised
// through a Variable whose Init is an [emit.ExprFuncLit] containing
// the statement; the func literal goes through renderExpr →
// renderFuncLit → renderStmtBlock → renderStmt, and the entire
// path is gofmt-validated by the file-level finalise step.
//
// The fixtures are minimal — one statement per case — so the
// rendered body keeps a stable, scannable shape.
func TestRenderStmt_Variants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body []*emit.Stmt
		want string // substring that must appear in the rendered file
	}{
		{
			name: "expression statement (bare call)",
			body: []*emit.Stmt{emit.NewExprStmt(&emit.Expr{
				ExprKind: emit.ExprCall,
				Callee:   &emit.Expr{ExprKind: emit.ExprIdent, Name: "f"},
			})},
			want: "f()",
		},
		{
			name: "assignment with `=`",
			body: []*emit.Stmt{emit.NewAssign(
				[]*emit.Expr{{ExprKind: emit.ExprIdent, Name: "x"}},
				"=",
				[]*emit.Expr{{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "1"}},
			)},
			want: "x = 1",
		},
		{
			name: "short variable declaration `:=`",
			body: []*emit.Stmt{emit.NewAssign(
				[]*emit.Expr{{ExprKind: emit.ExprIdent, Name: "x"}},
				":=",
				[]*emit.Expr{{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "1"}},
			)},
			want: "x := 1",
		},
		{
			name: "return with no values",
			body: []*emit.Stmt{emit.NewReturn()},
			want: "return",
		},
		{
			name: "return with one value",
			body: []*emit.Stmt{emit.NewReturn(&emit.Expr{
				ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "42",
			})},
			want: "return 42",
		},
		{
			name: "if/else",
			body: []*emit.Stmt{ifElseFixture()},
			want: "if ok {",
		},
		{
			name: "if with init clause",
			body: []*emit.Stmt{emit.NewIfInit(
				emit.NewAssign(
					[]*emit.Expr{{ExprKind: emit.ExprIdent, Name: "x"}},
					":=",
					[]*emit.Expr{{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "5"}},
				),
				&emit.Expr{
					ExprKind: emit.ExprBinary, Op: ">",
					Left:  &emit.Expr{ExprKind: emit.ExprIdent, Name: "x"},
					Right: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "0"},
				},
				[]*emit.Stmt{emit.NewReturn()},
				nil,
			)},
			want: "if x := 5; x > 0 {",
		},
		{
			name: "if/else-if chain",
			body: []*emit.Stmt{emit.NewIfElse(
				&emit.Expr{ExprKind: emit.ExprIdent, Name: "a"},
				[]*emit.Stmt{emit.NewReturn()},
				[]*emit.Stmt{emit.NewIfElse(
					&emit.Expr{ExprKind: emit.ExprIdent, Name: "b"},
					[]*emit.Stmt{emit.NewReturn()},
					[]*emit.Stmt{emit.NewReturn()},
				)},
			)},
			want: "} else if b {",
		},
		{
			name: "for with condition only",
			body: []*emit.Stmt{emit.NewFor(
				&emit.Expr{ExprKind: emit.ExprIdent, Name: "cond"},
				[]*emit.Stmt{emit.NewBreak("")},
			)},
			want: "for cond {",
		},
		{
			name: "for infinite",
			body: []*emit.Stmt{emit.NewFor(nil, []*emit.Stmt{emit.NewBreak("")})},
			want: "for {",
		},
		{
			name: "for range with key + value",
			body: []*emit.Stmt{emit.NewForRange(
				"k", "v",
				&emit.Expr{ExprKind: emit.ExprIdent, Name: "items"},
				nil,
			)},
			want: "for k, v := range items {",
		},
		{
			name: "for with init + cond + post (C-style)",
			body: []*emit.Stmt{emit.NewForFull(
				emit.NewAssign(
					[]*emit.Expr{{ExprKind: emit.ExprIdent, Name: "i"}},
					":=",
					[]*emit.Expr{{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "0"}},
				),
				&emit.Expr{
					ExprKind: emit.ExprBinary, Op: "<",
					Left:  &emit.Expr{ExprKind: emit.ExprIdent, Name: "i"},
					Right: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "10"},
				},
				emit.NewAssign(
					[]*emit.Expr{{ExprKind: emit.ExprIdent, Name: "i"}},
					"+=",
					[]*emit.Expr{{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "1"}},
				),
				[]*emit.Stmt{emit.NewBreak("")},
			)},
			want: "for i := 0; i < 10; i += 1 {",
		},
		{
			name: "for range key only",
			body: []*emit.Stmt{emit.NewForRange(
				"k", "",
				&emit.Expr{ExprKind: emit.ExprIdent, Name: "items"},
				nil,
			)},
			want: "for k := range items {",
		},
		{
			name: "for range value only renders blank key",
			body: []*emit.Stmt{emit.NewForRange(
				"", "v",
				&emit.Expr{ExprKind: emit.ExprIdent, Name: "items"},
				nil,
			)},
			want: "for _, v := range items {",
		},
		{
			name: "for range bare (no key, no value)",
			body: []*emit.Stmt{emit.NewForRange(
				"", "",
				&emit.Expr{ExprKind: emit.ExprIdent, Name: "items"},
				nil,
			)},
			want: "for range items {",
		},
		{
			name: "switch with case + default",
			body: []*emit.Stmt{emit.NewSwitch(
				&emit.Expr{ExprKind: emit.ExprIdent, Name: "x"},
				[]*emit.Stmt{
					emit.NewCase(
						[]*emit.Expr{{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "1"}},
						[]*emit.Stmt{emit.NewBreak("")},
					),
					emit.NewDefault([]*emit.Stmt{emit.NewReturn()}),
				},
			)},
			want: "switch x {",
		},
		{
			name: "switch with init clause",
			body: []*emit.Stmt{emit.NewSwitchInit(
				emit.NewAssign(
					[]*emit.Expr{{ExprKind: emit.ExprIdent, Name: "x"}},
					":=",
					[]*emit.Expr{{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "1"}},
				),
				&emit.Expr{ExprKind: emit.ExprIdent, Name: "x"},
				[]*emit.Stmt{emit.NewDefault([]*emit.Stmt{emit.NewReturn()})},
			)},
			want: "switch x := 1; x {",
		},
		{
			name: "bare switch (no cond)",
			body: []*emit.Stmt{emit.NewSwitch(
				nil,
				[]*emit.Stmt{emit.NewCase(
					[]*emit.Expr{{ExprKind: emit.ExprIdent, Name: "ok"}},
					[]*emit.Stmt{emit.NewReturn()},
				)},
			)},
			want: "switch {",
		},
		{
			name: "defer call",
			body: []*emit.Stmt{emit.NewDefer(&emit.Expr{
				ExprKind: emit.ExprCall,
				Callee:   &emit.Expr{ExprKind: emit.ExprIdent, Name: "cleanup"},
			})},
			want: "defer cleanup()",
		},
		{
			name: "go call",
			body: []*emit.Stmt{emit.NewGo(&emit.Expr{
				ExprKind: emit.ExprCall,
				Callee:   &emit.Expr{ExprKind: emit.ExprIdent, Name: "worker"},
			})},
			want: "go worker()",
		},
		{
			name: "break with label",
			body: []*emit.Stmt{emit.NewLabel("loop", emit.NewFor(nil, []*emit.Stmt{
				emit.NewBreak("loop"),
			}))},
			want: "break loop",
		},
		{
			name: "continue with label",
			body: []*emit.Stmt{emit.NewLabel("loop", emit.NewFor(nil, []*emit.Stmt{
				emit.NewContinue("loop"),
			}))},
			want: "continue loop",
		},
		{
			name: "local var",
			body: []*emit.Stmt{emit.NewVarStmt(
				"x",
				emit.Builtin("int"),
				&emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "5"},
			)},
			want: "var x int = 5",
		},
		{
			name: "local const",
			body: []*emit.Stmt{emit.NewConstStmt(
				"Pi",
				nil,
				&emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitFloat, RawText: "3.14"},
			)},
			want: "const Pi = 3.14",
		},
		{
			name: "raw verbatim text",
			body: []*emit.Stmt{emit.NewRawStmt(`println("hello")`)},
			want: `println("hello")`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := renderFuncLitBody(t, tc.body)
			if !strings.Contains(body, tc.want) {
				t.Fatalf("rendered body should contain %q; got:\n%s", tc.want, body)
			}
		})
	}
}

// ifElseFixture builds an if/else statement returning a bool
// literal in each branch. Extracted from the per-variant table
// because its line is otherwise too long for the formatter.
func ifElseFixture() *emit.Stmt {
	return emit.NewIfElse(
		&emit.Expr{ExprKind: emit.ExprIdent, Name: "ok"},
		[]*emit.Stmt{emit.NewReturn(&emit.Expr{
			ExprKind: emit.ExprLiteral, LitKind: emit.LitBool, RawText: "true",
		})},
		[]*emit.Stmt{emit.NewReturn(&emit.Expr{
			ExprKind: emit.ExprLiteral, LitKind: emit.LitBool, RawText: "false",
		})},
	)
}

// renderFuncLitBody builds a Variable whose Init is a func-literal
// expression carrying the supplied statements as its body. The
// returned string is the rendered file body. Used by
// [TestRenderStmt_Variants] to exercise every StmtKind through the
// public render path.
// TestRenderStmt_Render covers the [emit.StmtRender] variant, whose
// spelling comes from the wrapped node's own template rather than
// from the statement union.
//
// The variant exists so a plugin can contribute into a
// KindStmt-constrained slot while rendering through its own template.
// That path is exercised end to end by the reference contributors;
// what belongs here is the backend's half — that the wrapper
// dispatches on the wrapped node's Kind, and that the two ways it can
// be built wrong are reported rather than silently dropped.
func TestRenderStmt_Render(t *testing.T) {
	t.Parallel()

	t.Run("renders the wrapped node through that node's template", func(t *testing.T) {
		t.Parallel()
		wrapped := &emit.Constant{
			Name:  "limit",
			Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "10"},
		}
		body := renderFuncLitBody(t, []*emit.Stmt{emit.NewRenderStmt(wrapped)})
		if !strings.Contains(body, "const limit = 10") {
			t.Fatalf("wrapped node should render via emit.constant; got:\n%s", body)
		}
	})

	// A wrapper with nothing to delegate to means the contributing
	// plugin built the value wrong. Rendering nothing would drop that
	// plugin's contribution from the output with no signal at all.
	t.Run("reports a nil wrapped node instead of rendering nothing", func(t *testing.T) {
		t.Parallel()
		ctx, _, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, funcLitPackage(target, []*emit.Stmt{emit.NewRenderStmt(nil)}))

		err := mustNew(t).Render(ctx)
		if err == nil && !d.HasErrors() {
			t.Fatalf("a nil wrapped node must not render silently")
		}
		if err != nil && !errors.Is(err, golang.ErrUnsupportedStmt) {
			t.Fatalf("Render error should wrap ErrUnsupportedStmt; got %v", err)
		}
	})

	// The wrapped node names its template by Kind. A plugin that
	// declares a kind but forgets to ship the template gets a named
	// miss, not a blank line in someone else's file.
	t.Run("reports a wrapped kind with no registered template", func(t *testing.T) {
		t.Parallel()
		ctx, _, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		wrapped := &emit.Field{Name: "Unrendered"}
		addEmitPackage(t, ctx, funcLitPackage(target, []*emit.Stmt{emit.NewRenderStmt(wrapped)}))

		err := mustNew(t).Render(ctx)
		if err == nil && !d.HasErrors() {
			t.Fatalf("a wrapped kind with no template must not render silently")
		}
		if err != nil && !errors.Is(err, golang.ErrTemplateMissing) {
			t.Fatalf("Render error should wrap ErrTemplateMissing; got %v", err)
		}
	})
}

func renderFuncLitBody(t *testing.T, stmts []*emit.Stmt) string {
	t.Helper()
	ctx, mem, d := newBackendContext(t)
	target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
	addEmitPackage(t, ctx, funcLitPackage(target, stmts))
	body := assertRenderSucceeds(t, ctx, mem, d, target)
	return string(body)
}
