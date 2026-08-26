// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/lang/typescript/typescripttest"
)

func TestFile(t *testing.T) {
	t.Parallel()

	t.Run("a routed file reports its directory", func(t *testing.T) {
		t.Parallel()
		// Two files that landed in different directories have to stay
		// there for a relative specifier between them to resolve the
		// way it will for a consumer.
		if got := typescripttest.TSFile("stubs/user.gen.ts", "").Dir(); got != "stubs" {
			t.Errorf("Dir = %q, want stubs", got)
		}
		if got := typescripttest.TSFile("user.gen.ts", "").Dir(); got != "" {
			t.Errorf("Dir = %q, want the project root", got)
		}
	})

	t.Run("a declaration file is told apart", func(t *testing.T) {
		t.Parallel()
		if !typescripttest.TSFile("user.d.ts", "").IsDeclaration() {
			t.Error("a .d.ts file is not recognised")
		}
		if typescripttest.TSFile("user.ts", "").IsDeclaration() {
			t.Error("a .ts file is reported as a declaration file")
		}
	})
}

func TestGeneratedAddressing(t *testing.T) {
	t.Parallel()

	t.Run("files come back sorted", func(t *testing.T) {
		t.Parallel()
		// Every failure message lists the files it looked at, and a
		// stable order is what keeps two runs of a failing test
		// reporting the same thing.
		got := typescripttest.Of(
			typescripttest.TSFile("z.ts", primary),
			typescripttest.TSFile("a.ts", primary),
		).Files()
		if got[0].Path != "a.ts" {
			t.Fatalf("files = %v, want sorted", got)
		}
	})

	t.Run("the produced set is pinned", func(t *testing.T) {
		t.Parallel()
		// The name a consumer sees is not the name the plugin chose:
		// Layout composes it, and a routing directive can move the whole
		// thing.
		gen := typescripttest.Of(typescripttest.TSFile("user.gen.ts", primary))
		gen.AssertPaths(t, "user.gen.ts")

		s := probe(t)
		gen.AssertPaths(s, "other.gen.ts")
		assertReports(t, s, "user.gen.ts", "other.gen.ts")
	})

	t.Run("a file is addressed by path", func(t *testing.T) {
		t.Parallel()
		gen := typescripttest.Of(typescripttest.TSFile("user.gen.ts", primary))
		if gen.File(t, "user.gen.ts").Path() != "user.gen.ts" {
			t.Fatal("File returned the wrong file")
		}

		s := probe(t)
		gen.File(s, "absent.ts")
		assertReports(t, s, "absent.ts", "user.gen.ts")
	})

	t.Run("a file is addressed by suffix", func(t *testing.T) {
		t.Parallel()
		// The vocabulary a plugin author already has: outputs are
		// declared by suffix, so a test asking for one stays correct
		// when the origin it routed from is renamed.
		gen := typescripttest.Of(
			typescripttest.TSFile("user.gen.ts", primary),
			typescripttest.TSFile("user.stub.ts", primary),
		)
		if gen.Suffixed(t, ".stub.ts").Path() != "user.stub.ts" {
			t.Fatal("Suffixed returned the wrong file")
		}

		none := probe(t)
		gen.Suffixed(none, ".absent.ts")
		assertReports(t, none, ".absent.ts")

		many := probe(t)
		gen.Suffixed(many, ".ts")
		assertReports(t, many, "2 files", "whichever the sink happened to order first")
	})

	t.Run("a lone file is the primary one", func(t *testing.T) {
		t.Parallel()
		gen := typescripttest.Of(typescripttest.TSFile("user.gen.ts", primary))
		if gen.Primary(t).Path() != "user.gen.ts" {
			t.Fatal("Primary returned the wrong file")
		}
	})

	t.Run("Primary refuses to guess", func(t *testing.T) {
		t.Parallel()
		// A generator emitting a companion alongside its surface has
		// two, and an assertion that silently picked one would be about
		// whichever the sink ordered first.
		empty := probe(t)
		typescripttest.Of().Primary(empty)
		assertReports(t, empty, "no files")

		many := probe(t)
		typescripttest.Of(
			typescripttest.TSFile("a.ts", primary),
			typescripttest.TSFile("b.ts", primary),
		).Primary(many)
		assertReports(t, many, "2 files", "File or Suffixed")
	})
}

func TestAssertParsesOverASet(t *testing.T) {
	t.Parallel()

	t.Run("every file is checked", func(t *testing.T) {
		t.Parallel()
		typescripttest.Of(
			typescripttest.TSFile("a.ts", primary),
			typescripttest.TSFile("b.ts", primary),
		).AssertParses(t)
	})

	t.Run("one broken file among many is reported", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		typescripttest.Of(
			typescripttest.TSFile("good.ts", primary),
			typescripttest.TSFile("bad.ts", broken),
		).AssertParses(s)
		assertReports(t, s, "bad.ts")
	})

	t.Run("an empty set is reported rather than passing", func(t *testing.T) {
		t.Parallel()
		// Otherwise a run that produced nothing passes every assertion
		// downstream having looked at nothing, which reads as a
		// generator that works.
		s := probe(t)
		typescripttest.Of().AssertParses(s)
		assertReports(t, s, "no files")
	})
}

func TestAssertTypeChecks(t *testing.T) {
	t.Parallel()

	t.Run("output that type-checks passes", func(t *testing.T) {
		t.Parallel()
		typescripttest.
			Of(typescripttest.TSFile("echo.gen.ts", mistyped)).
			WithSource(typescripttest.TSFile("user.ts", supportModule)).
			AssertTypeChecks(t)
	})

	t.Run("a type error names the file and the line", func(t *testing.T) {
		t.Parallel()
		// The assertion that catches what parsing cannot: this file has
		// no syntax error and is wrong about a type it imported.
		s := probe(t)
		typescripttest.
			Of(typescripttest.TSFile("echo.gen.ts", callsWithWrongType)).
			WithSource(typescripttest.TSFile("user.ts", supportModule)).
			AssertTypeChecks(s)
		if s.skipped {
			t.Skip("no TypeScript compiler; the assertion itself was not exercised")
		}
		assertReports(t, s, "echo.gen.ts", "does not type-check")
	})

	t.Run("a missing support module is what the failure names", func(t *testing.T) {
		t.Parallel()
		// Rather than whatever the generator got wrong, which is what a
		// test that forgot WithSource would otherwise go looking for.
		s := probe(t)
		typescripttest.
			Of(typescripttest.TSFile("echo.gen.ts", mistyped)).
			AssertTypeChecks(s)
		if s.skipped {
			t.Skip("no TypeScript compiler; the assertion itself was not exercised")
		}
		assertReports(t, s, "./user")
	})

	t.Run("the strict options are on by default", func(t *testing.T) {
		t.Parallel()
		// A check run without them passes on output that fails for the
		// consumer — exactOptionalPropertyTypes alone is the difference
		// between `x?: T` and `x: T | undefined`.
		s := probe(t)
		typescripttest.
			Of(typescripttest.TSFile("loose.ts", looselyTyped)).
			AssertTypeChecks(s)
		if s.skipped {
			t.Skip("no TypeScript compiler; the assertion itself was not exercised")
		}
		assertReports(t, s, "loose.ts")
	})

	t.Run("an option can be relaxed on purpose", func(t *testing.T) {
		t.Parallel()
		typescripttest.
			Of(typescripttest.TSFile("loose.ts", looselyTyped)).
			WithCompilerOptions(map[string]any{"strict": false}).
			AssertTypeChecks(t)
	})
}

// callsWithWrongType parses cleanly and passes a number where the
// module it imports from declares a string. Nothing but a type check
// finds it.
const callsWithWrongType = `// Code generated by eidos. DO NOT EDIT.

import type { User } from './user';

export declare function take(u: User): void;

export const wrong: User = { id: 42 };
`

// looselyTyped compiles under `strict: false` and fails under the
// strict defaults, which is what makes it the probe for them.
const looselyTyped = `// Code generated by eidos. DO NOT EDIT.

export declare function take(u: string): void;

export const value: string = null;
`

// dumpsAreReadable pins that a failure prints the file it is about.
func TestFailureListing(t *testing.T) {
	t.Parallel()

	t.Run("a type-check failure prints the numbered source", func(t *testing.T) {
		t.Parallel()
		// A tsc failure names code the author did not write, at line
		// numbers in a directory that no longer exists. Without the
		// listing the assertion is worse than the substring check it
		// replaces.
		s := probe(t)
		typescripttest.
			Of(typescripttest.TSFile("echo.gen.ts", callsWithWrongType)).
			WithSource(typescripttest.TSFile("user.ts", supportModule)).
			AssertTypeChecks(s)
		if s.skipped {
			t.Skip("no TypeScript compiler; the assertion itself was not exercised")
		}
		if !strings.Contains(s.msg, "--- echo.gen.ts ---") {
			t.Errorf("the failure does not list the generated file: %s", s.msg)
		}
		if !strings.Contains(s.msg, "--- user.ts ---") {
			t.Errorf("the failure does not list the support module: %s", s.msg)
		}
	})
}
