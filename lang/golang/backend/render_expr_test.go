// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend_test

import (
	"errors"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang/backend"
)

// TestRenderExpr_LiteralKinds pins each [emit.LiteralKind] variant
// the funcmap currently supports against its rendered form. Uniform
// "literal in → rendered string out" mapping makes a table test the
// natural fit.
func TestRenderExpr_LiteralKinds(t *testing.T) {
	t.Parallel()

	lit := func(kind emit.LiteralKind, raw string) *emit.Expr {
		return &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: kind, RawText: raw}
	}
	cases := []struct {
		name string
		lit  *emit.Expr
		want string
	}{
		{"string is re-quoted", lit(emit.LitString, "hi"), "\"hi\""},
		{"int passes through raw", lit(emit.LitInt, "42"), "42"},
		{"uint passes through raw", lit(emit.LitUint, "42"), "42"},
		{"float passes through raw", lit(emit.LitFloat, "1.5"), "1.5"},
		{"bool passes through raw", lit(emit.LitBool, "false"), "false"},
		{"nil renders the keyword", lit(emit.LitNil, ""), "nil"},
		{"rune wraps in single quotes", lit(emit.LitRune, "a"), "'a'"},
		{"raw passes through verbatim", lit(emit.LitRaw, "0x1p3"), "0x1p3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := renderConstantValue(t, tc.lit)
			want := "const K = " + tc.want
			if !strings.Contains(body, want) {
				t.Fatalf("body should contain %q; got:\n%s", want, body)
			}
		})
	}
}

// renderConstantValue builds a single-constant fixture whose Value
// is the supplied expression, renders it, and returns the rendered
// file body. Centralised so the per-ExprKind tests stay terse.
func renderConstantValue(t *testing.T, value *emit.Expr) string {
	t.Helper()
	ctx, mem, d := newBackendContext(t)
	target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
	addEmitPackage(t, ctx, &emit.Package{
		Name: "x", Path: "x",
		Constants: []*emit.Constant{{
			Name: "K", Package: "x", Target: target,
			Value: value,
		}},
	})
	body := assertRenderSucceeds(t, ctx, mem, d, target)
	return string(body)
}

// TestRenderExpr_Variants covers every documented [emit.ExprKind]
// variant via the public render path. Each variant is exercised
// through a Variable initialiser so the assertion rides the full
// template + gofmt pipeline.
func TestRenderExpr_Variants(t *testing.T) {
	t.Parallel()

	ident := func(name string) *emit.Expr {
		return &emit.Expr{ExprKind: emit.ExprIdent, Name: name}
	}
	intLit := func(s string) *emit.Expr {
		return &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: s}
	}

	cases := []struct {
		name string
		expr *emit.Expr
		want string
	}{
		{
			name: "field selector",
			expr: &emit.Expr{ExprKind: emit.ExprField, Receiver: ident("u"), Name: "ID"},
			want: "u.ID",
		},
		{
			name: "index expression",
			expr: &emit.Expr{ExprKind: emit.ExprIndex, Receiver: ident("arr"), IndexExpr: intLit("0")},
			want: "arr[0]",
		},
		{
			name: "two-index slice",
			expr: &emit.Expr{
				ExprKind: emit.ExprSlice, Receiver: ident("arr"),
				Low: intLit("1"), High: intLit("3"),
			},
			want: "arr[1:3]",
		},
		{
			name: "three-index slice with Max",
			expr: &emit.Expr{
				ExprKind: emit.ExprSlice, Receiver: ident("arr"),
				Low: intLit("1"), High: intLit("3"), Max: intLit("5"),
			},
			want: "arr[1:3:5]",
		},
		{
			name: "binary expression",
			expr: &emit.Expr{ExprKind: emit.ExprBinary, Op: "+", Left: intLit("1"), Right: intLit("2")},
			want: "1 + 2",
		},
		{
			name: "unary expression",
			expr: &emit.Expr{ExprKind: emit.ExprUnary, Op: "-", Receiver: intLit("5")},
			want: "-5",
		},
		{
			name: "function call",
			expr: &emit.Expr{ExprKind: emit.ExprCall, Callee: ident("f"), Args: []*emit.Expr{intLit("1")}},
			want: "f(1)",
		},
		{
			name: "method call",
			expr: &emit.Expr{
				ExprKind: emit.ExprMethodCall,
				Receiver: ident("r"),
				Name:     "Do",
				Args:     []*emit.Expr{intLit("1")},
			},
			want: "r.Do(1)",
		},
		{
			name: "type assertion",
			expr: &emit.Expr{ExprKind: emit.ExprTypeAssert, Receiver: ident("x"), AsType: emit.Builtin("int")},
			want: "x.(int)",
		},
		{
			name: "new builtin",
			expr: &emit.Expr{ExprKind: emit.ExprNew, AsType: emit.Builtin("int")},
			want: "new(int)",
		},
		{
			name: "make slice with size",
			expr: &emit.Expr{
				ExprKind: emit.ExprMake,
				AsType:   emit.SliceOf(emit.Builtin("int")),
				Args:     []*emit.Expr{intLit("4")},
			},
			want: "make([]int, 4)",
		},
		{
			name: "make map no args",
			expr: &emit.Expr{
				ExprKind: emit.ExprMake,
				AsType:   emit.MapOf(emit.Builtin("string"), emit.Builtin("int")),
			},
			want: "make(map[string]int)",
		},
		{
			name: "composite literal positional",
			expr: &emit.Expr{
				ExprKind: emit.ExprComposite,
				AsType:   emit.SliceOf(emit.Builtin("int")),
				Args:     []*emit.Expr{intLit("1"), intLit("2"), intLit("3")},
			},
			want: "[]int{1, 2, 3}",
		},
		{
			name: "composite literal keyed",
			expr: &emit.Expr{
				ExprKind: emit.ExprCompositeKeyed,
				AsType:   emit.MapOf(emit.Builtin("string"), emit.Builtin("int")),
				Keys:     []string{`"a"`, `"b"`},
				Args:     []*emit.Expr{intLit("1"), intLit("2")},
			},
			want: `map[string]int{"a": 1, "b": 2}`,
		},
		{
			name: "parenthesised expression",
			expr: &emit.Expr{
				ExprKind: emit.ExprParen,
				Receiver: &emit.Expr{ExprKind: emit.ExprBinary, Op: "+", Left: intLit("1"), Right: intLit("2")},
			},
			want: "(1 + 2)",
		},
		{
			name: "dereference",
			expr: &emit.Expr{ExprKind: emit.ExprDeref, Receiver: ident("p")},
			want: "*p",
		},
		{
			name: "address-of",
			expr: &emit.Expr{ExprKind: emit.ExprAddr, Receiver: ident("v")},
			want: "&v",
		},
		{
			name: "raw verbatim",
			expr: &emit.Expr{ExprKind: emit.ExprRaw, RawText: `unsafe.Pointer(nil)`},
			want: "unsafe.Pointer(nil)",
		},
		{
			name: "generic call with type args",
			expr: &emit.Expr{
				ExprKind: emit.ExprCall,
				Callee:   ident("f"),
				TypeArgs: []emit.Ref{emit.Builtin("int")},
				Args:     []*emit.Expr{intLit("1")},
			},
			want: "f[int](1)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := renderVariableInit(t, tc.expr)
			if !strings.Contains(body, tc.want) {
				t.Fatalf("rendered body should contain %q; got:\n%s", tc.want, body)
			}
		})
	}
}

// renderVariableInit builds a Variable whose Init is the supplied
// expression, renders the file, and returns the body string. The
// inferred-type form `var V = <init>` keeps the rendered line
// minimal so each per-ExprKind assertion stays scannable.
func renderVariableInit(t *testing.T, init *emit.Expr) string {
	t.Helper()
	ctx, mem, d := newBackendContext(t)
	target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
	addEmitPackage(t, ctx, &emit.Package{
		Name: "x", Path: "x",
		Variables: []*emit.Variable{{
			Name: "V", Package: "x", Target: target,
			Init: init,
		}},
	})
	body := assertRenderSucceeds(t, ctx, mem, d, target)
	return string(body)
}

// TestRenderExpr_NilGuard pins the nil-input guard: a Variable
// constructed without an Init renders with no `= …` clause. The
// renderExpr helper returns the empty string when invoked with nil,
// which the variable template skips via its `{{ if .Init }}` guard;
// the resulting output omits the assignment entirely.
func TestRenderExpr_NilGuard(t *testing.T) {
	t.Parallel()
	t.Run("variable without Init renders with no assignment", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Variables: []*emit.Variable{{
				Name: "X", Package: "x", Target: target,
				Type: emit.Builtin("int"),
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if strings.Contains(string(body), "=") {
			t.Fatalf("uninitialised var should have no '='; got:\n%s", body)
		}
	})
}

// TestRenderExpr_UnknownKind covers the funcmap's default branch:
// invented or future ExprKind values (beyond the documented
// variants) surface as an Error diagnostic via ErrUnsupportedExpr.
// Today every documented kind is wired, so this branch is
// reachable only via an out-of-range discriminator value.
func TestRenderExpr_UnknownKind(t *testing.T) {
	t.Parallel()

	t.Run("out-of-range ExprKind returns ErrUnsupportedExpr", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Variables: []*emit.Variable{{
				Name: "X", Package: "x", Target: target,
				Init: &emit.Expr{ExprKind: emit.ExprKind(9999)},
			}},
		})
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("render must not produce output on unknown expr kind")
		}
		if !diagnosticsContain(d, diag.Error, "unsupported Expr") {
			t.Fatalf("expected ErrUnsupportedExpr diagnostic; got %+v", d.Diagnostics())
		}
	})

	t.Run("ErrUnsupportedExpr is exported", func(t *testing.T) {
		t.Parallel()
		if backend.ErrUnsupportedExpr == nil {
			t.Fatalf("ErrUnsupportedExpr must be exported and non-nil")
		}
		if !errors.Is(backend.ErrUnsupportedExpr, backend.ErrUnsupportedExpr) {
			t.Fatalf("ErrUnsupportedExpr must satisfy errors.Is reflexivity")
		}
	})

	t.Run("out-of-range LitKind returns ErrUnsupportedExpr", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Variables: []*emit.Variable{{
				Name: "X", Package: "x", Target: target,
				Init: &emit.Expr{
					ExprKind: emit.ExprLiteral,
					LitKind:  emit.LiteralKind(9999),
					RawText:  "?",
				},
			}},
		})
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("render must not produce output on unknown literal kind")
		}
		if !diagnosticsContain(d, diag.Error, "unsupported Expr") {
			t.Fatalf("expected ErrUnsupportedExpr diagnostic; got %+v", d.Diagnostics())
		}
	})
}
