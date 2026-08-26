// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/lang/typescript/typescripttest"
)

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("reads a well-formed file", func(t *testing.T) {
		t.Parallel()
		if got := parse(t).Path(); got != "user.gen.ts" {
			t.Fatalf("Path = %q", got)
		}
	})

	t.Run("keeps the bytes it was given", func(t *testing.T) {
		t.Parallel()
		if string(parse(t).Bytes()) != primary {
			t.Fatal("Bytes did not round-trip")
		}
	})

	t.Run("a broken file still parses into a queryable tree", func(t *testing.T) {
		t.Parallel()
		// tree-sitter recovers from syntax errors, so a test can still
		// ask structural questions of a file it knows is broken. That
		// is what lets AssertParses be an assertion rather than a
		// precondition.
		s := typescripttest.Parse(t, typescripttest.TSFile("broken.ts", broken))
		if s == nil {
			t.Fatal("a recoverable file produced no tree")
		}
	})

	t.Run("a tsx file selects the other grammar", func(t *testing.T) {
		t.Parallel()
		// `<T>value` is a type assertion in `.ts` and the opening of a
		// JSX element in `.tsx`, so the same bytes parse to different
		// trees and one grammar for both would mis-parse one of them.
		src := "export const x = <T>(a: T): T => a;\n"
		if typescripttest.Parse(t, typescripttest.TSFile("c.tsx", src)) == nil {
			t.Fatal("the tsx grammar produced no tree")
		}
	})
}

func TestAssertParses(t *testing.T) {
	t.Parallel()

	t.Run("a clean file passes", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertParses(t)
	})

	t.Run("a syntax error names the line and shows it", func(t *testing.T) {
		t.Parallel()
		// The position is in generated code the author never wrote and
		// cannot open, so a bare line number diagnoses nothing.
		s := probe(t)
		typescripttest.Parse(t, typescripttest.TSFile("broken.ts", broken)).AssertParses(s)
		assertReports(t, s, "broken.ts", "not valid TypeScript", "export interface User")
	})
}

func TestAssertGeneratedHeader(t *testing.T) {
	t.Parallel()

	t.Run("the marker is recognised", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertGeneratedHeader(t)
	})

	t.Run("a file without it is reported", func(t *testing.T) {
		t.Parallel()
		// A generator that stopped emitting it starts having its output
		// reformatted by the consumer's tooling and shown in review
		// diffs.
		s := probe(t)
		typescripttest.
			Parse(t, typescripttest.TSFile("x.ts", "export type A = string;\n")).
			AssertGeneratedHeader(s)
		assertReports(t, s, "x.ts", "export type A = string;")
	})
}

func TestImports(t *testing.T) {
	t.Parallel()

	t.Run("every specifier is listed, unquoted", func(t *testing.T) {
		t.Parallel()
		got := parse(t).Imports()
		if len(got) != 1 || got[0] != "./models/person" {
			t.Fatalf("Imports = %v", got)
		}
	})

	t.Run("a re-export counts as reaching for the module", func(t *testing.T) {
		t.Parallel()
		// Which is what a test asking "does this file depend on that
		// module" means either way.
		src := "export { User } from './user';\nexport * from './role';\n"
		got := typescripttest.Parse(t, typescripttest.TSFile("i.ts", src)).Imports()
		if len(got) != 2 {
			t.Fatalf("Imports = %v, want both re-exports", got)
		}
	})

	t.Run("a plain export is not an import", func(t *testing.T) {
		t.Parallel()
		if got := typescripttest.
			Parse(t, typescripttest.TSFile("i.ts", "export type A = string;\n")).
			Imports(); len(got) != 0 {
			t.Fatalf("Imports = %v, want none", got)
		}
	})

	t.Run("a missing specifier is reported with what is there", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertImports(s, "./absent")
		assertReports(t, s, "./absent", "./models/person")
	})

	t.Run("a specifier that must not be there is reported", func(t *testing.T) {
		t.Parallel()
		// The assertion a self-import bug fails: a module importing
		// itself is legal TypeScript that resolves to a cycle.
		parse(t).AssertNoImport(t, "./self")

		s := probe(t)
		parse(t).AssertNoImport(s, "./models/person")
		assertReports(t, s, "./models/person", "must not")
	})

	t.Run("the whole import set can be pinned", func(t *testing.T) {
		t.Parallel()
		// An extra specifier means the generator registered an import
		// for a type it did not emit, which fails under noUnusedLocals
		// in the consumer's build and nowhere here.
		parse(t).AssertImportsOnly(t, "./models/person")

		s := probe(t)
		parse(t).AssertImportsOnly(s, "./models/person", "./extra")
		assertReports(t, s, "exactly")
	})
}

func TestTextAssertions(t *testing.T) {
	t.Parallel()

	t.Run("a substring is found and its absence is reported", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertContains(t, "export enum Role")

		s := probe(t)
		parse(t).AssertContains(s, "export enum Missing")
		assertReports(t, s, "export enum Missing", "user.gen.ts")
	})

	t.Run("a forbidden substring is reported with its line", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertNotContains(t, "any")

		s := probe(t)
		parse(t).AssertNotContains(s, "export enum Role")
		assertReports(t, s, "must not", "line 7")
	})
}

func TestAssertGolden(t *testing.T) {
	t.Parallel()

	t.Run("a matching golden file passes", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "user.golden.ts")
		if err := os.WriteFile(path, []byte(primary), 0o600); err != nil {
			t.Fatalf("seed golden: %v", err)
		}
		parse(t).AssertGolden(t, path)
	})

	t.Run("a difference is reported", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "user.golden.ts")
		if err := os.WriteFile(path, []byte("export type A = string;\n"), 0o600); err != nil {
			t.Fatalf("seed golden: %v", err)
		}
		s := probe(t)
		parse(t).AssertGolden(s, path)
		if !s.failed {
			t.Fatal("a mismatched golden file passed")
		}
	})
}

func TestDeclOrder(t *testing.T) {
	t.Parallel()

	t.Run("every declaration is named in source order", func(t *testing.T) {
		t.Parallel()
		got := parse(t).DeclNames()
		want := []string{"ID", "Role", "User", "Repo", "MAX", "createUser"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("DeclNames = %v, want %v", got, want)
		}
	})

	t.Run("order is asserted pairwise and along a list", func(t *testing.T) {
		t.Parallel()
		// A generator emitting a sorted surface that stopped sorting
		// produces a diff on every run against an unchanged source.
		parse(t).AssertOrder(t, "ID", "User").
			AssertOrderAll(t, "ID", "Role", "User", "Repo")
	})

	t.Run("a reversed pair is reported with the order it found", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertOrder(s, "User", "ID")
		assertReports(t, s, "after", "ID")
	})

	t.Run("an absent declaration is reported by name", func(t *testing.T) {
		t.Parallel()
		first := probe(t)
		parse(t).AssertOrder(first, "Absent", "User")
		assertReports(t, first, "Absent")

		second := probe(t)
		parse(t).AssertOrder(second, "User", "Absent")
		assertReports(t, second, "Absent")
	})
}

func TestAssertFormatted(t *testing.T) {
	t.Parallel()

	t.Run("canonical output passes", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertFormatted(t)
	})

	t.Run("a trailing space is reported", func(t *testing.T) {
		t.Parallel()
		// Nothing downstream reports it — the output parses, it
		// type-checks, and it lands in a consumer's repository looking
		// hand-edited.
		s := probe(t)
		typescripttest.
			Parse(t, typescripttest.TSFile("x.ts", "export type A = string;   \n")).
			AssertFormatted(s)
		assertReports(t, s, "not in canonical form")
	})

	t.Run("a doubled blank line is reported", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		typescripttest.
			Parse(t, typescripttest.TSFile("x.ts", "export type A = string;\n\n\n\nexport type B = A;\n")).
			AssertFormatted(s)
		assertReports(t, s, "not in canonical form")
	})

	t.Run("the normalised form is what the assertion compares against", func(t *testing.T) {
		t.Parallel()
		got := typescripttest.
			Parse(t, typescripttest.TSFile("x.ts", "\n\nexport type A = string;  \n\n\n\n")).
			Normalised()
		if string(got) != "export type A = string;\n" {
			t.Fatalf("Normalised = %q", got)
		}
	})
}

func TestAssertDocumented(t *testing.T) {
	t.Parallel()

	t.Run("a fully documented module passes", func(t *testing.T) {
		t.Parallel()
		src := `// Code generated by eidos. DO NOT EDIT.

/** A user. */
export interface User {
  id: string;
}

/**
 * The ceiling.
 */
export const MAX: number = 1;
`
		typescripttest.Parse(t, typescripttest.TSFile("u.ts", src)).AssertDocumented(t)
	})

	t.Run("every bare export is named at once", func(t *testing.T) {
		t.Parallel()
		// A generator that documents most of what it emits and forgets
		// one is the common case, so reporting the first would send the
		// author round the loop per declaration.
		src := `// Code generated by eidos. DO NOT EDIT.

/** Documented. */
export interface A {
  x: string;
}

export interface B {
  x: string;
}

export type C = string;
`
		s := probe(t)
		typescripttest.Parse(t, typescripttest.TSFile("u.ts", src)).AssertDocumented(s)
		assertReports(t, s, "[B C]")
	})

	t.Run("an unexported declaration is not asked for docs", func(t *testing.T) {
		t.Parallel()
		// A consumer cannot see it, so its documentation is nobody's
		// business but the generator's.
		src := "// Code generated by eidos. DO NOT EDIT.\n\ninterface Hidden {\n  x: string;\n}\n"
		typescripttest.Parse(t, typescripttest.TSFile("u.ts", src)).AssertDocumented(t)
	})

	t.Run("a line comment is not a doc block", func(t *testing.T) {
		t.Parallel()
		// `//` above a declaration is a note to whoever reads the
		// source; a consumer's editor shows only JSDoc.
		src := "// Code generated by eidos. DO NOT EDIT.\n\n// A note.\nexport type A = string;\n"
		s := probe(t)
		typescripttest.Parse(t, typescripttest.TSFile("u.ts", src)).AssertDocumented(s)
		assertReports(t, s, "[A]")
	})
}
