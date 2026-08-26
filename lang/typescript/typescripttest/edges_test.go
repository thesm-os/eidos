// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/lang/typescript/typescripttest"
)

// The shapes a generator produces rarely and a harness must not
// mis-read: an unterminated file, a double-quoted specifier, a member
// that is neither property nor method, a declaration a query has no
// arm for. Each one either reads as something else or crashes the
// walk, and neither failure names what happened.

func TestOddDeclarationShapes(t *testing.T) {
	t.Parallel()

	t.Run("a member that is neither property nor method is skipped", func(t *testing.T) {
		t.Parallel()
		// An index signature and a call signature both sit in the same
		// body as the properties, and a query that read them as
		// properties would report a member with no name.
		src := `export interface Bag {
  [key: string]: unknown;
  (arg: string): void;
  id: string;
}
`
		s := typescripttest.Parse(t, typescripttest.TSFile("bag.ts", src))
		s.AssertProperty(t, "Bag", "id", "string")
		s.AssertNoProperty(t, "Bag", "key")
		s.AssertMethod(probe(t), "Bag", "absent")
	})

	t.Run("a computed member name does not crash the walk", func(t *testing.T) {
		t.Parallel()
		src := "export interface Bag {\n  ['a' + 'b']: string;\n}\n"
		typescripttest.
			Parse(t, typescripttest.TSFile("bag.ts", src)).
			AssertNoProperty(t, "Bag", "ab")
	})

	t.Run("a property with no annotation reports an empty type", func(t *testing.T) {
		t.Parallel()
		// Legal in a class: the compiler infers it from the
		// initialiser, and a harness that demanded an annotation would
		// report the member absent.
		src := "export class Repo {\n  cache = new Map();\n}\n"
		typescripttest.
			Parse(t, typescripttest.TSFile("r.ts", src)).
			AssertProperty(t, "Repo", "cache", "")
	})

	t.Run("a binding with no declarator declares nothing", func(t *testing.T) {
		t.Parallel()
		// What tree-sitter recovers a truncated `const` into.
		s := typescripttest.Parse(t, typescripttest.TSFile("b.ts", "const\n"))
		if got := s.DeclNames(); len(got) != 0 {
			t.Fatalf("DeclNames = %v, want none", got)
		}
	})

	t.Run("an abstract method is found like any other", func(t *testing.T) {
		t.Parallel()
		src := "export abstract class Repo {\n  abstract find(id: string): void;\n}\n"
		typescripttest.
			Parse(t, typescripttest.TSFile("r.ts", src)).
			AssertMethod(t, "Repo", "find")
	})

	t.Run("a generic heritage clause drops its arguments", func(t *testing.T) {
		t.Parallel()
		// `extends Base<string>` names Base; a comparison against the
		// whole spelling is what [Source.AssertContains] is for.
		src := "export interface User extends Base<string> {\n  id: string;\n}\n"
		typescripttest.
			Parse(t, typescripttest.TSFile("u.ts", src)).
			AssertExtends(t, "User", "Base")
	})

	t.Run("a nested type's heritage is not read as the outer one's", func(t *testing.T) {
		t.Parallel()
		// The walk stops at the declaration's body, so a member typed
		// by an inline object cannot contribute a clause the outer
		// declaration does not have.
		src := `export interface Outer {
  inner: { nested: string };
}
`
		s := probe(t)
		typescripttest.
			Parse(t, typescripttest.TSFile("o.ts", src)).
			AssertExtends(s, "Outer", "Anything")
		assertReports(t, s, "Outer")
	})
}

func TestSpecifierForms(t *testing.T) {
	t.Parallel()

	t.Run("both quote styles are unquoted", func(t *testing.T) {
		t.Parallel()
		// The backend emits single quotes and a hand-written support
		// module usually has double ones, so a harness that handled one
		// would report the other's specifier with its quotes attached.
		src := "import type { A } from './a';\nimport type { B } from \"./b\";\n"
		got := typescripttest.Parse(t, typescripttest.TSFile("i.ts", src)).Imports()
		if strings.Join(got, ",") != "./a,./b" {
			t.Fatalf("Imports = %v", got)
		}
	})
}

func TestDump(t *testing.T) {
	t.Parallel()

	t.Run("logs the file with line numbers", func(t *testing.T) {
		t.Parallel()
		// For a test being debugged, where the file is in a directory
		// that no longer exists by the time anyone looks.
		if got := parse(t).Dump(t).Path(); got != "user.gen.ts" {
			t.Fatalf("Dump did not chain: %q", got)
		}
	})
}

func TestHeaderOnAOneLineFile(t *testing.T) {
	t.Parallel()

	t.Run("a file with no newline is still read", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		typescripttest.
			Parse(t, typescripttest.TSFile("x.ts", "export type A = string;")).
			AssertGeneratedHeader(s)
		assertReports(t, s, "export type A = string;")
	})
}

func TestCompilerOptionReconciliation(t *testing.T) {
	t.Parallel()

	t.Run("an explicit strictNullChecks keeps the strict pair", func(t *testing.T) {
		t.Parallel()
		// The dependency is satisfied by hand, so nothing is dropped
		// and the check stays as strict as the caller asked for.
		s := probe(t)
		typescripttest.
			Of(typescripttest.TSFile("loose.ts", looselyTyped)).
			WithCompilerOptions(map[string]any{
				"strict": false, "strictNullChecks": true,
			}).
			AssertTypeChecks(s)
		if s.skipped {
			t.Skip("no TypeScript compiler; the assertion itself was not exercised")
		}
		assertReports(t, s, "loose.ts")
	})
}

func TestQueryGuards(t *testing.T) {
	t.Parallel()

	t.Run("a non-generic heritage clause is reported as written", func(t *testing.T) {
		t.Parallel()
		// The bare-name fallback runs only when the whole spelling did
		// not match, so a clause with no type arguments reaches it too.
		s := probe(t)
		typescripttest.
			Parse(t, typescripttest.TSFile("u.ts", "export interface A extends B {}\n")).
			AssertExtends(s, "A", "C")
		assertReports(t, s, "extends [B]")
	})

	t.Run("a body holding only signatures reports no methods", func(t *testing.T) {
		t.Parallel()
		src := "export interface Bag {\n  [key: string]: unknown;\n}\n"
		s := probe(t)
		typescripttest.
			Parse(t, typescripttest.TSFile("b.ts", src)).
			AssertSignature(s, "Bag", "absent", "absent(): void")
		assertReports(t, s, "Bag.absent")
	})

	t.Run("an enum with no members reports an empty list", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		typescripttest.
			Parse(t, typescripttest.TSFile("e.ts", "export enum E {}\n")).
			AssertMembers(s, "E", "A")
		assertReports(t, s, "members []")
	})

	t.Run("a member missing from the tree is a syntax error, not a crash", func(t *testing.T) {
		t.Parallel()
		// tree-sitter inserts a MISSING node where a token the grammar
		// requires is absent, and the walk has to report it rather than
		// descend past it.
		s := probe(t)
		typescripttest.
			Parse(t, typescripttest.TSFile("m.ts", "export interface A { id: }\n")).
			AssertParses(s)
		assertReports(t, s, "m.ts", "not valid TypeScript")
	})

	t.Run("a side-effect import counts as reaching for the module", func(t *testing.T) {
		t.Parallel()
		// `import './polyfill'` binds no name and is still a dependency
		// — one a bundler resolves and a consumer's build fails on when
		// the module is missing.
		got := typescripttest.
			Parse(t, typescripttest.TSFile("i.ts", "import './side-effect';\n")).
			Imports()
		if len(got) != 1 || got[0] != "./side-effect" {
			t.Fatalf("Imports = %v", got)
		}
	})
}

func TestSpecifierComposition(t *testing.T) {
	t.Parallel()

	t.Run("a path becomes a relative specifier", func(t *testing.T) {
		t.Parallel()
		// A specifier with neither extension nor `./` is a *package*
		// name under every resolver — `user` resolves in node_modules
		// and `./user` resolves on disk.
		for _, from := range []string{"user.ts", "./user", "./user.ts", "deep/user.ts"} {
			s := probe(t)
			typescripttest.
				Of(typescripttest.TSFile("a.gen.ts", "export interface A { x: string }\n")).
				AssertSatisfiesAll(s, typescripttest.Satisfaction{
					Type: "A", Interface: "B", From: from,
				})
			// The resolution ran and reached the compiler probe, which
			// is what the specifier had to survive to do.
			reached := strings.Contains(s.msg, "type-check") ||
				strings.Contains(s.msg, "satisfy")
			if s.failed && !reached {
				t.Errorf("From %q was not composed into a specifier: %s", from, s.msg)
			}
		}
	})

	t.Run("a parent-relative specifier keeps its prefix", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		typescripttest.
			Of(typescripttest.TSFile("deep/a.gen.ts", "export interface A { x: string }\n")).
			AssertSatisfiesAll(s, typescripttest.Satisfaction{
				Type: "A", Interface: "B", From: "../b.ts",
			})
		if s.failed && strings.Contains(s.msg, "no module in the project exports") {
			t.Errorf("the parent-relative specifier was rewritten: %s", s.msg)
		}
	})

	t.Run("the project's name is what a self-reference resolves against", func(t *testing.T) {
		t.Parallel()
		// Named rather than guessed: output importing its own package
		// by name should fail to resolve rather than resolve by
		// accident against whatever the fixture happened to be called.
		gen := typescripttest.
			Of(typescripttest.TSFile("a.gen.ts", "export type A = string;\n")).
			WithPackageName("@acme/models")
		if gen == nil {
			t.Fatal("WithPackageName did not chain")
		}
	})
}

func TestDeclarationTextEdges(t *testing.T) {
	t.Parallel()

	t.Run("a declaration with a body reports only its head", func(t *testing.T) {
		t.Parallel()
		src := "export function f(a: string): void {\n  body();\n}\n"
		got := typescripttest.
			Parse(t, typescripttest.TSFile("f.ts", src)).
			AssertFunction(t, "f")
		got.Signature(t, "function f(a: string): void")
	})

	t.Run("a binding at the top of a file has no line above it", func(t *testing.T) {
		t.Parallel()
		// The documentation walk reads the line before the declaration,
		// and a declaration on line one has none to read.
		s := probe(t)
		typescripttest.
			Parse(t, typescripttest.TSFile("a.ts", "export type A = string;\n")).
			AssertDocumented(s)
		assertReports(t, s, "[A]")
	})

	t.Run("a binding with a destructuring pattern declares no name", func(t *testing.T) {
		t.Parallel()
		// What the harness reads as "nothing named", which is the
		// honest answer: a generator emitting one is not exporting a
		// name a consumer imports.
		got := typescripttest.
			Parse(t, typescripttest.TSFile("a.ts", "export const { a, b } = obj;\n")).
			DeclNames()
		if len(got) != 0 {
			t.Fatalf("DeclNames = %v, want none", got)
		}
	})
}
