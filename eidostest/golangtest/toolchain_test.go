// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golangtest_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/eidostest/golangtest"
)

func TestAssertCompiles(t *testing.T) {
	t.Parallel()

	t.Run("accepts output that builds", func(t *testing.T) {
		t.Parallel()
		gen(t, goodDouble).AssertCompiles(t)
	})

	t.Run("catches what no substring can", func(t *testing.T) {
		t.Parallel()
		// An undefined reference type-checks nowhere and matches every
		// substring assertion a test would write about the method.
		broken := strings.Replace(goodDouble, "return nil", "return undefinedHelper()", 1)
		s := probe(t)
		gen(t, broken).AssertCompiles(s)
		if !s.failed || !strings.Contains(s.msg, "undefinedHelper") {
			t.Fatalf("AssertCompiles = %v, %q", s.failed, s.msg)
		}
	})

	t.Run("prints the offending source with line numbers", func(t *testing.T) {
		t.Parallel()
		// The compiler names a position in code the author never wrote,
		// in a directory that is gone by the time they read the failure.
		broken := strings.Replace(goodDouble, "return nil", "return undefinedHelper()", 1)
		s := probe(t)
		gen(t, broken).AssertCompiles(s)
		if !strings.Contains(s.msg, "   1 | // Code generated") {
			t.Fatalf("message carries no numbered listing:\n%s", s.msg)
		}
	})

	t.Run("derives the module path from the resolved import path", func(t *testing.T) {
		t.Parallel()
		// A companion in an external test package imports the primary by
		// path; a flattened module would resolve it for the wrong reason.
		gen(t, goodDouble, golangtest.File{
			Path:       "printer_stub.gen_test.go",
			Src:        []byte(generatedSuite),
			ImportPath: "example.com/storepkg",
		}).AssertVets(t)
	})
}

func TestAssertSatisfies(t *testing.T) {
	t.Parallel()

	t.Run("accepts a double that stands in", func(t *testing.T) {
		t.Parallel()
		gen(t, goodDouble).AssertSatisfies(t, "PrinterStub", "Printer")
	})

	t.Run("catches a dropped variadic marker", func(t *testing.T) {
		t.Parallel()
		// The bug live in two reference generators. The double compiles,
		// every structural assertion about Print passes, and it cannot be
		// passed anywhere a Printer is expected.
		s := probe(t)
		gen(t, variadicDropped).AssertSatisfies(s, "PrinterStub", "Printer")
		if !s.failed || !strings.Contains(s.msg, "Print") {
			t.Fatalf("AssertSatisfies missed the dropped variadic: %v, %q", s.failed, s.msg)
		}
	})

	t.Run("catches a method lost altogether", func(t *testing.T) {
		t.Parallel()
		short := strings.Replace(goodDouble,
			"func (s *PrinterStub) Close() error { return nil }", "", 1)
		s := probe(t)
		gen(t, short).AssertSatisfies(s, "PrinterStub", "Printer")
		if !s.failed {
			t.Fatal("AssertSatisfies accepted a double missing a method")
		}
	})

	t.Run("reports when there is no non-test file to assert against", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		golangtest.Of(golangtest.File{Path: "x_test.go", Src: []byte("package x\n")}).
			AssertSatisfies(s, "T", "I")
		if !s.failed || !strings.Contains(s.msg, "no non-test file") {
			t.Fatalf("message %q", s.msg)
		}
	})
}

func TestAssertTestsPass(t *testing.T) {
	t.Parallel()

	t.Run("runs the suite the generator emitted", func(t *testing.T) {
		t.Parallel()
		// The loop nothing else closes: a check asserting that a t.Run of
		// some name exists passes just as well when the check is empty.
		gen(t, goodDouble, golangtest.File{
			Path:       "printer_stub.gen_test.go",
			Src:        []byte(generatedSuite),
			ImportPath: "example.com/storepkg",
		}).AssertTestsPass(t)
	})

	t.Run("fails when the generated suite fails", func(t *testing.T) {
		t.Parallel()
		failing := strings.Replace(generatedSuite,
			`if err := s.Close(); err != nil {`, `if err := s.Close(); err == nil {`, 1)
		s := probe(t)
		gen(t, goodDouble, golangtest.File{
			Path:       "printer_stub.gen_test.go",
			Src:        []byte(failing),
			ImportPath: "example.com/storepkg",
		}).AssertTestsPass(s)
		if !s.failed || !strings.Contains(s.msg, "TestPrinterStubPrint") {
			t.Fatalf("AssertTestsPass = %v, %q", s.failed, s.msg)
		}
	})

	t.Run("reports a run that emitted no suite at all", func(t *testing.T) {
		t.Parallel()
		// Silently passing here would report a green suite for a
		// generator whose test output stopped being produced.
		s := probe(t)
		gen(t, goodDouble).AssertTestsPass(s)
		if !s.failed || !strings.Contains(s.msg, "no _test.go file") {
			t.Fatalf("message %q", s.msg)
		}
	})
}
