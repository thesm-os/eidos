// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/emit"
)

// ErrUnsupportedExpr is returned by [renderState.renderExpr] when
// called with an [emit.ExprKind] or [emit.LiteralKind] variant the
// current funcmap can't render — typically an out-of-range
// discriminator value (every documented variant is wired). The
// wrapped message names the offending kind so diagnostics
// attribute the gap precisely.
var ErrUnsupportedExpr = errors.New("backend: unsupported Expr")

// renderExpr produces the Go source spelling for an [emit.Expr].
// Every documented [emit.ExprKind] variant is supported. Nil input
// returns the empty string so callers can place the helper
// directly into templates without explicit nil-guards on optional
// initialisers and sub-expressions.
//
// `renderExpr` is one of the reserved dispatch funcmap entries —
// plugin overrides are rejected at Build time.
func (s *renderState) renderExpr(e *emit.Expr) (string, error) {
	if e == nil {
		return "", nil
	}
	switch e.ExprKind {
	case emit.ExprLiteral:
		return renderLiteral(e)
	case emit.ExprIdent:
		return e.Name, nil
	case emit.ExprField:
		recv, err := s.renderExpr(e.Receiver)
		if err != nil {
			return "", err
		}
		return recv + "." + e.Name, nil
	case emit.ExprIndex:
		recv, err := s.renderExpr(e.Receiver)
		if err != nil {
			return "", err
		}
		idx, err := s.renderExpr(e.IndexExpr)
		if err != nil {
			return "", err
		}
		return recv + "[" + idx + "]", nil
	case emit.ExprSlice:
		return s.renderSliceExpr(e)
	case emit.ExprBinary:
		left, err := s.renderExpr(e.Left)
		if err != nil {
			return "", err
		}
		right, err := s.renderExpr(e.Right)
		if err != nil {
			return "", err
		}
		return left + " " + e.Op + " " + right, nil
	case emit.ExprUnary:
		operand, err := s.renderExpr(e.Receiver)
		if err != nil {
			return "", err
		}
		return e.Op + operand, nil
	case emit.ExprCall:
		return s.renderCall(e)
	case emit.ExprMethodCall:
		return s.renderMethodCall(e)
	case emit.ExprTypeAssert:
		recv, err := s.renderExpr(e.Receiver)
		if err != nil {
			return "", err
		}
		asType, err := s.renderType(e.AsType)
		if err != nil {
			return "", err
		}
		return recv + ".(" + asType + ")", nil
	case emit.ExprNew:
		asType, err := s.renderType(e.AsType)
		if err != nil {
			return "", err
		}
		return "new(" + asType + ")", nil
	case emit.ExprMake:
		return s.renderMake(e)
	case emit.ExprComposite:
		return s.renderCompositeLit(e, false)
	case emit.ExprCompositeKeyed:
		return s.renderCompositeLit(e, true)
	case emit.ExprFuncLit:
		return s.renderFuncLit(e)
	case emit.ExprParen:
		inner, err := s.renderExpr(e.Receiver)
		if err != nil {
			return "", err
		}
		return "(" + inner + ")", nil
	case emit.ExprDeref:
		operand, err := s.renderExpr(e.Receiver)
		if err != nil {
			return "", err
		}
		return "*" + operand, nil
	case emit.ExprAddr:
		operand, err := s.renderExpr(e.Receiver)
		if err != nil {
			return "", err
		}
		return "&" + operand, nil
	case emit.ExprRaw:
		return e.RawText, nil
	case emit.ExprExternal:
		// Cross-language frontends thread the source-language path
		// through e.Pkg; the bridge-imports map resolves it to the
		// Go-canonical import path with the same translation
		// [renderState.renderType] applies. Go-source pipelines see
		// no bridge meta and the path passes through verbatim.
		alias, err := s.imports.Imp(s.resolveImportPath(e.Pkg))
		if err != nil {
			return "", fmt.Errorf("backend/golang: renderExpr ExternalRef: %w", err)
		}
		if alias == "" {
			// Same-package elision: Imp returned the empty alias
			// because e.Pkg equals the rendered file's own import
			// path. Drop the qualifier and emit the bare symbol.
			return e.Name, nil
		}
		return alias + "." + e.Name, nil
	default:
		return "", fmt.Errorf("%w: ExprKind=%s", ErrUnsupportedExpr, e.ExprKind)
	}
}

// renderLiteral produces the Go source spelling for a single
// literal expression, dispatching on the [emit.LiteralKind].
// Strings are re-quoted via [strconv.Quote]; numeric and boolean
// literals render their raw text; nil renders as the keyword;
// runes wrap in single quotes; raw literals pass through.
func renderLiteral(e *emit.Expr) (string, error) {
	switch e.LitKind {
	case emit.LitString:
		return strconv.Quote(e.RawText), nil
	case emit.LitInt, emit.LitUint, emit.LitFloat:
		return e.RawText, nil
	case emit.LitBool:
		return e.RawText, nil
	case emit.LitNil:
		return "nil", nil
	case emit.LitRune:
		return "'" + e.RawText + "'", nil
	case emit.LitRaw:
		return e.RawText, nil
	default:
		return "", fmt.Errorf("%w: LitKind=%s", ErrUnsupportedExpr, e.LitKind)
	}
}

// renderSliceExpr produces a Go slice expression. Any of Low /
// High / Max may be nil to denote default bounds; the rendered
// form omits the corresponding slot. `Max` triggers the
// three-index form `recv[low:high:max]`; otherwise the two-index
// `recv[low:high]` is used.
func (s *renderState) renderSliceExpr(e *emit.Expr) (string, error) {
	recv, err := s.renderExpr(e.Receiver)
	if err != nil {
		return "", err
	}
	low, err := s.renderExpr(e.Low)
	if err != nil {
		return "", err
	}
	high, err := s.renderExpr(e.High)
	if err != nil {
		return "", err
	}
	if e.Max == nil {
		return recv + "[" + low + ":" + high + "]", nil
	}
	maxExpr, err := s.renderExpr(e.Max)
	if err != nil {
		return "", err
	}
	return recv + "[" + low + ":" + high + ":" + maxExpr + "]", nil
}

// renderCall produces a Go function-call expression with optional
// generic instantiation: `fn(args...)` or `fn[T1, T2](args...)`.
func (s *renderState) renderCall(e *emit.Expr) (string, error) {
	callee, err := s.renderExpr(e.Callee)
	if err != nil {
		return "", err
	}
	typeArgs, err := s.renderTypeArgs(e.TypeArgs)
	if err != nil {
		return "", err
	}
	args, err := s.renderExprList(e.Args)
	if err != nil {
		return "", err
	}
	return callee + typeArgs + "(" + args + ")", nil
}

// renderMethodCall produces the method-call form, including
// optional generic instantiation: `recv.method(args...)` or
// `recv.method[T1](args...)`.
func (s *renderState) renderMethodCall(e *emit.Expr) (string, error) {
	recv, err := s.renderExpr(e.Receiver)
	if err != nil {
		return "", err
	}
	typeArgs, err := s.renderTypeArgs(e.TypeArgs)
	if err != nil {
		return "", err
	}
	args, err := s.renderExprList(e.Args)
	if err != nil {
		return "", err
	}
	return recv + "." + e.Name + typeArgs + "(" + args + ")", nil
}

// renderMake produces a Go `make(T, args...)` call.
func (s *renderState) renderMake(e *emit.Expr) (string, error) {
	asType, err := s.renderType(e.AsType)
	if err != nil {
		return "", err
	}
	args, err := s.renderExprList(e.Args)
	if err != nil {
		return "", err
	}
	if args == "" {
		return "make(" + asType + ")", nil
	}
	return "make(" + asType + ", " + args + ")", nil
}

// renderCompositeLit produces a Go composite literal — positional
// form `T{a, b, c}` when keyed is false, keyed form
// `T{key1: a, key2: b}` when true. Distinct from
// [renderState.renderComposite] which handles type-level
// composites ([emit.CompositeRef]).
func (s *renderState) renderCompositeLit(e *emit.Expr, keyed bool) (string, error) {
	asType, err := s.renderType(e.AsType)
	if err != nil {
		return "", err
	}
	if !keyed {
		args, err := s.renderExprList(e.Args)
		if err != nil {
			return "", err
		}
		return asType + "{" + args + "}", nil
	}
	parts := make([]string, 0, len(e.Args))
	for i, arg := range e.Args {
		val, err := s.renderExpr(arg)
		if err != nil {
			return "", err
		}
		parts = append(parts, e.Keys[i]+": "+val)
	}
	return asType + "{" + strings.Join(parts, ", ") + "}", nil
}

// renderFuncLit produces a Go function-literal expression:
// `func(params) returns { body }`. The body is rendered through
// [renderState.renderStmt]; the params and returns through
// [renderState.renderParams] and [renderState.renderReturns].
func (s *renderState) renderFuncLit(e *emit.Expr) (string, error) {
	params, err := s.renderParams(e.FuncParams)
	if err != nil {
		return "", err
	}
	retText, err := s.renderReturns(emit.AnonReturns(e.FuncReturns...))
	if err != nil {
		return "", err
	}
	sig := "func" + params
	if retText != "" {
		sig += " " + retText
	}
	body, err := s.renderStmtBlock(e.FuncBody)
	if err != nil {
		return "", err
	}
	return sig + " {\n" + body + "}", nil
}

// renderExprList renders a comma-separated list of expressions.
// Empty input produces the empty string.
func (s *renderState) renderExprList(exprs []*emit.Expr) (string, error) {
	parts := make([]string, 0, len(exprs))
	for _, e := range exprs {
		r, err := s.renderExpr(e)
		if err != nil {
			return "", err
		}
		parts = append(parts, r)
	}
	return strings.Join(parts, ", "), nil
}

// renderTypeArgs renders the generic-instantiation bracket list
// `[T1, T2, ...]` for [emit.Expr.TypeArgs]. Empty input returns
// the empty string.
func (s *renderState) renderTypeArgs(typeArgs []emit.Ref) (string, error) {
	if len(typeArgs) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(typeArgs))
	for _, t := range typeArgs {
		r, err := s.renderType(t)
		if err != nil {
			return "", err
		}
		parts = append(parts, r)
	}
	return "[" + strings.Join(parts, ", ") + "]", nil
}
