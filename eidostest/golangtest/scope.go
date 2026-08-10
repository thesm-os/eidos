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

// InVar narrows to a package-level var's initialiser.
//
// The scope a slot host most often renders into. A weaver's
// contributions land inside `var Chain = []Middleware{…}` as readily
// as inside a method body, and without this every claim about what
// the composite literal holds — membership, count, order — falls back
// to a substring matched against the whole file, which cannot tell
// this var's contents from a neighbouring one's.
//
// A var with no initialiser is reported rather than yielding an empty
// scope: every assertion over one would pass or fail on nothing.
func (s *Source) InVar(tb testing.TB, name string) *Scope {
	tb.Helper()
	label := "var " + name
	spec, idx := s.valueSpec(name)
	if spec == nil {
		tb.Errorf("golangtest: %s declares no var %q; it declares %v",
			s.path, name, s.varNames())
		return &Scope{src: s, label: label}
	}
	value := initialiser(spec, idx)
	if value == nil {
		tb.Errorf("golangtest: %s var %q has no initialiser, so there is nothing to narrow to",
			s.path, name)
		return &Scope{src: s, label: label}
	}
	return &Scope{src: s, label: label, body: render(value), found: true}
}

// initialiser returns the expression assigned to the name at index
// idx of a value spec.
//
// A spec with one value and several names is a multiple assignment
// from a single call — `var a, b = f()` — where every name shares the
// one expression.
func initialiser(spec *ast.ValueSpec, idx int) ast.Expr {
	switch {
	case len(spec.Values) == 0:
		return nil
	case len(spec.Values) == 1:
		return spec.Values[0]
	case idx < len(spec.Values):
		return spec.Values[idx]
	default:
		return nil
	}
}

// Body returns the scope's rendered body, for a caller composing its
// own assertion.
func (sc *Scope) Body() string { return sc.body }

// AssertOrder fails unless first appears before second within the
// scope.
//
// The claim every cross-cutting weaver is actually making, and the
// one nothing else in this package could express:
// [Source.AssertOrder] ranks top-level declarations, while a slot
// contribution lands one level down, inside a body or a composite
// literal. A prebody contribution rendered after the return statement
// compiles, vets, and satisfies every substring assertion about it —
// it is simply dead.
//
// Compared on first occurrence, so a marker the generator repeats is
// ranked by where it starts rather than where it ends.
func (sc *Scope) AssertOrder(tb testing.TB, first, second string) *Scope {
	tb.Helper()
	return sc.AssertOrderAll(tb, first, second)
}

// AssertOrderAll fails unless every substring appears in the scope,
// in the order given.
//
// The list form of [Scope.AssertOrder]: a slot host with three
// contributors is one claim about one order, not two claims plus an
// unwritten transitivity argument.
func (sc *Scope) AssertOrderAll(tb testing.TB, parts ...string) *Scope {
	tb.Helper()
	if !sc.found {
		return sc
	}
	prev, prevAt := "", -1
	for _, part := range parts {
		at := strings.Index(sc.body, part)
		if at < 0 {
			tb.Errorf("golangtest: %s body of %s does not contain %q\n--- body ---\n%s",
				sc.src.path, sc.label, part, sc.body)
			return sc
		}
		if at < prevAt {
			tb.Errorf("golangtest: %s body of %s renders %q before %q, which must come first"+
				"\n--- body ---\n%s", sc.src.path, sc.label, part, prev, sc.body)
			return sc
		}
		prev, prevAt = part, at
	}
	return sc
}

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

// AssertTestFuncs fails when the file's set of test entry points is
// anything other than exactly the named ones.
//
// A generator whose output is a suite is making a claim about
// coverage — one recording check per interface method, one boundary
// case per validated field — and "the tests I expected are present"
// only tells half of it. The other half is that nothing else is: a
// projection that emitted a check for a method it cannot honestly
// exercise, or emitted the same one twice under different names,
// passes every membership assertion ever written about it.
func (s *Source) AssertTestFuncs(tb testing.TB, names ...string) *Source {
	tb.Helper()
	got, want, extra, missing := diffSets(s.TestFuncs(), names)
	if extra == nil && missing == nil {
		return s
	}
	tb.Errorf("golangtest: %s declares tests %v, want exactly %v (unexpected: %v; absent: %v)",
		s.path, got, want, extra, missing)
	return s
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
