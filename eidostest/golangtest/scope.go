// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golangtest

import (
	"go/ast"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Scope is one declaration's body, for the assertions no structural
// query expresses.
//
// The escape hatch, deliberately narrowed: a substring matched
// against a whole file cannot say *which* declaration carries it, so
// a check meaning "the method that cannot fail has no fault arm" ends
// up counting occurrences across the file and inferring the answer.
// Scoped to a declaration it asks the question directly, and the
// answer stops moving when an unrelated method is added.
//
// The body is compared in canonical form, so gofmt's column
// alignment inside a composite literal is not part of the assertion.
type Scope struct {
	src   *Source
	label string
	body  string
	found bool
}

// InFunc narrows to a plain function's body.
func (s *Source) InFunc(tb testing.TB, name string) *Scope {
	tb.Helper()
	fn := s.funcDecl("", name)
	if fn == nil {
		tb.Errorf("golangtest: %s declares no function %q; it declares %v",
			s.path, name, s.funcNames())
		return &Scope{src: s, label: name}
	}
	return &Scope{src: s, label: name, body: bodyText(fn), found: true}
}

// InMethod narrows to a method's body.
func (s *Source) InMethod(tb testing.TB, recv, name string) *Scope {
	tb.Helper()
	fn := s.funcDecl(recv, name)
	if fn == nil {
		tb.Errorf("golangtest: %s declares no method %s.%s; %s declares %v",
			s.path, recv, name, recv, s.methodNames(recv))
		return &Scope{src: s, label: recv + "." + name}
	}
	return &Scope{src: s, label: recv + "." + name, body: bodyText(fn), found: true}
}

// Body returns the scope's rendered body, for a caller composing its
// own assertion.
func (sc *Scope) Body() string { return sc.body }

// AssertContains fails when substr does not appear in the body.
func (sc *Scope) AssertContains(tb testing.TB, substr string) *Scope {
	tb.Helper()
	if !sc.found {
		return sc
	}
	if !strings.Contains(sc.body, substr) {
		tb.Errorf("golangtest: %s body of %s does not contain %q\n--- body ---\n%s",
			sc.src.path, sc.label, substr, sc.body)
	}
	return sc
}

// AssertNotContains fails when substr appears in the body.
func (sc *Scope) AssertNotContains(tb testing.TB, substr string) *Scope {
	tb.Helper()
	if !sc.found {
		return sc
	}
	if strings.Contains(sc.body, substr) {
		tb.Errorf("golangtest: %s body of %s contains %q, which it must not\n--- body ---\n%s",
			sc.src.path, sc.label, substr, sc.body)
	}
	return sc
}

// AssertCount fails when substr does not appear exactly n times in
// the body.
func (sc *Scope) AssertCount(tb testing.TB, substr string, n int) *Scope {
	tb.Helper()
	if !sc.found {
		return sc
	}
	if got := strings.Count(sc.body, substr); got != n {
		tb.Errorf("golangtest: %s body of %s contains %q %d time(s), want %d\n--- body ---\n%s",
			sc.src.path, sc.label, substr, got, n, sc.body)
	}
	return sc
}

// bodyText renders a function's body with canonical spacing.
func bodyText(fn *ast.FuncDecl) string {
	if fn.Body == nil {
		return ""
	}
	return render(fn.Body)
}

// Subtests lists the names a test function passes to t.Run, in source
// order, including those nested inside another subtest.
//
// Generated test suites are a generator's real output contract, and
// their subtest names are the only part of one a reader ever sees in
// a failure. Reading them structurally beats matching `t.Run("…"` as
// a substring, which cannot tell a subtest from a string that happens
// to spell one.
func (s *Source) Subtests(tb testing.TB, funcName string) []string {
	tb.Helper()
	fn := s.funcDecl("", funcName)
	if fn == nil {
		tb.Errorf("golangtest: %s declares no function %q; it declares %v",
			s.path, funcName, s.funcNames())
		return nil
	}
	var out []string
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel || sel.Sel.Name != "Run" {
			return true
		}
		lit, isLit := call.Args[0].(*ast.BasicLit)
		if !isLit {
			return true
		}
		name, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		out = append(out, name)
		return true
	})
	return out
}

// AssertSubtest fails when the test function declares no subtest of
// that name.
func (s *Source) AssertSubtest(tb testing.TB, funcName, subtest string) *Source {
	tb.Helper()
	got := s.Subtests(tb, funcName)
	if !slices.Contains(got, subtest) {
		tb.Errorf("golangtest: %s %s declares no subtest %q; it declares %v",
			s.path, funcName, subtest, got)
	}
	return s
}

// AssertNoSubtest fails when the test function declares a subtest of
// that name.
//
// The half that matters most for a generator that withholds a check:
// a check it cannot honestly make must be absent rather than present
// and vacuous, and only its absence distinguishes "correctly omitted"
// from "silently dropped".
func (s *Source) AssertNoSubtest(tb testing.TB, funcName, subtest string) *Source {
	tb.Helper()
	if slices.Contains(s.Subtests(tb, funcName), subtest) {
		tb.Errorf("golangtest: %s %s declares subtest %q, which it must not",
			s.path, funcName, subtest)
	}
	return s
}

// TestFuncs lists every `Test…(t *testing.T)` the file declares.
func (s *Source) TestFuncs() []string {
	var out []string
	for _, d := range s.file.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
			out = append(out, fn.Name.Name)
		}
	}
	return out
}

// AssertParallel fails when the named test function does not call
// t.Parallel.
//
// A generated suite runs in every consumer's CI, once per annotated
// declaration. One that forgets to go parallel taxes each of them for
// the life of the generator, and no consumer will ever think to look
// at generated code for the cause.
func (s *Source) AssertParallel(tb testing.TB, funcName string) *Source {
	tb.Helper()
	fn := s.funcDecl("", funcName)
	if fn == nil {
		tb.Errorf("golangtest: %s declares no function %q; it declares %v",
			s.path, funcName, s.funcNames())
		return s
	}
	if !callsParallel(fn.Body) {
		tb.Errorf("golangtest: %s %s does not call t.Parallel; every consumer's CI "+
			"pays for that once per generated suite", s.path, funcName)
	}
	return s
}

// callsParallel reports whether a block calls `<ident>.Parallel()`
// directly, without descending into a nested function literal — a
// subtest's own call says nothing about its parent.
func callsParallel(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}
	for _, stmt := range body.List {
		expr, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, isCall := expr.X.(*ast.CallExpr)
		if !isCall {
			continue
		}
		if sel, isSel := call.Fun.(*ast.SelectorExpr); isSel && sel.Sel.Name == "Parallel" {
			return true
		}
	}
	return false
}
