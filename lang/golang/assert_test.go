// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
)

// parses reports whether body is a legal Go statement list.
//
// Every entry here renders source a consumer compiles, and a check
// that only compared strings would pass on output that does not
// parse — which is the one failure mode that lands in someone else's
// build. Wrapped in a function because a bare statement list is not
// a Go file.
func parses(t *testing.T, body string) {
	t.Helper()
	src := "package p\nfunc f(t T) {\n" + body + "\n}\n"
	if _, err := parser.ParseFile(token.NewFileSet(), "x.go", src, 0); err != nil {
		t.Fatalf("rendered statement does not parse: %v\n%s", err, src)
	}
}

// The comparison assertions.
func TestAssertComparisons(t *testing.T) {
	t.Parallel()

	t.Run("equal negates the comparison it asserts", func(t *testing.T) {
		t.Parallel()
		got := golang.AssertEqual("t", "got", "want", "round-trips")
		if !strings.Contains(got, "if got != want {") {
			t.Errorf("equality must fail on inequality, got:\n%s", got)
		}
		parses(t, got)
	})

	t.Run("equal prints both sides", func(t *testing.T) {
		t.Parallel()
		got := golang.AssertEqual("t", "got", "want", "round-trips")
		if !strings.Contains(got, "got %v, want %v") {
			t.Errorf("which of the two is wrong is not derivable from one, got:\n%s", got)
		}
	})

	t.Run("not-equal prints one side", func(t *testing.T) {
		t.Parallel()
		got := golang.AssertNotEqual("t", "a", "b", "variants differ")
		if strings.Count(got, "%v") != 1 {
			t.Errorf("the two are equal when this fails, so printing both says it twice, got:\n%s", got)
		}
		parses(t, got)
	})

	t.Run("len compares against the count and prints it", func(t *testing.T) {
		t.Parallel()
		got := golang.AssertLen("t", "all", 3, "declares three variants")
		if !strings.Contains(got, "if len(all) != 3 {") {
			t.Errorf("length must be compared against the declared count, got:\n%s", got)
		}
		parses(t, got)
	})
}

// TestAssertOperandsCarryingBraces covers the operand shape that
// reached a consumer's build.
//
// The composite literal arrived through the builder generator, but
// nothing about the defect was its: every entry in this file composes
// its condition into an `if` header, so any caller comparing against a
// struct value got source that did not parse. Fixed here rather than
// at the one call site, which is why the cases below drive the dialect
// directly.
//
// [parses] is what makes each of these an assertion rather than a
// string comparison — the failure mode is source that will not
// compile, and only a parser reports that.
func TestAssertOperandsCarryingBraces(t *testing.T) {
	t.Parallel()

	t.Run("equal against a composite literal parses", func(t *testing.T) {
		t.Parallel()
		got := golang.AssertEqual("t", "got.Home",
			`domain.Address{Street: "test-home"}`, "the setter writes what it was given")
		if !strings.Contains(got, `if got.Home != (domain.Address{Street: "test-home"}) {`) {
			t.Errorf("the literal must be parenthesised in the header, got:\n%s", got)
		}
		parses(t, got)
	})

	t.Run("not-equal against a composite literal parses", func(t *testing.T) {
		t.Parallel()
		parses(t, golang.AssertNotEqual("t", "a", "pkg.T{}", "differs from the zero value"))
	})

	t.Run("a literal on the left is parenthesised too", func(t *testing.T) {
		t.Parallel()
		// Which side carries the literal is the caller's choice, and
		// the header rejects it in either position.
		parses(t, golang.AssertEqual("t", "pkg.T{}", "want", "the zero value round-trips"))
	})

	t.Run("an empty composite literal parses", func(t *testing.T) {
		t.Parallel()
		parses(t, golang.AssertEqual("t", "got", "Address{}", "starts zero"))
	})

	t.Run("an operand carrying no brace is left alone", func(t *testing.T) {
		t.Parallel()
		// Parenthesising everything would also have parsed. Generated
		// checks are read by whoever has to fix the failure, so the
		// ordinary comparison keeps its ordinary spelling.
		got := golang.AssertEqual("t", "got", "want", "round-trips")
		if strings.Contains(got, "(got)") || strings.Contains(got, "(want)") {
			t.Errorf("a plain operand gained parentheses it did not need, got:\n%s", got)
		}
	})

	t.Run("the nil comparisons are unaffected", func(t *testing.T) {
		t.Parallel()
		for _, got := range []string{
			golang.AssertNil("t", "err", "returns no error"),
			golang.AssertNotNil("t", "v", "returns a value"),
			golang.AssertNoError("t", "err", "succeeds"),
			golang.AssertError("t", "err", "refuses"),
		} {
			if strings.Contains(got, "(nil)") {
				t.Errorf("nil gained parentheses it did not need, got:\n%s", got)
			}
			parses(t, got)
		}
	})
}

// TestAssertDeepEqual covers the form a member `==` cannot serve.
func TestAssertDeepEqual(t *testing.T) {
	t.Parallel()

	t.Run("reports the diff rather than the two values", func(t *testing.T) {
		t.Parallel()
		// Two structs printed with %v are two walls of text with one
		// field different, which is the reading problem the diff
		// exists to remove.
		got := golang.AssertDeepEqual("t", "orderDiff", "got.Cart", "want", "the setter writes it")
		if !strings.Contains(got, "orderDiff(want, got.Cart)") {
			t.Errorf("the callee was not applied to the pair, got:\n%s", got)
		}
		if strings.Contains(got, "%v") {
			t.Errorf("a value was printed beside the diff, got:\n%s", got)
		}
		parses(t, got)
	})

	t.Run("names which side is which", func(t *testing.T) {
		t.Parallel()
		// A diff with no legend is unreadable: the reader cannot tell
		// an added line from a removed one.
		got := golang.AssertDeepEqual("t", "orderDiff", "got.Cart", "want", "the setter writes it")
		if !strings.Contains(got, "(-want +got)") {
			t.Errorf("the diff carries no legend, got:\n%s", got)
		}
	})

	t.Run("a composite literal in the want position parses", func(t *testing.T) {
		t.Parallel()
		// The shape that did not compile before: as an argument the
		// literal's brace is unambiguous, where in the header of an
		// `if` it was not.
		got := golang.AssertDeepEqual("t", "orderDiff", "got.Home",
			`shop.Address{Street: "x"}`, "the setter writes it")
		parses(t, got)
	})
}

// The condition assertions, and the parenthesising that keeps them
// honest.
func TestAssertConditions(t *testing.T) {
	t.Parallel()

	t.Run("true parenthesises before negating", func(t *testing.T) {
		t.Parallel()
		got := golang.AssertTrue("t", "a == b", "values agree")
		if !strings.Contains(got, "if !(a == b) {") {
			t.Errorf("`!a == b` negates the left operand alone, got:\n%s", got)
		}
		parses(t, got)
	})

	t.Run("false asserts the condition does not hold", func(t *testing.T) {
		t.Parallel()
		got := golang.AssertFalse("t", "seen[v]", "each value is distinct")
		if !strings.Contains(got, "if (seen[v]) {") {
			t.Errorf("falsity fails when the condition holds, got:\n%s", got)
		}
		parses(t, got)
	})

	t.Run("a condition-only failure prints no format directive", func(t *testing.T) {
		t.Parallel()
		got := golang.AssertTrue("t", "ok", "holds")
		if strings.Contains(got, "%") {
			t.Errorf("a directive with nothing to fill it renders as %%!v(MISSING), got:\n%s", got)
		}
	})
}

// The error assertions, and which of them aborts.
func TestAssertErrors(t *testing.T) {
	t.Parallel()

	t.Run("an unexpected error aborts", func(t *testing.T) {
		t.Parallel()
		got := golang.AssertNoError("t", "err", "parses")
		if !strings.Contains(got, "t.Fatalf(") {
			t.Errorf("everything after reads a value the call did not produce, got:\n%s", got)
		}
		parses(t, got)
	})

	t.Run("an expected error reports", func(t *testing.T) {
		t.Parallel()
		got := golang.AssertError("t", "err", "refuses the unknown")
		if !strings.Contains(got, "t.Errorf(") {
			t.Errorf("nothing downstream depends on an expected failure, got:\n%s", got)
		}
		parses(t, got)
	})

	t.Run("nil and not-nil compare against nil", func(t *testing.T) {
		t.Parallel()
		parses(t, golang.AssertNil("t", "cause", "carries no cause"))
		parses(t, golang.AssertNotNil("t", "cause", "carries a cause"))
	})
}

// The message is a Go literal whatever it holds.
//
// The one failure that lands in a consumer's build rather than in a
// test here: a message ending its own literal early leaves the rest
// of the line as source.
func TestAssertMessagesAreQuoted(t *testing.T) {
	t.Parallel()

	for _, msg := range []string{
		`a "quoted" word`,
		`a back\slash`,
		"a\nnewline",
	} {
		t.Run("quotes "+strings.ReplaceAll(msg, "\n", " "), func(t *testing.T) {
			t.Parallel()
			parses(t, golang.AssertEqual("t", "a", "b", msg))
		})
	}
}

// The caller names the test handle.
//
// A subtest closure shadows the outer handle, so a helper written
// against a fixed `t` reports the failure against a parent that has
// not failed.
func TestAssertUsesTheHandleItWasGiven(t *testing.T) {
	t.Parallel()

	got := golang.AssertEqual("sub", "a", "b", "holds")
	if !strings.Contains(got, "sub.Errorf(") {
		t.Errorf("the handle must be the one passed, got:\n%s", got)
	}
}

// Every entry the dialect names is registered, and every registered
// entry is named.
//
// The constants are what an override spells. One that drifts from
// the map registers a helper nothing calls, and the default goes on
// rendering — a failure with no symptom.
func TestAssertFuncMapMatchesItsNames(t *testing.T) {
	t.Parallel()

	fm := golang.AssertFuncMap()
	names := []string{
		golang.FuncAssertEqual, golang.FuncAssertDeepEqual,
		golang.FuncAssertNotEqual,
		golang.FuncAssertTrue, golang.FuncAssertFalse,
		golang.FuncAssertNil, golang.FuncAssertNotNil,
		golang.FuncAssertLen, golang.FuncAssertNoError,
		golang.FuncNeedsDiffHelper,
		golang.FuncAssertError,
	}

	t.Run("every named entry is registered", func(t *testing.T) {
		t.Parallel()
		for _, name := range names {
			if _, ok := fm[name]; !ok {
				t.Errorf("%s is named but not registered", name)
			}
		}
	})

	t.Run("every registered entry is named", func(t *testing.T) {
		t.Parallel()
		if len(fm) != len(names) {
			t.Errorf("registered %d entries for %d names; an unnamed one cannot be overridden",
				len(fm), len(names))
		}
	})
}

// The dialect reaches templates through the bundle the backend
// registers once, so a plugin calls it without declaring anything.
func TestAssertIsInAllFuncMap(t *testing.T) {
	t.Parallel()

	all := golang.AllFuncMap()
	for name := range golang.AssertFuncMap() {
		if _, ok := all[name]; !ok {
			t.Errorf("%s is missing from AllFuncMap, so no template can call it", name)
		}
	}
}

// The dialect is overrideable, which is the whole of what swapping
// it costs.
//
// [golang.FuncMap] is the reserved canonical set the backend refuses
// to let a plugin replace. An assertion entry landing in it would
// make the swap impossible and the failure would only appear in a
// consumer's build of their own override.
func TestAssertIsNotReserved(t *testing.T) {
	t.Parallel()

	reserved := golang.FuncMap()
	for name := range golang.AssertFuncMap() {
		if _, ok := reserved[name]; ok {
			t.Errorf("%s is reserved, so no plugin can override the dialect", name)
		}
	}
}
