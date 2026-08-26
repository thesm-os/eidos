// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
)

func TestIsReserved(t *testing.T) {
	t.Parallel()

	t.Run("covers all three reserved groups", func(t *testing.T) {
		t.Parallel()
		// Always-reserved, strict-mode-reserved, and TypeScript's
		// type-position keywords. A generated declaration cannot know
		// which context it lands in, so all three are refused.
		for _, name := range []string{"class", "return", "interface", "static", "string", "never"} {
			if !typescript.IsReserved(name) {
				t.Errorf("%q not reported as reserved", name)
			}
		}
	})

	t.Run("a contextual keyword binds safely", func(t *testing.T) {
		t.Parallel()
		// TypeScript resolves these by position; renaming them would
		// make identifiers ugly for nothing.
		for _, name := range []string{"as", "from", "of", "get", "set", "async", "user"} {
			if typescript.IsReserved(name) {
				t.Errorf("%q refused, but it binds safely", name)
			}
		}
	})
}

func TestIsValidIdent(t *testing.T) {
	t.Parallel()

	t.Run("accepts the identifier grammar", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"a", "_x", "$x", "aB9", "café", "_"} {
			if !typescript.IsValidIdent(name) {
				t.Errorf("%q rejected", name)
			}
		}
	})

	t.Run("rejects what the grammar does not admit", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"", "9a", "a-b", "a b", "a.b", "a!"} {
			if typescript.IsValidIdent(name) {
				t.Errorf("%q accepted", name)
			}
		}
	})

	t.Run("a reserved word is well-formed but unbindable", func(t *testing.T) {
		t.Parallel()
		// Two different questions: `class` is a valid identifier that
		// happens to be reserved.
		if !typescript.IsValidIdent("class") {
			t.Error("a reserved word reported as malformed")
		}
	})
}

func TestIdent(t *testing.T) {
	t.Parallel()

	t.Run("passes a usable name through", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"user", "_id", "$ref", "camelCase"} {
			if got := typescript.Ident(name); got != name {
				t.Errorf("Ident(%q) = %q, want it unchanged", name, got)
			}
		}
	})

	t.Run("replaces runes the grammar refuses", func(t *testing.T) {
		t.Parallel()
		for name, want := range map[string]string{
			"a-b":          "a_b",
			"a b":          "a_b",
			"content.type": "content_type",
		} {
			if got := typescript.Ident(name); got != want {
				t.Errorf("Ident(%q) = %q, want %q", name, got, want)
			}
		}
	})

	t.Run("prefixes a leading digit", func(t *testing.T) {
		t.Parallel()
		if got := typescript.Ident("2fast"); got != "_2fast" {
			t.Fatalf("Ident(2fast) = %q, want _2fast", got)
		}
	})

	t.Run("suffixes a reserved word", func(t *testing.T) {
		t.Parallel()
		if got := typescript.Ident("class"); got != "class_" {
			t.Fatalf("Ident(class) = %q, want class_", got)
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		t.Parallel()
		// The trailing underscore survives a second pass because
		// `class_` is not itself reserved. A leading one would read as
		// a deliberate privacy marker, which is a different claim.
		for _, name := range []string{"class", "a-b", "2fast", "", "user"} {
			once := typescript.Ident(name)
			if twice := typescript.Ident(once); twice != once {
				t.Errorf("Ident(%q) = %q then %q; not idempotent", name, once, twice)
			}
		}
	})

	t.Run("empty input yields an underscore", func(t *testing.T) {
		t.Parallel()
		if got := typescript.Ident(""); got != "_" {
			t.Fatalf("Ident(empty) = %q, want _", got)
		}
	})
}

func TestPropertyKey(t *testing.T) {
	t.Parallel()

	t.Run("a well-formed name renders bare", func(t *testing.T) {
		t.Parallel()
		if got := typescript.PropertyKey("userId"); got != "userId" {
			t.Fatalf("PropertyKey = %q", got)
		}
	})

	t.Run("anything else is quoted", func(t *testing.T) {
		t.Parallel()
		for name, want := range map[string]string{
			"content-type": "'content-type'",
			"2xx":          "'2xx'",
			"has space":    "'has space'",
		} {
			if got := typescript.PropertyKey(name); got != want {
				t.Errorf("PropertyKey(%q) = %q, want %q", name, got, want)
			}
		}
	})

	t.Run("a reserved word renders bare", func(t *testing.T) {
		t.Parallel()
		// Property position is not binding position: `{ class: string }`
		// is valid TypeScript, and quoting would change the key's
		// spelling — the one thing a property key may not do.
		if got := typescript.PropertyKey("class"); got != "class" {
			t.Fatalf("PropertyKey(class) = %q, want it bare", got)
		}
	})
}

func TestUniqueIdent(t *testing.T) {
	t.Parallel()

	t.Run("an unclaimed name is returned as given", func(t *testing.T) {
		t.Parallel()
		if got := typescript.UniqueIdent("value", "other"); got != "value" {
			t.Fatalf("UniqueIdent = %q, want value", got)
		}
	})

	t.Run("a claimed name is numbered from two", func(t *testing.T) {
		t.Parallel()
		// `x1` would suggest an `x0` that does not exist.
		if got := typescript.UniqueIdent("x", "x"); got != "x2" {
			t.Fatalf("UniqueIdent = %q, want x2", got)
		}
		if got := typescript.UniqueIdent("x", "x", "x2"); got != "x3" {
			t.Fatalf("UniqueIdent = %q, want x3", got)
		}
	})

	t.Run("a reserved word is made bindable before it is numbered", func(t *testing.T) {
		t.Parallel()
		if got := typescript.UniqueIdent("class"); got != "class_" {
			t.Fatalf("UniqueIdent = %q, want class_", got)
		}
	})

	t.Run("an unwriteable name still yields one", func(t *testing.T) {
		t.Parallel()
		if got := typescript.UniqueIdent(""); got != "_" {
			t.Fatalf("UniqueIdent(\"\") = %q, want _", got)
		}
	})
}
