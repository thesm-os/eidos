// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golangtest_test

import (
	"slices"
	"strings"
	"testing"
)

func TestScope(t *testing.T) {
	t.Parallel()

	t.Run("narrows a substring to one method", func(t *testing.T) {
		t.Parallel()
		// The question a whole-file check cannot ask: not "does the file
		// mention this" but "does *this* method".
		parse(t).InMethod(t, "StoreStub", "Get").AssertContains(t, "answer(s.OnGet)")
	})

	t.Run("proves the same construct is absent elsewhere", func(t *testing.T) {
		t.Parallel()
		// Counting occurrences across a file and inferring which method
		// has one is what this replaces; the answer here stops moving
		// when an unrelated method is added.
		parse(t).InMethod(t, "StoreStub", "Close").AssertNotContains(t, "answer(")
	})

	t.Run("counts within the scope", func(t *testing.T) {
		t.Parallel()
		parse(t).InMethod(t, "StoreStub", "Get").AssertCount(t, "r.", 2)
	})

	t.Run("carries the body into the failure", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).InMethod(t, "StoreStub", "Get").AssertContains(s, "absent")
		if !s.failed || !strings.Contains(s.msg, "answer(s.OnGet)") {
			t.Fatalf("message %q does not carry the body", s.msg)
		}
	})

	t.Run("reports a count that does not match", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).InMethod(t, "StoreStub", "Get").AssertCount(s, "r.", 5)
		if !s.failed || !strings.Contains(s.msg, "2 time(s), want 5") {
			t.Fatalf("message %q", s.msg)
		}
	})

	t.Run("narrows to a plain function", func(t *testing.T) {
		t.Parallel()
		parse(t).InFunc(t, "NewStoreStub").AssertContains(t, "&StoreStub{}")
	})

	t.Run("chains off an absent declaration without panicking", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).InFunc(s, "Absent").AssertContains(s, "x").AssertNotContains(s, "y")
		parse(t).InMethod(s, "StoreStub", "Absent").AssertCount(s, "x", 1)
	})

	t.Run("exposes the body for a caller's own assertion", func(t *testing.T) {
		t.Parallel()
		if got := parse(t).InMethod(t, "StoreStub", "Close").Body(); got != "{\n}" {
			t.Fatalf("Body = %q", got)
		}
	})
}

func TestSubtests(t *testing.T) {
	t.Parallel()

	t.Run("lists what a generated suite declares", func(t *testing.T) {
		t.Parallel()
		got := parseTests(t).Subtests(t, "TestStoreStubGet")
		want := []string{
			"records what it was called with",
			"answers with the value pinned by Returns",
		}
		if !slices.Equal(got, want) {
			t.Fatalf("Subtests = %v, want %v", got, want)
		}
	})

	t.Run("finds one by name", func(t *testing.T) {
		t.Parallel()
		parseTests(t).AssertSubtest(t, "TestStoreStubGet", "records what it was called with")
	})

	t.Run("lists them all when one is absent", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parseTests(t).AssertSubtest(s, "TestStoreStubGet", "absent check")
		if !s.failed || !strings.Contains(s.msg, "records what it was called with") {
			t.Fatalf("message %q does not list the subtests", s.msg)
		}
	})

	t.Run("proves a withheld check is absent", func(t *testing.T) {
		t.Parallel()
		// The half that matters most: only its absence distinguishes
		// "correctly omitted" from "silently dropped".
		const absent = "answers with the value pinned by Returns"
		parseTests(t).AssertNoSubtest(t, "TestStoreStubClose", absent)
		s := probe(t)
		parseTests(t).AssertNoSubtest(s, "TestStoreStubClose", "records what it was called with")
		if !s.failed {
			t.Fatal("AssertNoSubtest accepted a declared subtest")
		}
	})

	t.Run("reports an absent test function", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parseTests(t).Subtests(s, "TestAbsent")
		if !s.failed || !strings.Contains(s.msg, "TestStoreStubGet") {
			t.Fatalf("message %q does not list the functions", s.msg)
		}
	})

	t.Run("lists the generated test entry points", func(t *testing.T) {
		t.Parallel()
		want := []string{"TestStoreStubGet", "TestStoreStubClose"}
		if got := parseTests(t).TestFuncs(); !slices.Equal(got, want) {
			t.Fatalf("TestFuncs = %v, want %v", got, want)
		}
	})
}

func TestParallelDeclaration(t *testing.T) {
	t.Parallel()

	t.Run("accepts a suite that goes parallel", func(t *testing.T) {
		t.Parallel()
		parseTests(t).AssertParallel(t, "TestStoreStubGet")
	})

	t.Run("rejects one that does not", func(t *testing.T) {
		t.Parallel()
		// A subtest's own call says nothing about its parent, and the
		// parent is what every consumer's CI serialises on.
		s := probe(t)
		parseTests(t).AssertParallel(s, "TestStoreStubClose")
		if !s.failed || !strings.Contains(s.msg, "t.Parallel") {
			t.Fatalf("message %q", s.msg)
		}
	})

	t.Run("reports an absent function", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parseTests(t).AssertParallel(s, "TestAbsent")
		if !s.failed {
			t.Fatal("AssertParallel accepted an absent function")
		}
	})
}
