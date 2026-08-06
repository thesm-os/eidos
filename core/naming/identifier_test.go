// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package naming_test

import (
	"testing"
	"unicode"

	"go.thesmos.sh/eidos/core/naming"
)

func TestIdentifier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty input becomes underscore", "", "_"},
		{"plain identifier passes through", "userID", "userID"},
		{"underscores are preserved", "user_id", "user_id"},
		{"non-letter non-digit becomes underscore", "user-id.v2", "user_id_v2"},
		{"leading digit gets underscore prefix", "2things", "_2things"},
		{"digit elsewhere is left alone", "thing2things", "thing2things"},
		{"unicode letters are preserved", "héllo_wörld", "héllo_wörld"},
		{"all-symbol input becomes all-underscore", "!!!", "___"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertEqualString(t, naming.Identifier(tc.in), tc.want)
		})
	}
}

// FuzzIdentifier drives the sanitiser over arbitrary input.
//
// Identifier is the last line of defence between untrusted source
// text — a proto field name, a directive argument, a database column —
// and a Go file that has to compile. Its contract is total: there is no
// input it may reject, so every input must produce something the Go
// parser accepts. An input that escapes it surfaces as a syntax error
// in generated code, by which point the offending source text is
// several pipeline stages away from the diagnostic.
//
// The oracle is a validator written from the language spec rather than
// a re-derivation of what the sanitiser does; see isGoIdentifierToken
// for why go/token cannot supply it here. Emptiness needs no separate
// assertion because the validator rejects the empty string.
//
// The seeds cover every branch of the switch (letter, underscore,
// digit-at-start, digit-elsewhere, replaced rune) plus the boundaries
// worth naming: empty input, a single digit, non-ASCII letters,
// non-ASCII digits, a digit-like symbol that is not a digit, a Go
// keyword, and invalid UTF-8 in leading, interior, and trailing
// position.
func FuzzIdentifier(f *testing.F) {
	for _, seed := range []string{
		"",
		"_",
		"userID",
		"user_id",
		"user-id.v2",
		"2things",
		"2",
		"thing2things",
		"héllo_wörld",
		"!!!",
		"func",
		"٣",
		"²",
		"ß",
		"\xff",
		"a\xffb",
		"a\xff",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, s string) {
		got := naming.Identifier(s)

		if !isGoIdentifierToken(got) {
			t.Fatalf("Identifier(%q) = %q, which is not a Go identifier token", s, got)
		}

		// Sanitising an already-sanitised name must change nothing.
		// Generated names are re-sanitised by every layer that handles
		// them; a second pass that mutated the name would make the
		// result depend on how many layers ran.
		if again := naming.Identifier(got); again != got {
			t.Fatalf("Identifier is not idempotent on %q: %q -> %q", s, got, again)
		}
	})
}

// isGoIdentifierToken reports whether s is a syntactically valid Go
// identifier token: a non-empty run of letters, digits, and
// underscores whose first rune is not a digit. "letter" and "digit"
// are the spec's Unicode categories, which is what unicode.IsLetter
// and unicode.IsDigit test.
//
// go/token.IsIdentifier is the obvious oracle and is deliberately not
// used: core/ is the language-agnostic layer of the framework and
// depguard forbids language-specific stdlib there, Identifier's
// Go-shaped output notwithstanding. Restating the grammar keeps the
// layering intact, and the restatement is still an independent check —
// it validates a finished string, where Identifier transforms one rune
// at a time, so none of the sanitiser's failure modes (a missed
// replacement, a digit left in the leading position, a rune index
// mistaken for a byte index) can hide in a shared assumption.
//
// Keywords pass. Identifier documents reserved words as the caller's
// concern, so "func" sanitises to "func": a valid identifier token,
// just not a usable declaration name.
func isGoIdentifierToken(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || unicode.IsLetter(r):
		case unicode.IsDigit(r) && i > 0:
		default:
			return false
		}
	}
	return true
}
