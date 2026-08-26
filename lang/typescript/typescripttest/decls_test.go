// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript/typescripttest"
)

func TestDeclarationKinds(t *testing.T) {
	t.Parallel()

	t.Run("each kind is found by name", func(t *testing.T) {
		t.Parallel()
		s := parse(t)
		for name, found := range map[string]string{
			"interface":  s.AssertInterface(t, "User").Name(),
			"class":      s.AssertClass(t, "Repo").Name(),
			"type alias": s.AssertAlias(t, "ID").Name(),
			"enum":       s.AssertEnum(t, "Role").Name(),
			"function":   s.AssertFunction(t, "createUser").Name(),
			"binding":    s.AssertBinding(t, "MAX").Name(),
		} {
			if found == "" {
				t.Errorf("%s: the declaration came back unnamed", name)
			}
		}
	})

	t.Run("a declaration returns its own source text", func(t *testing.T) {
		t.Parallel()
		// The escape hatch: an assertion this package has no method for
		// reads the text and does its own thing. The `export` is not
		// part of it — see [Decl.Text].
		if got := parse(t).AssertAlias(t, "ID").Text(); got != "type ID = string;" {
			t.Fatalf("Text = %q", got)
		}
	})

	t.Run("an absent declaration is reported with what is there", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertInterface(s, "Absent")
		assertReports(t, s, "no interface Absent", "User")
	})

	t.Run("the wrong kind is reported as the kind it is", func(t *testing.T) {
		t.Parallel()
		// Which is what is actually true: the name is taken, by
		// something else. A "not found" would send the reader looking
		// for a generator that emitted nothing.
		s := probe(t)
		parse(t).AssertClass(s, "User")
		assertReports(t, s, "User", "interface_declaration", "not as a(n) class")
	})

	t.Run("a name that must be free is checked without a kind", func(t *testing.T) {
		t.Parallel()
		// A generator that started emitting a class where it used to
		// emit an interface has still broken whatever the assertion was
		// protecting.
		parse(t).AssertNoDecl(t, "Absent")

		s := probe(t)
		parse(t).AssertNoDecl(s, "User")
		assertReports(t, s, "User", "interface_declaration", "must not")
	})
}

func TestPropertyAssertions(t *testing.T) {
	t.Parallel()

	t.Run("a property is found with its type", func(t *testing.T) {
		t.Parallel()
		parse(t).
			AssertProperty(t, "User", "id", "ID").
			AssertProperty(t, "User", "tags", "string[]").
			AssertProperty(t, "User", "owner", "Person | null").
			AssertProperty(t, "Repo", "cache", "Record<string, User>")
	})

	t.Run("spacing does not decide the answer", func(t *testing.T) {
		t.Parallel()
		// A comparison against a written-out type should survive
		// however the renderer spaced it.
		parse(t).AssertProperty(t, "User", "owner", "Person  |  null")
	})

	t.Run("an empty type asserts only that the property is there", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertProperty(t, "User", "nick", "")
	})

	t.Run("a wrong type is reported as the type it is", func(t *testing.T) {
		t.Parallel()
		// Which is what is true most of the time: the property is
		// there, holding something else.
		s := probe(t)
		parse(t).AssertProperty(s, "User", "id", "string")
		assertReports(t, s, "User.id", `"ID"`, `want "string"`)
	})

	t.Run("an absent property is reported with the ones that exist", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertProperty(s, "User", "absent", "string")
		assertReports(t, s, "User.absent", "id: ID", "nick: string")
	})

	t.Run("a property on an absent declaration is reported", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertProperty(s, "Absent", "id", "string")
		assertReports(t, s, "Absent")
	})

	t.Run("a property that must not be there is reported", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertNoProperty(t, "User", "password")

		s := probe(t)
		parse(t).AssertNoProperty(s, "User", "id")
		assertReports(t, s, "User.id", "must not")
	})

	t.Run("a declaration with no body is reported as such", func(t *testing.T) {
		t.Parallel()
		// An alias has none, and "no property" would send the reader
		// looking for a member that was never possible.
		s := probe(t)
		parse(t).AssertProperty(s, "ID", "x", "string")
		assertReports(t, s, "ID", "no body")
	})
}

func TestMethodAssertions(t *testing.T) {
	t.Parallel()

	t.Run("a method is found on an interface and on a class", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertMethod(t, "User", "greet").AssertMethod(t, "Repo", "find")
	})

	t.Run("an absent method is reported with the ones that exist", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertMethod(s, "User", "absent")
		assertReports(t, s, "User.absent", "greet")
	})

	t.Run("a whole signature is pinned", func(t *testing.T) {
		t.Parallel()
		// The assertion a dropped rest marker fails: a double taking
		// one value where the contract takes many still declares the
		// method.
		parse(t).
			AssertSignature(t, "User", "greet", "greet(loud: boolean): string").
			AssertSignature(t, "Repo", "find", "find(id: string, ...extra: string[]): User")
	})

	t.Run("a changed signature is reported as the one it is", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertSignature(s, "Repo", "find", "find(id: string): User")
		assertReports(t, s, "Repo.find", "...extra")
	})

	t.Run("a signature on an absent method is reported", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertSignature(s, "User", "absent", "absent(): void")
		assertReports(t, s, "User.absent")
	})
}

func TestHeritageAssertions(t *testing.T) {
	t.Parallel()

	t.Run("extends and implements are told apart", func(t *testing.T) {
		t.Parallel()
		// Extending inherits members, implementing only asserts they
		// are present. A generator that emitted one where the plugin
		// meant the other produces a class whose members come from
		// nowhere.
		parse(t).AssertExtends(t, "User", "Base").AssertImplements(t, "Repo", "Store")
	})

	t.Run("the wrong clause is reported with what the declaration says", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertExtends(s, "Repo", "Store")
		assertReports(t, s, "Repo", "extends")
	})

	t.Run("heritage on an absent declaration is reported", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertExtends(s, "Absent", "Base")
		assertReports(t, s, "Absent")
	})
}

func TestEnumMemberAssertions(t *testing.T) {
	t.Parallel()

	t.Run("the whole member list is pinned", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertMembers(t, "Role", "Admin", "Guest")
	})

	t.Run("a bare member is named by its identifier", func(t *testing.T) {
		t.Parallel()
		// A member that took the implicit counter is the identifier
		// itself rather than an assignment with a name field.
		src := "export enum Level {\n  Low,\n  High = 5,\n}\n"
		typescripttest.
			Parse(t, typescripttest.TSFile("e.ts", src)).
			AssertMembers(t, "Level", "Low", "High")
	})

	t.Run("a different member set is reported with both", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertMembers(s, "Role", "Admin")
		assertReports(t, s, "Role", "Admin Guest")
	})
}

func TestKindNegatives(t *testing.T) {
	t.Parallel()

	t.Run("each kind can be asserted absent", func(t *testing.T) {
		t.Parallel()
		parse(t).
			AssertNoInterface(t, "Absent").
			AssertNoClass(t, "Absent").
			AssertNoAlias(t, "Absent").
			AssertNoEnum(t, "Absent").
			AssertNoFunction(t, "Absent").
			AssertNoBinding(t, "Absent")
	})

	t.Run("a present declaration of that kind is reported", func(t *testing.T) {
		t.Parallel()
		for name, assert := range map[string]func(testing.TB, string) *typescripttest.Source{
			"User":       parse(t).AssertNoInterface,
			"Repo":       parse(t).AssertNoClass,
			"ID":         parse(t).AssertNoAlias,
			"Role":       parse(t).AssertNoEnum,
			"createUser": parse(t).AssertNoFunction,
			"MAX":        parse(t).AssertNoBinding,
		} {
			s := probe(t)
			assert(s, name)
			assertReports(t, s, name, "must not")
		}
	})

	t.Run("the same name as another kind does not trip it", func(t *testing.T) {
		t.Parallel()
		// Kind-specific, unlike AssertNoDecl: what a generator emitting
		// a class where it used to emit an interface broke is the
		// consumer who wrote `implements`, and a name-only check would
		// pass.
		parse(t).AssertNoClass(t, "User").AssertNoInterface(t, "Repo")
	})

	t.Run("a method can be asserted absent", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertNoMethod(t, "User", "absent")

		s := probe(t)
		parse(t).AssertNoMethod(s, "User", "greet")
		assertReports(t, s, "User.greet", "must not")
	})

	t.Run("a method on an absent declaration reports the declaration", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertNoMethod(s, "Absent", "greet")
		assertReports(t, s, "Absent")
	})
}

func TestDeclAssertions(t *testing.T) {
	t.Parallel()

	t.Run("a declaration hands back the file it came from", func(t *testing.T) {
		t.Parallel()
		// So a chain that narrowed to one declaration can widen again
		// without the caller holding both.
		if got := parse(t).AssertInterface(t, "User").Source().Path(); got != "user.gen.ts" {
			t.Fatalf("Source = %q", got)
		}
	})

	t.Run("a doc comment is asserted on and against", func(t *testing.T) {
		t.Parallel()
		// A doc comment carried forward verbatim brings the source's
		// `@internal` and its reference to a symbol the generated
		// module does not export — each reads as a statement about the
		// generated declaration and is false.
		src := `// Code generated by eidos. DO NOT EDIT.

/**
 * A user of the system.
 *
 * @internal
 */
export interface User {
  id: string;
}
`
		d := typescripttest.
			Parse(t, typescripttest.TSFile("u.ts", src)).
			AssertInterface(t, "User")
		d.AssertDoc(t, "A user of the system.").AssertDocLacks(t, "@deprecated")

		missing := probe(t)
		d.AssertDoc(missing, "absent phrase")
		assertReports(t, missing, "absent phrase", "A user of the system.")

		present := probe(t)
		d.AssertDocLacks(present, "@internal")
		assertReports(t, present, "@internal", "must not")
	})

	t.Run("an undocumented declaration reads as an empty doc", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertAlias(t, "ID").AssertDoc(s, "anything")
		assertReports(t, s, "anything")
	})

	t.Run("a signature is pinned whole", func(t *testing.T) {
		t.Parallel()
		// What a consumer binds to is the whole head rather than any
		// one member, and a substring check passes against a parameter
		// list that gained one.
		parse(t).AssertFunction(t, "createUser").
			Signature(t, "function createUser(id: string): User")

		s := probe(t)
		parse(t).AssertFunction(s, "createUser").Signature(s, "function createUser(): User")
		assertReports(t, s, "createUser")
	})

	t.Run("a signature on an absent declaration is reported", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertFunction(s, "absent").Signature(s, "anything")
		assertReports(t, s, "absent")
	})
}
