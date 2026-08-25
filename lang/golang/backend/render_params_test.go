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

// TestRenderParams_Variadic covers the variadic-parameter branch of
// renderParams — `...` prefix on the last parameter's type.
func TestRenderParams_Variadic(t *testing.T) {
	t.Parallel()
	t.Run("variadic param renders with '...' prefix", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Interfaces: []*emit.Interface{{
				Name: "Printer", Package: "x", Target: target,
				Methods: []*emit.Method{
					{Name: "Print", Params: []*emit.Param{
						{Name: "args", Type: emit.Builtin("string"), Variadic: true},
					}},
				},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "Print(args ...string)") {
			t.Fatalf("variadic param mismatched; got:\n%s", body)
		}
	})
}

// TestRenderParams_MixedNamed covers the [ErrMixedNamedParams]
// validation: a parameter list mixing named and unnamed entries
// violates Go's grammar and surfaces as an Error diagnostic.
func TestRenderParams_MixedNamed(t *testing.T) {
	t.Parallel()

	t.Run("mixed-named param list produces ErrMixedNamedParams diagnostic", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Interfaces: []*emit.Interface{{
				Name: "I", Package: "x", Target: target,
				Methods: []*emit.Method{
					{Name: "M", Params: []*emit.Param{
						{Name: "a", Type: emit.Builtin("int")},
						{Type: emit.Builtin("string")},
					}},
				},
			}},
		})
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("render must not produce output on mixed-named params")
		}
		if !diagnosticsContain(d, diag.Error, "mixes named and unnamed") {
			t.Fatalf("expected ErrMixedNamedParams diagnostic; got %+v", d.Diagnostics())
		}
	})

	t.Run("ErrMixedNamedParams is exported", func(t *testing.T) {
		t.Parallel()
		if backend.ErrMixedNamedParams == nil {
			t.Fatalf("ErrMixedNamedParams must be exported and non-nil")
		}
		if !errors.Is(backend.ErrMixedNamedParams, backend.ErrMixedNamedParams) {
			t.Fatalf("ErrMixedNamedParams must satisfy errors.Is reflexivity")
		}
	})
}

// TestRenderReturns_NamedAnonymousVariants pins the four cases the
// renderReturns helper handles:
//
//  1. Zero returns → no clause.
//  2. One anonymous return → bare type.
//  3. One named return → parenthesised `name type`.
//  4. Two or more returns (named or anonymous) → parenthesised list.
//
// Mixed named/unnamed slots in a single signature surface as
// [emit.ErrMixedNamedReturns].
func TestRenderReturns_NamedAnonymousVariants(t *testing.T) {
	t.Parallel()

	t.Run("one anonymous return renders bare", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{{
				Name: "Single", Package: "x", Target: target,
				Returns: emit.AnonReturns(emit.Builtin("int")),
				Body: []*emit.Stmt{emit.NewReturn(&emit.Expr{
					ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "0",
				})},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "func Single() int {") {
			t.Fatalf("one anonymous return should render bare; got:\n%s", body)
		}
	})

	t.Run("one named return renders parenthesised name + type", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{{
				Name: "Counter", Package: "x", Target: target,
				Returns: []*emit.Return{{Name: "n", Type: emit.Builtin("int")}},
				Body: []*emit.Stmt{emit.NewReturn(&emit.Expr{
					ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "0",
				})},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "func Counter() (n int) {") {
			t.Fatalf("one named return should render parenthesised; got:\n%s", body)
		}
	})

	t.Run("multiple named returns render parenthesised list", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{{
				Name: "ReadAll", Package: "x", Target: target,
				Returns: []*emit.Return{
					{Name: "n", Type: emit.Builtin("int")},
					{Name: "err", Type: emit.Builtin("error")},
				},
				Body: []*emit.Stmt{emit.NewReturn(
					&emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "0"},
					&emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitNil},
				)},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "func ReadAll() (n int, err error) {") {
			t.Fatalf("two named returns should render parenthesised; got:\n%s", body)
		}
	})

	t.Run("mixed named and anonymous returns surface ErrMixedNamedReturns", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{{
				Name: "Mixed", Package: "x", Target: target,
				Returns: []*emit.Return{
					{Name: "n", Type: emit.Builtin("int")},
					{Type: emit.Builtin("error")},
				},
			}},
		})
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("mixed-named returns must suppress sink writes")
		}
		if !diagnosticsContain(d, diag.Error, "mix named and unnamed") {
			t.Fatalf("expected ErrMixedNamedReturns diagnostic; got %+v", d.Diagnostics())
		}
	})

	t.Run("ErrMixedNamedReturns is exported and satisfies errors.Is", func(t *testing.T) {
		t.Parallel()
		if emit.ErrMixedNamedReturns == nil {
			t.Fatalf("ErrMixedNamedReturns must be exported and non-nil")
		}
		if !errors.Is(emit.ErrMixedNamedReturns, emit.ErrMixedNamedReturns) {
			t.Fatalf("ErrMixedNamedReturns must satisfy errors.Is reflexivity")
		}
	})
}

// TestRenderReceiver_NilReceiver covers the renderReceiver branch
// for a method whose [emit.Method.Receiver] is nil — the helper
// returns the empty string so the method template can still render
// the method name without a receiver clause. Reachable when a
// cross-cutting plugin contributes a Method to a struct's
// `methods` slot without populating the receiver type.
func TestRenderReceiver_NilReceiver(t *testing.T) {
	t.Parallel()

	t.Run("slot method with nil receiver renders without receiver clause", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		host := &emit.Struct{Name: "Holder", Package: "x", Target: target}
		host.Methods = []*emit.Method{{Name: "NoReceiver"}}
		addEmitPackage(t, ctx, emitPackage("x", host))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		// The method template's `func{{ with renderReceiver . }} {{ . }}{{ end }} Name`
		// idiom drops the receiver clause entirely when renderReceiver
		// returns the empty string. gofmt collapses to `func NoReceiver()`.
		if !strings.Contains(string(body), "func NoReceiver()") {
			t.Fatalf("nil-receiver method must render without receiver clause; got:\n%s", body)
		}
	})
}
