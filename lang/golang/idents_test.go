// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
)

// TestIsKeyword pins Go's reserved-word set.
//
// A generator deriving an identifier from a source name reaches one
// eventually — a proto field called `type`, a column called
// `range` — and the result does not parse.
func TestIsKeyword(t *testing.T) {
	t.Parallel()

	t.Run("recognises every reserved word", func(t *testing.T) {
		t.Parallel()
		for _, kw := range []string{
			"break", "case", "chan", "const", "continue", "default", "defer",
			"else", "fallthrough", "for", "func", "go", "goto", "if", "import",
			"interface", "map", "package", "range", "return", "select",
			"struct", "switch", "type", "var",
		} {
			if !golang.IsKeyword(kw) {
				t.Errorf("IsKeyword(%q) = false", kw)
			}
		}
	})

	t.Run("does not claim a predeclared identifier", func(t *testing.T) {
		t.Parallel()
		// The two sets differ in consequence: shadowing `len` compiles,
		// naming something `for` does not parse.
		for _, s := range []string{"len", "error", "string", "nil"} {
			if golang.IsKeyword(s) {
				t.Errorf("IsKeyword(%q) = true; that is predeclared, not reserved", s)
			}
		}
	})

	t.Run("does not claim an ordinary identifier", func(t *testing.T) {
		t.Parallel()
		for _, s := range []string{"user", "Type", "ranger", ""} {
			if golang.IsKeyword(s) {
				t.Errorf("IsKeyword(%q) = true", s)
			}
		}
	})
}

func TestIsPredeclared(t *testing.T) {
	t.Parallel()

	t.Run("recognises types, constants and builtins", func(t *testing.T) {
		t.Parallel()
		for _, s := range []string{
			"any", "bool", "byte", "comparable", "error", "int", "rune", "string",
			"uintptr", "true", "false", "iota", "nil",
			"append", "cap", "clear", "close", "copy", "delete", "len", "make",
			"max", "min", "new", "panic", "recover",
		} {
			if !golang.IsPredeclared(s) {
				t.Errorf("IsPredeclared(%q) = false", s)
			}
		}
	})

	t.Run("does not claim a reserved word", func(t *testing.T) {
		t.Parallel()
		if golang.IsPredeclared("func") {
			t.Fatalf("IsPredeclared(func) = true; that is reserved, not predeclared")
		}
	})
}

// TestSafeIdent pins the layer core/naming explicitly defers here.
//
// naming.Identifier sanitises runes and says reserved words are
// "not handled here — callers layer it on top, typically inside a
// language-specific helper".
func TestSafeIdent(t *testing.T) {
	t.Parallel()

	t.Run("suffixes a reserved word", func(t *testing.T) {
		t.Parallel()
		if got := golang.SafeIdent("type"); got != "type_" {
			t.Fatalf("SafeIdent(type) = %q, want type_", got)
		}
	})

	t.Run("suffixes a predeclared identifier", func(t *testing.T) {
		t.Parallel()
		// Legal Go that compiles and then breaks the next line wanting
		// the builtin — a type error somewhere else in the body, which
		// is a poor place to learn a field was called `len`.
		if got := golang.SafeIdent("len"); got != "len_" {
			t.Fatalf("SafeIdent(len) = %q, want len_", got)
		}
	})

	t.Run("leaves an ordinary identifier unchanged", func(t *testing.T) {
		t.Parallel()
		// Renaming beyond necessity breaks the correspondence a reader
		// relies on between generated and source names.
		if got := golang.SafeIdent("userID"); got != "userID" {
			t.Fatalf("SafeIdent(userID) = %q, want unchanged", got)
		}
	})

	t.Run("sanitises invalid runes before checking", func(t *testing.T) {
		t.Parallel()
		if got := golang.SafeIdent("user-id"); got != "user_id" {
			t.Fatalf("SafeIdent(user-id) = %q, want user_id", got)
		}
	})

	t.Run("a name sanitising to a keyword is still suffixed", func(t *testing.T) {
		t.Parallel()
		// The check runs after sanitisation, so a source name that only
		// becomes reserved once its punctuation is stripped is caught.
		if got := golang.SafeIdent("type"); golang.IsKeyword(got) {
			t.Fatalf("SafeIdent returned the keyword %q", got)
		}
	})

	t.Run("the suffixed form is never itself reserved", func(t *testing.T) {
		t.Parallel()
		// An underscore suffix is the one adjustment that cannot
		// collide with a keyword, since no Go keyword ends in one.
		for kw := range map[string]bool{"type": true, "range": true, "func": true} {
			if got := golang.SafeIdent(kw); golang.IsKeyword(got) || golang.IsPredeclared(got) {
				t.Errorf("SafeIdent(%q) = %q, still reserved", kw, got)
			}
		}
	})
}

func TestUniqueIdent(t *testing.T) {
	t.Parallel()

	t.Run("returns the name when nothing collides", func(t *testing.T) {
		t.Parallel()
		if got := golang.UniqueIdent("id", "ctx"); got != "id" {
			t.Fatalf("UniqueIdent = %q, want id", got)
		}
	})

	t.Run("suffixes a digit on collision", func(t *testing.T) {
		t.Parallel()
		if got := golang.UniqueIdent("id", "id"); got != "id2" {
			t.Fatalf("UniqueIdent = %q, want id2", got)
		}
	})

	t.Run("keeps counting past repeated collisions", func(t *testing.T) {
		t.Parallel()
		// A digit rather than another underscore: `x_`, `x__`, `x___`
		// are indistinguishable at a glance in a failure message.
		if got := golang.UniqueIdent("id", "id", "id2", "id3"); got != "id4" {
			t.Fatalf("UniqueIdent = %q, want id4", got)
		}
	})

	t.Run("a reserved base is made safe before deduplication", func(t *testing.T) {
		t.Parallel()
		if got := golang.UniqueIdent("type", "type_"); got != "type_2" {
			t.Fatalf("UniqueIdent = %q, want type_2", got)
		}
	})

	t.Run("never returns a reserved name", func(t *testing.T) {
		t.Parallel()
		// A caller must not be handed a name that is safe from the
		// scope but not from the language.
		got := golang.UniqueIdent("len")
		if golang.IsKeyword(got) || golang.IsPredeclared(got) {
			t.Fatalf("UniqueIdent returned the reserved %q", got)
		}
	})
}

// TestReceiverIdent pins the identifier generated methods bind their
// receiver to.
//
// A receiver shadowed by a parameter of the same letter is a compile
// error in generated code, and the source has no say in which letter
// either takes.
func TestReceiverIdent(t *testing.T) {
	t.Parallel()

	t.Run("takes the type's first letter, lower-cased", func(t *testing.T) {
		t.Parallel()
		if got := golang.ReceiverIdent("Store"); got != "s" {
			t.Fatalf("ReceiverIdent(Store) = %q, want s", got)
		}
	})

	t.Run("avoids a colliding parameter name", func(t *testing.T) {
		t.Parallel()
		if got := golang.ReceiverIdent("Store", "s"); got != "s2" {
			t.Fatalf("ReceiverIdent = %q, want s2", got)
		}
	})

	t.Run("skips a leading non-letter", func(t *testing.T) {
		t.Parallel()
		if got := golang.ReceiverIdent("_Store"); got != "s" {
			t.Fatalf("ReceiverIdent(_Store) = %q, want s", got)
		}
	})

	t.Run("falls back for a name with no letters", func(t *testing.T) {
		t.Parallel()
		if got := golang.ReceiverIdent("_123"); got != "r" {
			t.Fatalf("ReceiverIdent(_123) = %q, want r", got)
		}
	})

	t.Run("falls back for an empty name", func(t *testing.T) {
		t.Parallel()
		if got := golang.ReceiverIdent(""); got != "r" {
			t.Fatalf("ReceiverIdent(\"\") = %q, want r", got)
		}
	})

	t.Run("never returns a reserved identifier", func(t *testing.T) {
		t.Parallel()
		// `Var`, `Func`, `Map` are ordinary Go type names whose initial
		// is harmless — but the guard has to hold for any input.
		for _, name := range []string{"Var", "Func", "Interface", "Chan"} {
			got := golang.ReceiverIdent(name)
			if golang.IsKeyword(got) || golang.IsPredeclared(got) {
				t.Errorf("ReceiverIdent(%q) = %q, which is reserved", name, got)
			}
		}
	})
}

// TestPackageName pins the import-path-to-clause rule, including
// the major-version suffix everyone gets wrong.
func TestPackageName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path, want string
	}{
		{"example.com/foo", "foo"},
		{"foo", "foo"},
		{"example.com/foo/bar", "bar"},
		{"example.com/foo/v2", "foo"},
		{"example.com/foo/v10", "foo"},
		{"example.com/foo/v1", "v1"},
		{"example.com/foo/v0", "v0"},
		{"example.com/foo/vendor", "vendor"},
		{"example.com/foo/", "foo"},
		{"example.com/my-pkg", "my_pkg"},
		{"v2", "v2"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.path+" → "+tc.want, func(t *testing.T) {
			t.Parallel()
			if got := golang.PackageName(tc.path); got != tc.want {
				t.Fatalf("PackageName(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestIsInternal pins Go's internal-package reachability rule.
//
// A generator emitting a reference into another package's internal
// tree produces output that compiles where it was generated and
// fails for anyone who imports it.
func TestIsInternal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want bool
	}{
		{"example.com/foo/internal/bar", true},
		{"internal/bar", true},
		{"example.com/internal", true},
		{"example.com/foo/bar", false},
		{"example.com/internalise", false},
		{"example.com/myinternal", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			if got := golang.IsInternal(tc.path); got != tc.want {
				t.Fatalf("IsInternal(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestReceiverIdent_LowerCaseType(t *testing.T) {
	t.Parallel()

	t.Run("takes the initial of an unexported type", func(t *testing.T) {
		t.Parallel()
		// A generator emitting a method on a package-private type gets
		// the same convention as an exported one.
		if got := golang.ReceiverIdent("store"); got != "s" {
			t.Fatalf("ReceiverIdent(store) = %q, want s", got)
		}
	})
}
