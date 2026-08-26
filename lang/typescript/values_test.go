// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
)

func TestQuote(t *testing.T) {
	t.Parallel()

	t.Run("wraps in single quotes", func(t *testing.T) {
		t.Parallel()
		if got := typescript.Quote("plain"); got != "'plain'" {
			t.Fatalf("Quote = %q", got)
		}
	})

	t.Run("escapes what the grammar requires", func(t *testing.T) {
		t.Parallel()
		for in, want := range map[string]string{
			"it's":   `'it\'s'`,
			`a\b`:    `'a\\b'`,
			"a\nb":   `'a\nb'`,
			"a\tb":   `'a\tb'`,
			"a\rb":   `'a\rb'`,
			"a\x00b": `'a\x00b'`,
		} {
			if got := typescript.Quote(in); got != want {
				t.Errorf("Quote(%q) = %s, want %s", in, got, want)
			}
		}
	})

	t.Run("a printable non-ASCII rune passes through", func(t *testing.T) {
		t.Parallel()
		// Go's own quoting escapes these to \u form, which makes
		// generated output unreadable for anyone whose identifiers are
		// not ASCII. TypeScript source is UTF-8 and the rune is legal.
		if got := typescript.Quote("café"); got != "'café'" {
			t.Fatalf("Quote(café) = %s, want the rune verbatim", got)
		}
	})

	t.Run("a control character becomes a padded unicode escape", func(t *testing.T) {
		t.Parallel()
		// \uXXXX is fixed-width, so a low code point pads to four
		// digits — `\u1` is not a valid escape.
		if got := typescript.Quote("a\u0001b"); got != `'a\u0001b'` {
			t.Fatalf("Quote = %s, want the control character escaped and padded", got)
		}
	})

	t.Run("the result is quoted on both ends", func(t *testing.T) {
		t.Parallel()
		for _, in := range []string{"", "a", "it's", "a\nb", "café"} {
			got := typescript.Quote(in)
			if !strings.HasPrefix(got, "'") || !strings.HasSuffix(got, "'") {
				t.Errorf("Quote(%q) = %s, want it quoted on both ends", in, got)
			}
		}
	})
}

func TestBool(t *testing.T) {
	t.Parallel()

	t.Run("renders both literals", func(t *testing.T) {
		t.Parallel()
		if typescript.Bool(true) != "true" || typescript.Bool(false) != "false" {
			t.Fatal("Bool does not render the two literals")
		}
	})
}

func TestStringUnion(t *testing.T) {
	t.Parallel()

	t.Run("joins quoted members", func(t *testing.T) {
		t.Parallel()
		if got := typescript.StringUnion([]string{"a", "b", "c"}); got != "'a' | 'b' | 'c'" {
			t.Fatalf("StringUnion = %q", got)
		}
	})

	t.Run("an empty set is never", func(t *testing.T) {
		t.Parallel()
		// Returning the empty string would produce `type Empty = ;`,
		// which does not parse.
		if got := typescript.StringUnion(nil); got != typescript.TypeNever {
			t.Fatalf("StringUnion(nil) = %q, want never", got)
		}
	})

	t.Run("a member needing escapes is escaped", func(t *testing.T) {
		t.Parallel()
		if got := typescript.StringUnion([]string{"it's"}); got != `'it\'s'` {
			t.Fatalf("StringUnion = %s", got)
		}
	})
}
