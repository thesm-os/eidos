// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipelinetest_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/emit"
)

// fileAssertion drives one captured file through a freshly-built
// pipeline and returns the FileAssertion bound to the supplied TB.
func fileAssertion(tb testing.TB, body string) *pipelinetest.FileAssertion {
	tb.Helper()
	return targetedAssertion(tb, emit.Target{Dir: "out", Filename: "f.go", Package: "out"}, body)
}

// targetedAssertion is [fileAssertion] with the captured target
// supplied by the caller. Tests that care about how a target's
// routing metadata reads back in a failure message drive this form.
func targetedAssertion(tb testing.TB, tgt emit.Target, body string) *pipelinetest.FileAssertion {
	tb.Helper()
	p := pipelinetest.New(tb).
		WithFrontend(pipelinetest.FromNodes()).
		WithBackend(&stubBackend{
			name:   "stub-be",
			lang:   "stub",
			writes: map[emit.Target][]byte{tgt: []byte(body)},
		}).
		Build().
		Run()
	return p.AssertFile(tgt.Filename)
}

// dirlessTarget is a routed target with no directory component —
// the shape [emit.Target.JoinPath] returns "" for. Every assertion
// failure message interpolates the file's path, so this is the case
// that decides whether a failure names the file at all.
func dirlessTarget() emit.Target {
	return emit.Target{Filename: "orphan.go", Package: "orphan"}
}

// TestFileAssertion_FailureNamesTheFile covers every assertion in
// the family at once: each interpolates the captured target's path,
// and [emit.Target.JoinPath] returns "" whenever Dir is empty. A
// blank interpolation reads as `file  does not contain …` in the
// one package whose job is to explain what went wrong.
func TestFileAssertion_FailureNamesTheFile(t *testing.T) {
	t.Parallel()

	fail := map[string]func(*pipelinetest.FileAssertion){
		"Contains":    func(a *pipelinetest.FileAssertion) { a.Contains("absent") },
		"NotContains": func(a *pipelinetest.FileAssertion) { a.NotContains("body") },
		"Equals":      func(a *pipelinetest.FileAssertion) { a.Equals("other") },
		"EqualsBytes": func(a *pipelinetest.FileAssertion) { a.EqualsBytes([]byte("other")) },
	}
	for name, trigger := range fail {
		t.Run("the "+name+" failure names the file when the target has no directory", func(t *testing.T) {
			t.Parallel()
			fake := newFakeT()
			trigger(targetedAssertion(fake, dirlessTarget(), "body"))
			if !fake.Failed() {
				t.Fatalf("expected the assertion to fail")
			}
			msg := strings.Join(fake.errs, "\n")
			if !strings.Contains(msg, "orphan.go") {
				t.Fatalf("failure message must name the file; got %q", msg)
			}
		})
	}
}

func TestFileAssertion_Target(t *testing.T) {
	t.Parallel()

	t.Run("returns the target the file was rendered against", func(t *testing.T) {
		t.Parallel()
		a := fileAssertion(t, "hello")
		if a.Target().Filename != "f.go" {
			t.Fatalf("target mismatch: %+v", a.Target())
		}
	})
}

func TestFileAssertion_Bytes(t *testing.T) {
	t.Parallel()

	t.Run("returns a copy independent of the assertion's storage", func(t *testing.T) {
		t.Parallel()
		a := fileAssertion(t, "hello")
		got := a.Bytes()
		if string(got) != "hello" {
			t.Fatalf("Bytes returned wrong content: %q", got)
		}
		got[0] = 'X'
		if string(a.Bytes()) != "hello" {
			t.Fatalf("mutating the returned slice must not affect the assertion")
		}
	})
}

func TestFileAssertion_String(t *testing.T) {
	t.Parallel()

	t.Run("returns the file content as a string", func(t *testing.T) {
		t.Parallel()
		a := fileAssertion(t, "hello")
		if a.String() != "hello" {
			t.Fatalf("String returned %q, want %q", a.String(), "hello")
		}
	})
}

func TestFileAssertion_Contains(t *testing.T) {
	t.Parallel()

	t.Run("succeeds when the substring is present", func(t *testing.T) {
		t.Parallel()
		fileAssertion(t, "alpha beta gamma").Contains("beta")
	})

	t.Run("Errorf when the substring is missing", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		fileAssertion(fake, "alpha").Contains("beta")
		if !fake.Failed() {
			t.Fatalf("expected Errorf on missing substring")
		}
		if !strings.Contains(strings.Join(fake.errs, "\n"), "beta") {
			t.Fatalf("error should mention the missing substring; got %v", fake.errs)
		}
	})

	t.Run("returns the assertion for chaining", func(t *testing.T) {
		t.Parallel()
		a := fileAssertion(t, "hello world")
		if a.Contains("hello") != a {
			t.Fatalf("Contains should return its receiver for chaining")
		}
	})
}

func TestFileAssertion_NotContains(t *testing.T) {
	t.Parallel()

	t.Run("succeeds when the substring is absent", func(t *testing.T) {
		t.Parallel()
		fileAssertion(t, "alpha gamma").NotContains("beta")
	})

	t.Run("Errorf when the substring is present", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		fileAssertion(fake, "alpha beta gamma").NotContains("beta")
		if !fake.Failed() {
			t.Fatalf("expected Errorf on present substring")
		}
	})

	t.Run("returns the assertion for chaining", func(t *testing.T) {
		t.Parallel()
		a := fileAssertion(t, "hello")
		if a.NotContains("world") != a {
			t.Fatalf("NotContains should return its receiver for chaining")
		}
	})
}

func TestFileAssertion_Equals(t *testing.T) {
	t.Parallel()

	t.Run("succeeds when content is byte-identical", func(t *testing.T) {
		t.Parallel()
		fileAssertion(t, "exact").Equals("exact")
	})

	t.Run("Errorf when content differs", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		fileAssertion(fake, "got").Equals("want")
		if !fake.Failed() {
			t.Fatalf("expected Errorf on mismatched content")
		}
	})

	t.Run("returns the assertion for chaining", func(t *testing.T) {
		t.Parallel()
		a := fileAssertion(t, "hello")
		if a.Equals("hello") != a {
			t.Fatalf("Equals should return its receiver for chaining")
		}
	})
}

func TestFileAssertion_EqualsBytes(t *testing.T) {
	t.Parallel()

	t.Run("succeeds when content is byte-identical", func(t *testing.T) {
		t.Parallel()
		fileAssertion(t, "exact").EqualsBytes([]byte("exact"))
	})

	t.Run("Errorf when content differs", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		fileAssertion(fake, "got").EqualsBytes([]byte("want"))
		if !fake.Failed() {
			t.Fatalf("expected Errorf on mismatched bytes")
		}
	})

	t.Run("returns the assertion for chaining", func(t *testing.T) {
		t.Parallel()
		a := fileAssertion(t, "hello")
		if a.EqualsBytes([]byte("hello")) != a {
			t.Fatalf("EqualsBytes should return its receiver for chaining")
		}
	})
}
