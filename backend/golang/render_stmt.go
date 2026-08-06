// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"errors"
	"fmt"
	"strings"

	"go.thesmos.sh/eidos/emit"
)

// ErrUnsupportedStmt is returned by [renderState.renderStmt] when it
// cannot produce a spelling for the statement it was handed. Two
// cases: an out-of-range [emit.StmtKind] discriminator (every
// documented variant is wired), and an [emit.StmtRender] whose
// [emit.Stmt.Node] is nil, which means the contributing plugin built
// the wrapper without anything to delegate to. The wrapped message
// names the offending case so diagnostics attribute the gap
// precisely.
var ErrUnsupportedStmt = errors.New("golang: unsupported Stmt")

// renderStmt produces the Go source spelling for an [emit.Stmt].
// Every documented [emit.StmtKind] variant is supported. Nil input
// returns the empty string so callers can place the helper
// directly into templates without explicit nil-guards on optional
// sub-statements (init clauses, post clauses, …).
//
// Output lines do not carry leading indentation — `go/format.Source`
// inserts indentation based on the enclosing block depth when the
// final file body is finalised.
//
// `renderStmt` is one of the reserved dispatch funcmap entries —
// plugin overrides are rejected at Build time.
func (s *renderState) renderStmt(st *emit.Stmt) (string, error) {
	if st == nil {
		return "", nil
	}
	switch st.StmtKind {
	case emit.StmtBlock:
		body, err := s.renderStmtBlock(st.Block)
		if err != nil {
			return "", err
		}
		return "{\n" + body + "}", nil
	case emit.StmtExpr:
		return s.renderExpr(st.Call)
	case emit.StmtAssign:
		return s.renderAssign(st)
	case emit.StmtReturn:
		return s.renderReturnStmt(st)
	case emit.StmtIf:
		return s.renderIf(st)
	case emit.StmtFor:
		return s.renderFor(st)
	case emit.StmtForRange:
		return s.renderForRange(st)
	case emit.StmtSwitch:
		return s.renderSwitch(st)
	case emit.StmtCase:
		return s.renderCase(st)
	case emit.StmtDefault:
		return s.renderDefault(st)
	case emit.StmtDefer:
		call, err := s.renderExpr(st.Call)
		if err != nil {
			return "", err
		}
		return "defer " + call, nil
	case emit.StmtGo:
		call, err := s.renderExpr(st.Call)
		if err != nil {
			return "", err
		}
		return "go " + call, nil
	case emit.StmtBreak:
		return labelledKeyword("break", st.Label), nil
	case emit.StmtContinue:
		return labelledKeyword("continue", st.Label), nil
	case emit.StmtLabel:
		inner, err := s.renderStmt(st.Inner)
		if err != nil {
			return "", err
		}
		return st.Label + ":\n" + inner, nil
	case emit.StmtVar:
		return s.renderLocalDecl("var", st)
	case emit.StmtConst:
		return s.renderLocalDecl("const", st)
	case emit.StmtRaw:
		return st.RawText, nil
	case emit.StmtRender:
		if st.Node == nil {
			return "", fmt.Errorf("%w: StmtRender carries no Node", ErrUnsupportedStmt)
		}
		return s.render(st.Node)
	default:
		return "", fmt.Errorf("%w: StmtKind=%s", ErrUnsupportedStmt, st.StmtKind)
	}
}

// renderStmtBlock renders a slice of statements as the body of a
// brace-wrapped block — one statement per line, terminated by `\n`.
// The caller wraps the result in `{ … }` as appropriate;
// go/format.Source handles indentation.
func (s *renderState) renderStmtBlock(stmts []*emit.Stmt) (string, error) {
	var b strings.Builder
	for _, st := range stmts {
		rendered, err := s.renderStmt(st)
		if err != nil {
			return "", err
		}
		b.WriteString(rendered)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// renderAssign produces the Go assignment-statement spelling.
// Handles both regular assignment (`=`, `+=`, `-=`, …) and short
// variable declarations (`:=`). Multi-target / multi-value forms
// `a, b = c, d` are supported.
func (s *renderState) renderAssign(st *emit.Stmt) (string, error) {
	lhs, err := s.renderExprList(st.Targets)
	if err != nil {
		return "", err
	}
	rhs, err := s.renderExprList(st.Values)
	if err != nil {
		return "", err
	}
	return lhs + " " + st.Op + " " + rhs, nil
}

// renderReturnStmt produces the Go `return` statement. Zero values
// renders as a bare `return`; one or more values join with `, `.
func (s *renderState) renderReturnStmt(st *emit.Stmt) (string, error) {
	if len(st.Returns) == 0 {
		return "return", nil
	}
	rhs, err := s.renderExprList(st.Returns)
	if err != nil {
		return "", err
	}
	return "return " + rhs, nil
}

// renderIf produces a Go `if [init;] cond { then } [else …]`
// statement. The else branch chains directly when it carries a
// single nested [emit.StmtIf] (the `else if …` idiom) and uses a
// brace-wrapped block otherwise.
func (s *renderState) renderIf(st *emit.Stmt) (string, error) {
	var b strings.Builder
	b.WriteString("if ")
	if st.Init != nil {
		init, err := s.renderStmt(st.Init)
		if err != nil {
			return "", err
		}
		b.WriteString(init)
		b.WriteString("; ")
	}
	cond, err := s.renderExpr(st.Cond)
	if err != nil {
		return "", err
	}
	b.WriteString(cond)
	b.WriteString(" {\n")
	thenBody, err := s.renderStmtBlock(st.Block)
	if err != nil {
		return "", err
	}
	b.WriteString(thenBody)
	b.WriteByte('}')
	if len(st.Else) == 0 {
		return b.String(), nil
	}
	// `else if …` chains a nested If without an extra block layer.
	if len(st.Else) == 1 && st.Else[0].StmtKind == emit.StmtIf {
		nested, nestedErr := s.renderStmt(st.Else[0])
		if nestedErr != nil {
			return "", nestedErr
		}
		b.WriteString(" else ")
		b.WriteString(nested)
		return b.String(), nil
	}
	elseBody, err := s.renderStmtBlock(st.Else)
	if err != nil {
		return "", err
	}
	b.WriteString(" else {\n")
	b.WriteString(elseBody)
	b.WriteByte('}')
	return b.String(), nil
}

// renderFor produces a Go `for` loop. Renders four shapes
// depending on which clauses are populated:
//
//   - All nil → `for { … }`           (infinite loop)
//   - Cond only → `for cond { … }`   (while-style)
//   - Init/Cond/Post → `for init; cond; post { … }` (C-style)
//
// Any combination of nil clauses produces the corresponding
// reduced form.
func (s *renderState) renderFor(st *emit.Stmt) (string, error) {
	body, err := s.renderStmtBlock(st.Block)
	if err != nil {
		return "", err
	}
	header, err := s.renderForHeader(st)
	if err != nil {
		return "", err
	}
	return header + " {\n" + body + "}", nil
}

// renderForHeader produces the `for …` header (without the body
// braces) for renderFor — `for`, `for cond`, or `for init; cond;
// post`.
func (s *renderState) renderForHeader(st *emit.Stmt) (string, error) {
	hasInit := st.Init != nil
	hasPost := st.Post != nil
	hasCond := st.Cond != nil
	if !hasInit && !hasPost && !hasCond {
		return "for", nil
	}
	if !hasInit && !hasPost {
		cond, err := s.renderExpr(st.Cond)
		if err != nil {
			return "", err
		}
		return "for " + cond, nil
	}
	init, err := s.renderStmt(st.Init)
	if err != nil {
		return "", err
	}
	cond, err := s.renderExpr(st.Cond)
	if err != nil {
		return "", err
	}
	post, err := s.renderStmt(st.Post)
	if err != nil {
		return "", err
	}
	return "for " + init + "; " + cond + "; " + post, nil
}

// renderForRange produces a Go range-loop. Empty key / value names
// render as the underscore-omission idiom: `for range x`,
// `for k := range x`, `for _, v := range x`, `for k, v := range x`.
func (s *renderState) renderForRange(st *emit.Stmt) (string, error) {
	body, err := s.renderStmtBlock(st.Block)
	if err != nil {
		return "", err
	}
	over, err := s.renderExpr(st.RangeOver)
	if err != nil {
		return "", err
	}
	header := rangeHeader(st.RangeKey, st.RangeValue, over)
	return header + " {\n" + body + "}", nil
}

// rangeHeader assembles the `for … range x` clause based on
// which range variables are named. Both empty produces the bare
// `for range x` form Go 1.22+ allows.
func rangeHeader(key, value, over string) string {
	switch {
	case key == "" && value == "":
		return "for range " + over
	case value == "":
		return "for " + key + " := range " + over
	case key == "":
		return "for _, " + value + " := range " + over
	default:
		return "for " + key + ", " + value + " := range " + over
	}
}

// renderSwitch produces a Go `switch [init;] [cond] { … }` block.
// Cases render through [renderState.renderStmt] dispatch on
// StmtCase / StmtDefault.
func (s *renderState) renderSwitch(st *emit.Stmt) (string, error) {
	var b strings.Builder
	b.WriteString("switch ")
	if st.Init != nil {
		init, err := s.renderStmt(st.Init)
		if err != nil {
			return "", err
		}
		b.WriteString(init)
		b.WriteString("; ")
	}
	if st.Cond != nil {
		cond, err := s.renderExpr(st.Cond)
		if err != nil {
			return "", err
		}
		b.WriteString(cond)
		b.WriteByte(' ')
	}
	b.WriteString("{\n")
	for _, c := range st.Cases {
		rendered, err := s.renderStmt(c)
		if err != nil {
			return "", err
		}
		b.WriteString(rendered)
		b.WriteByte('\n')
	}
	b.WriteByte('}')
	return b.String(), nil
}

// renderCase produces one `case v1, v2: body` clause inside a
// switch.
func (s *renderState) renderCase(st *emit.Stmt) (string, error) {
	values, err := s.renderExprList(st.Values)
	if err != nil {
		return "", err
	}
	body, err := s.renderStmtBlock(st.Block)
	if err != nil {
		return "", err
	}
	return "case " + values + ":\n" + body, nil
}

// renderDefault produces the `default: body` clause inside a
// switch.
func (s *renderState) renderDefault(st *emit.Stmt) (string, error) {
	body, err := s.renderStmtBlock(st.Block)
	if err != nil {
		return "", err
	}
	return "default:\n" + body, nil
}

// renderLocalDecl produces the body of a `var` or `const`
// statement inside a function body. The keyword argument selects
// between the two forms — the rendered shape is otherwise
// identical: `<kw> Name [Type] [= Init]`.
func (s *renderState) renderLocalDecl(kw string, st *emit.Stmt) (string, error) {
	var b strings.Builder
	b.WriteString(kw)
	b.WriteByte(' ')
	b.WriteString(st.DeclName)
	if st.DeclType != nil {
		t, err := s.renderType(st.DeclType)
		if err != nil {
			return "", err
		}
		b.WriteByte(' ')
		b.WriteString(t)
	}
	if st.Call != nil {
		init, err := s.renderExpr(st.Call)
		if err != nil {
			return "", err
		}
		b.WriteString(" = ")
		b.WriteString(init)
	}
	return b.String(), nil
}

// labelledKeyword returns `break`, `continue`, etc. with an
// optional label appended (`break loop` rather than `break`). An
// empty label renders the bare keyword.
func labelledKeyword(kw, label string) string {
	if label == "" {
		return kw
	}
	return kw + " " + label
}
