// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"errors"
	"fmt"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/typescript"
)

// ErrUnsupportedExpr reports an expression shape this backend cannot
// spell.
//
// Deliberately narrow. A type declaration is what this backend
// renders, and the expressions reaching it are the values a constant
// or an enum member carries — literals and names. A generator
// emitting a call or a composite literal into TypeScript is emitting
// runtime code, which this package does not claim to render, and an
// error naming the shape is a better answer than a plausible
// mis-rendering.
var ErrUnsupportedExpr = errors.New("backend: cannot render expression")

// renderExpr spells a value expression.
func (s *renderState) renderExpr(e *emit.Expr) (string, error) {
	if e == nil {
		return "", nil
	}

	switch e.ExprKind {
	case emit.ExprLiteral:
		return literalText(e)
	case emit.ExprIdent:
		return typescript.Ident(e.Name), nil
	case emit.ExprRaw:
		// Raw is the generator saying "emit this verbatim". Taking it
		// at its word is the whole point of the kind.
		return e.RawText, nil
	case emit.ExprExternal:
		return s.imports.Named(e.Pkg, e.Name, false), nil
	case emit.ExprParen:
		inner, err := s.renderExpr(e.Receiver)
		if err != nil {
			return "", err
		}
		return "(" + inner + ")", nil
	default:
		return "", fmt.Errorf("%w: %v", ErrUnsupportedExpr, e.ExprKind)
	}
}

// literalText spells a literal.
//
// A string literal is re-quoted rather than passed through: the
// generator wrote it in whatever quoting its own language uses, and
// the file this renders into has one quote style. Every other literal
// kind carries text that is already the same in both languages.
func literalText(e *emit.Expr) (string, error) {
	switch e.LitKind {
	case emit.LitString, emit.LitRune:
		return typescript.Quote(e.RawText), nil
	case emit.LitInt, emit.LitUint, emit.LitFloat, emit.LitBool, emit.LitRaw:
		return e.RawText, nil
	case emit.LitNil:
		// TypeScript has two absent values and they are not
		// interchangeable under strictNullChecks. `null` is the one an
		// absent value crossing JSON becomes.
		return typescript.TypeNull, nil
	default:
		return "", fmt.Errorf("%w: literal kind %v", ErrUnsupportedExpr, e.LitKind)
	}
}
