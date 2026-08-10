// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
)

// Slot-entry assertions for a contributor's tests.
//
// A generator that appends into another plugin's slot has one claim
// worth pinning on the emit graph: that what it queued is the shape it
// meant to queue. Answering it means reading a statement's kind back —
// and `sdk` deliberately does not re-export the expression and
// statement enums, on the stated grounds that "a generator uses the
// constructors and never reads them back".
//
// That is true of a generator and false of a test of one, which left
// the two contributor tests in this repository importing `emit`
// directly for a single constant apiece. These assertions close that:
// the discriminators stay out of the plugin-author façade, and the
// test-side need is met by the test package, which may name them.

// AssertRenderStmt fails unless n is a render statement wrapping a
// value of type T, which it returns.
//
// The shape a contributor produces when it declares an emit kind of
// its own and ships a template for it: the slot is constrained to
// statements, so the contribution is wrapped, and the backend renders
// it by dispatching on the wrapped value's kind.
//
// Returns the zero T on failure rather than nil-ing out, so a chained
// field read in the caller reports the original failure rather than a
// nil dereference on the next line.
func AssertRenderStmt[T any](tb testing.TB, n emit.Node) T {
	tb.Helper()
	var zero T
	stmt, ok := n.(*emit.Stmt)
	if !ok {
		tb.Errorf("slot entry is %T, want *emit.Stmt", n)
		return zero
	}
	if stmt.StmtKind != emit.StmtRender {
		tb.Errorf("slot entry is a %s statement, want a render statement; a contributor "+
			"declaring its own kind wraps the value so the backend can dispatch on it",
			stmt.StmtKind)
		return zero
	}
	value, ok := stmt.Node.(T)
	if !ok {
		tb.Errorf("render statement wraps %T, want %T", stmt.Node, zero)
		return zero
	}
	return value
}

// AssertExternalCall fails unless n is an expression statement wrapping
// a call to an external function, and returns the callee's import path,
// its symbol, and the raw text of each argument.
//
// The other shape a contributor produces: no kind of its own, no
// template, just the emit constructors for a call the backend already
// knows how to render. Asserting on the graph rather than on rendered
// text keeps a contributor's tests independent of any one backend's
// formatting.
//
// Arguments come back as [emit.Expr.RawText], which carries a string
// literal's unquoted content — the form a caller wants to compare
// against the option value that produced it.
func AssertExternalCall(tb testing.TB, n emit.Node) (pkg, fn string, args []string) {
	tb.Helper()
	stmt, ok := n.(*emit.Stmt)
	if !ok {
		tb.Errorf("slot entry is %T, want *emit.Stmt", n)
		return "", "", nil
	}
	if stmt.StmtKind != emit.StmtExpr {
		tb.Errorf("slot entry is a %s statement, want an expression statement", stmt.StmtKind)
		return "", "", nil
	}
	call := stmt.Call
	if call == nil || call.ExprKind != emit.ExprCall {
		tb.Errorf("expression statement does not wrap a call: %+v", call)
		return "", "", nil
	}
	if call.Callee == nil {
		tb.Errorf("call carries no callee")
		return "", "", nil
	}
	// NewExternal carries the import path on Pkg and the symbol on
	// Name; the backend resolves the alias at render time.
	pkg, fn = call.Callee.Pkg, call.Callee.Name
	for i, a := range call.Args {
		if a == nil {
			tb.Errorf("call argument %d is nil", i)
			return pkg, fn, args
		}
		args = append(args, a.RawText)
	}
	return pkg, fn, args
}
