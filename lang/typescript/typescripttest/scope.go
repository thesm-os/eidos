// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	ts "github.com/tree-sitter/go-tree-sitter"
)

// Scope is one declaration's body, for the assertions no structural
// query expresses.
//
// The escape hatch, deliberately narrowed: a substring matched
// against a whole file cannot say *which* declaration carries it, so
// a check meaning "the method that cannot fail has no throw" ends up
// counting occurrences across the file and inferring the answer.
// Scoped to a declaration it asks the question directly, and the
// answer stops moving when an unrelated member is added.
//
// The body is compared with runs of whitespace collapsed, so the
// normaliser's indentation is not part of the assertion.
type Scope struct {
	src   *Source
	label string
	body  string
	found bool
}

// InFunction narrows to a module-level function's body.
func (s *Source) InFunction(tb testing.TB, name string) *Scope {
	tb.Helper()
	n := s.declNode(name)
	if n == nil || n.ChildByFieldName("body") == nil {
		tb.Errorf("typescripttest: %s declares no function %q with a body; it declares %v",
			s.path, name, s.DeclNames())
		return &Scope{src: s, label: name}
	}
	return &Scope{src: s, label: name, body: s.text(n.ChildByFieldName("body")), found: true}
}

// InMethod narrows to a method's body.
//
// A method on an interface has none — it is a signature — so this
// reports rather than returning an empty scope that every assertion
// would then pass against.
func (s *Source) InMethod(tb testing.TB, decl, name string) *Scope {
	tb.Helper()
	label := decl + "." + name
	body := s.bodyOf(tb, decl)
	if body == nil {
		return &Scope{src: s, label: label}
	}
	for i := range body.NamedChildCount() {
		member := body.NamedChild(i)
		got, ok := methodOf(member, s.src)
		if !ok || got != name {
			continue
		}
		inner := member.ChildByFieldName("body")
		if inner == nil {
			tb.Errorf("typescripttest: %s declares %s as a signature with no body, so there "+
				"is nothing to scope to", s.path, label)
			return &Scope{src: s, label: label}
		}
		return &Scope{src: s, label: label, body: s.text(inner), found: true}
	}
	tb.Errorf("typescripttest: %s declares no %s", s.path, label)
	return &Scope{src: s, label: label}
}

// InBinding narrows to a module-level binding's initialiser.
//
// The scope a slot host most often renders into. A weaver's
// contributions land inside `export const chain = [ … ]` as readily
// as inside a function, and a substring matched against the file
// cannot tell one array from another.
func (s *Source) InBinding(tb testing.TB, name string) *Scope {
	tb.Helper()
	n := s.declNode(name)
	if n == nil {
		tb.Errorf("typescripttest: %s declares no binding %q; it declares %v",
			s.path, name, s.DeclNames())
		return &Scope{src: s, label: name}
	}
	value := initialiserOf(n)
	if value == nil {
		tb.Errorf("typescripttest: %s declares %s with no initialiser, so there is nothing "+
			"to scope to", s.path, name)
		return &Scope{src: s, label: name}
	}
	return &Scope{src: s, label: name, body: s.text(value), found: true}
}

// initialiserOf returns the value a binding declaration assigns.
func initialiserOf(n *ts.Node) *ts.Node {
	d := bindingDeclarator(n)
	if d == nil {
		return nil
	}
	return d.ChildByFieldName("value")
}

// Body returns the scope's text, empty when the declaration was not
// found.
func (sc *Scope) Body() string { return sc.body }

// AssertContains fails when substr does not appear in the scope.
func (sc *Scope) AssertContains(tb testing.TB, substr string) *Scope {
	tb.Helper()
	if !sc.found {
		return sc
	}
	if !strings.Contains(collapse(sc.body), collapse(substr)) {
		tb.Errorf("typescripttest: %s in %s does not contain %q:\n%s",
			sc.label, sc.src.path, substr, sc.body)
	}
	return sc
}

// AssertNotContains fails when substr appears in the scope.
func (sc *Scope) AssertNotContains(tb testing.TB, substr string) *Scope {
	tb.Helper()
	if !sc.found {
		return sc
	}
	if strings.Contains(collapse(sc.body), collapse(substr)) {
		tb.Errorf("typescripttest: %s in %s contains %q, which it must not:\n%s",
			sc.label, sc.src.path, substr, sc.body)
	}
	return sc
}

// AssertCount fails when substr appears in the scope other than n
// times.
//
// The assertion a duplicated contribution fails. A slot appended to
// twice renders the same line twice, which every "contains" check
// passes and no reader of the diff notices until the generated code
// registers a handler under one name and the second one wins.
func (sc *Scope) AssertCount(tb testing.TB, substr string, n int) *Scope {
	tb.Helper()
	if !sc.found {
		return sc
	}
	if got := strings.Count(collapse(sc.body), collapse(substr)); got != n {
		tb.Errorf("typescripttest: %s in %s contains %q %d times, want %d:\n%s",
			sc.label, sc.src.path, substr, got, n, sc.body)
	}
	return sc
}

// AssertOrder fails when first does not appear before second within
// the scope.
func (sc *Scope) AssertOrder(tb testing.TB, first, second string) *Scope {
	tb.Helper()
	if !sc.found {
		return sc
	}
	body := collapse(sc.body)
	a, b := strings.Index(body, collapse(first)), strings.Index(body, collapse(second))
	switch {
	case a < 0:
		tb.Errorf("typescripttest: %s in %s does not contain %q:\n%s",
			sc.label, sc.src.path, first, sc.body)
	case b < 0:
		tb.Errorf("typescripttest: %s in %s does not contain %q:\n%s",
			sc.label, sc.src.path, second, sc.body)
	case a > b:
		tb.Errorf("typescripttest: %s in %s has %q after %q:\n%s",
			sc.label, sc.src.path, first, second, sc.body)
	}
	return sc
}

// AssertOrderAll fails when the parts do not appear in the order
// given. Anything not named is ignored.
func (sc *Scope) AssertOrderAll(tb testing.TB, parts ...string) *Scope {
	tb.Helper()
	for i := 1; i < len(parts); i++ {
		sc.AssertOrder(tb, parts[i-1], parts[i])
	}
	return sc
}

// Suites returns the names every top-level `describe` in the file
// declares, in source order.
//
// The outer half of a generated suite. A generator emitting one
// `describe` per contract has that list as its shape, and asserting
// on it catches a suite that stopped covering something without
// anyone reading the whole file.
func (s *Source) Suites() []string { return s.callNames(s.root(), suiteCallees, false) }

// Cases returns the names the cases inside one suite declare, in
// source order.
//
// Scoped to the suite rather than the file, because a generator
// emitting the same case name under two contracts is emitting exactly
// what a consumer expects and a file-wide list would report as a
// duplicate.
func (s *Source) Cases(tb testing.TB, suite string) []string {
	tb.Helper()
	n := s.suiteNode(suite)
	if n == nil {
		tb.Errorf("typescripttest: %s declares no suite %q; it declares %v",
			s.path, suite, s.Suites())
		return nil
	}
	return s.callNames(n, caseCallees, true)
}

// AssertSuites fails when the file's suites are not exactly those
// given, in order.
func (s *Source) AssertSuites(tb testing.TB, names ...string) *Source {
	tb.Helper()
	if got := s.Suites(); !slices.Equal(got, names) {
		tb.Errorf("typescripttest: %s declares suites %v, want %v", s.path, got, names)
	}
	return s
}

// AssertCase fails when the named suite declares no case of that
// name.
//
// Names the cases that are there, because the failure is nearly
// always a renamed case rather than a missing one, and a bare "not
// found" sends the reader to the generator that emitted it.
func (s *Source) AssertCase(tb testing.TB, suite, name string) *Source {
	tb.Helper()
	got := s.Cases(tb, suite)
	if !slices.Contains(got, name) {
		tb.Errorf("typescripttest: %s declares no case %q under %q; it declares %v",
			s.path, name, suite, got)
	}
	return s
}

// AssertNoCase fails when the named suite declares a case of that
// name.
//
// The direction a generator emitting conditional coverage needs: a
// case asserting something the declaration does not say is worse than
// no case at all, because it passes.
func (s *Source) AssertNoCase(tb testing.TB, suite, name string) *Source {
	tb.Helper()
	if slices.Contains(s.Cases(tb, suite), name) {
		tb.Errorf("typescripttest: %s declares case %q under %q, which it must not",
			s.path, name, suite)
	}
	return s
}

// AssertConcurrent fails when the named suite is not declared
// concurrent.
//
// `describe.concurrent` is what a runner needs to overlap the cases
// inside one suite, and a generated suite that dropped it slows every
// consumer's build without failing anything. Node's own runner spells
// the same idea `{ concurrency: true }`, so both are accepted.
func (s *Source) AssertConcurrent(tb testing.TB, suite string) *Source {
	tb.Helper()
	n := s.suiteNode(suite)
	if n == nil {
		tb.Errorf("typescripttest: %s declares no suite %q; it declares %v",
			s.path, suite, s.Suites())
		return s
	}
	text := collapse(s.text(n))
	if !strings.Contains(text, ".concurrent") && !strings.Contains(text, "concurrency: true") {
		tb.Errorf("typescripttest: %s declares suite %q without concurrency, so a consumer "+
			"runs its cases in series", s.path, suite)
	}
	return s
}

// The callees a suite and a case are declared through.
//
// Both vocabularies, because the ecosystem has two and a generator
// picks whichever its consumers' runner is configured for: `describe`
// and `it` are Jasmine's, inherited by Jest, Mocha and Vitest;
// `suite` and `test` are Node's. A harness recognising one would
// report every suite the other emits as absent.
var (
	//nolint:gochecknoglobals // immutable lookup tables
	suiteCallees = []string{"describe", "suite"}
	//nolint:gochecknoglobals // immutable lookup tables
	caseCallees = []string{"it", "test"}
)

// suiteNode returns the call declaring the named suite, or nil.
func (s *Source) suiteNode(name string) *ts.Node {
	var found *ts.Node
	walkCalls(s.root(), func(call *ts.Node) bool {
		if !callsOneOf(s, call, suiteCallees) || s.callLabel(call) != name {
			return true
		}
		found = call
		return false
	})
	return found
}

// callNames collects the label every matching call declares beneath
// n.
//
// `nested` decides whether the walk descends into a matching call: a
// file's suites are the outer ones and a suite's cases are the inner
// ones, and one walk answers both by stopping or not at each match.
func (s *Source) callNames(n *ts.Node, callees []string, nested bool) []string {
	var out []string
	walkCalls(n, func(call *ts.Node) bool {
		if !callsOneOf(s, call, callees) {
			return true
		}
		if label := s.callLabel(call); label != "" {
			out = append(out, label)
		}
		return nested
	})
	return out
}

// walkCalls visits every call expression beneath n, descending into
// one only when visit returns true.
func walkCalls(n *ts.Node, visit func(*ts.Node) bool) {
	if n == nil {
		return
	}
	if n.Kind() == "call_expression" && !visit(n) {
		return
	}
	for i := range n.NamedChildCount() {
		walkCalls(n.NamedChild(i), visit)
	}
}

// callsOneOf reports whether a call names one of the given functions,
// with or without a modifier — `describe`, `describe.concurrent`,
// `it.skip`.
func callsOneOf(s *Source, call *ts.Node, callees []string) bool {
	fn := call.ChildByFieldName("function")
	if fn == nil {
		return false
	}
	name, _, _ := strings.Cut(s.text(fn), ".")
	return slices.Contains(callees, strings.TrimSpace(name))
}

// callLabel returns the string a suite or case call was named with,
// empty for one whose first argument is not a literal.
func (s *Source) callLabel(call *ts.Node) string {
	args := call.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() == 0 {
		return ""
	}
	first := args.NamedChild(0)
	if first.Kind() != "string" {
		return ""
	}
	text := s.text(first)
	if unquoted, err := strconv.Unquote(text); err == nil {
		return unquoted
	}
	return strings.Trim(text, "'`\"")
}
