// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"strconv"
	"strings"
	"text/template"
)

// The assertion dialect: how a generated Go test states that
// something must hold.
//
// Every generator emitting checks needs the same handful of
// statements, and each one written in a template is written slightly
// differently — a `t.Errorf` here, a `t.Fatalf` there, a message that
// names the value in one and not in the next. The failure text is the
// whole product of a generated check, so a set of them spelled six
// ways reads as six generators rather than one.
//
// Registered once by the Go backend, like every other bundle in
// [AllFuncMap]. A plugin calls the entries and declares nothing; a
// plugin that wants a different dialect — a helper package, a
// third-party assertion library — replaces the entries through
// [sdk.LanguageSupport.Overrides], which is the whole of what
// swapping dialects costs.
//
// # Why these are statements and not calls
//
// The generated file depends on nothing but `testing`. An assertion
// helper package would read better at the call site and would make
// every consumer of every generator take a dependency they did not
// choose, on a module the generator's author picked — for test code
// they cannot edit. The stdlib spelling is longer and belongs to the
// person who has to read the failure.
//
// # Why the caller names t
//
// Each entry takes the test handle as its first argument rather than
// assuming `t`. A subtest closure shadows the outer handle, a table
// case may bind its own, and a helper written against a fixed name
// silently reports the failure against the parent — which passes,
// because the parent has not failed.

// The assertion vocabulary's entry names, so a template and an
// override agree on one spelling. A plugin replacing the dialect
// overrides these names; one that misspells a replacement registers a
// second helper nothing calls, and its own templates go on rendering
// the default.
const (
	FuncAssertEqual    = "assertEqual"
	FuncAssertNotEqual = "assertNotEqual"
	FuncAssertTrue     = "assertTrue"
	FuncAssertFalse    = "assertFalse"
	FuncAssertNil      = "assertNil"
	FuncAssertNotNil   = "assertNotNil"
	FuncAssertLen      = "assertLen"
	FuncAssertNoError  = "assertNoError"
	FuncAssertError    = "assertError"
)

// AssertFuncMap returns the assertion dialect.
//
//	{{ assertEqual "t" "got" "want" "String round-trips" }}
//	{{ assertTrue "t" (printf "%s(err, %s)" $is $sentinel) "refuses the unknown" }}
//
// Each entry renders one complete statement. A condition needing a
// qualified symbol is composed by the template through `external`, so
// the import registers on the rendered file — which is why nothing
// here takes a package path: a helper returning text cannot ask for
// an import, and one that spelled `errors.Is` itself would emit a
// file that does not import errors.
func AssertFuncMap() template.FuncMap {
	return template.FuncMap{
		FuncAssertEqual:    AssertEqual,
		FuncAssertNotEqual: AssertNotEqual,
		FuncAssertTrue:     AssertTrue,
		FuncAssertFalse:    AssertFalse,
		FuncAssertNil:      AssertNil,
		FuncAssertNotNil:   AssertNotNil,
		FuncAssertLen:      AssertLen,
		FuncAssertNoError:  AssertNoError,
		FuncAssertError:    AssertError,
	}
}

// AssertEqual renders the check that got equals want.
//
// Both values are printed on failure, because which of the two is
// wrong is the first thing a reader needs and neither is derivable
// from the other. `%v` rather than `%q`: the pair may be any type the
// caller compared, and a verb chosen for strings prints a struct as a
// quoted rendering of its fields.
func AssertEqual(t, got, want, msg string) string {
	return ifThen(cmp(got, "!=", want), reportf(t, methodErrorf, msg, "got %v, want %v", got, want))
}

// AssertNotEqual renders the check that got differs from want.
//
// Only the value is printed. The two are equal when this fails, so
// printing both says the same thing twice.
func AssertNotEqual(t, got, want, msg string) string {
	return ifThen(cmp(got, "==", want), reportf(t, methodErrorf, msg, "both are %v", got))
}

// AssertTrue renders the check that a condition holds.
//
// The condition is parenthesised before it is negated. `!a == b` is
// legal Go that negates the left operand alone, so a caller passing a
// comparison would get a check asserting something it does not say.
func AssertTrue(t, cond, msg string) string {
	return ifThen("!("+cond+")", report(t, msg))
}

// AssertFalse renders the check that a condition does not hold.
func AssertFalse(t, cond, msg string) string {
	return ifThen("("+cond+")", report(t, msg))
}

// AssertNil renders the check that a value is nil.
func AssertNil(t, expr, msg string) string {
	return ifThen(cmp(expr, "!=", "nil"), reportf(t, methodErrorf, msg, "got %v, want nil", expr))
}

// AssertNotNil renders the check that a value is not nil.
func AssertNotNil(t, expr, msg string) string {
	return ifThen(cmp(expr, "==", "nil"), report(t, msg))
}

// AssertLen renders the check that a collection holds n entries.
//
// The actual length is printed rather than the collection: a slice
// that is one entry short prints nearly the same as the one that is
// not, and the number is what the reader is comparing.
func AssertLen(t, expr string, n int, msg string) string {
	want := strconv.Itoa(n)
	length := "len(" + expr + ")"
	return ifThen(cmp(length, "!=", want),
		reportf(t, methodErrorf, msg, "got %d, want "+want, length))
}

// AssertNoError renders the check that a call succeeded.
//
// Fatal rather than an error: everything after an unexpected failure
// reads a value the call did not produce, so continuing reports a
// cascade of failures with one cause.
func AssertNoError(t, expr, msg string) string {
	return ifThen(cmp(expr, "!=", "nil"), reportf(t, methodFatalf, msg, "%v", expr))
}

// AssertError renders the check that a call failed.
//
// An error is expected here, so nothing downstream depends on it and
// the check reports rather than aborts.
func AssertError(t, expr, msg string) string {
	return ifThen(cmp(expr, "==", "nil"), report(t, msg))
}

// ifThen renders a one-branch if statement.
//
// Unindented and on its own lines. The backend formats every rendered
// file before it is written, so indentation composed here would be
// discarded — and a template that had to be indented correctly for
// the output to read well is a template nobody can edit.
func ifThen(cond, body string) string {
	return "if " + cond + " {\n" + body + "\n}"
}

// cmp joins two operands with an operator.
func cmp(left, op, right string) string {
	return left + " " + op + " " + right
}

// The two ways a generated check reports. Named rather than spelled
// at each site so the reporting-versus-aborting choice reads as a
// decision each assertion made.
const (
	methodErrorf = "Errorf"
	methodFatalf = "Fatalf"
)

// report renders a failure carrying only its message, for a check
// whose condition already says everything a reader needs: an
// assertion that a value is not nil has nothing to print but the nil
// it found.
//
// Always reporting rather than aborting, and the method is fixed
// rather than a parameter. A check with nothing to print is one whose
// answer nothing downstream reads, so there is no cascade to cut
// short — [reportf] is where the choice is live.
//
// The message is quoted through [Quote] rather than wrapped in
// literal quotes: one carrying a quote, a backslash or a newline
// would otherwise end the literal early, and the failure lands in the
// consumer's build of a file they did not write.
func report(t, msg string) string {
	return t + "." + methodErrorf + "(" + Quote(msg) + ")"
}

// reportf renders a failure that prints values alongside its message.
//
// The message leads and the values follow, so a reader scanning
// output sees what was being asserted before what it got. The format
// is appended to the message inside one literal rather than
// concatenated at runtime, which keeps the whole of what is printed
// visible in the generated source.
func reportf(t, method, msg, format string, args ...string) string {
	return t + "." + method + "(" + Quote(msg+": "+format) + ", " +
		strings.Join(args, ", ") + ")"
}
