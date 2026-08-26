// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// quoteRune is the quote character every string this package emits is
// wrapped in.
//
// Single, because that is where the ecosystem's formatters land: the
// TypeScript codebases that configure Prettier at all overwhelmingly
// set singleQuote, and Biome ships single as its default. The backend
// runs neither, so this constant is the whole decision — and it has
// to be one value for output to be byte-identical run to run.
const quoteRune = '\''

// Quote renders s as a TypeScript string literal, single-quoted, with
// the minimum escaping the grammar requires.
//
// Not [strconv.Quote]: Go's quoting escapes every non-ASCII rune to
// \u form, which turns `café` into `café` and makes generated
// output unreadable for anyone whose identifiers are not ASCII.
// TypeScript source is UTF-8 and a printable non-ASCII rune is legal
// in a literal, so it passes through.
func Quote(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteRune(quoteRune)
	for _, r := range s {
		switch r {
		case quoteRune:
			b.WriteString(`\'`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case 0:
			// `\0` is a legal escape only where the next character is
			// not a digit — `'\0' + '1'` parses as the deprecated
			// octal escape. `\x00` has no such adjacency rule.
			b.WriteString(`\x00`)
		default:
			b.WriteString(escapeRune(r))
		}
	}
	b.WriteRune(quoteRune)
	return b.String()
}

// escapeRune renders one rune with no dedicated escape: verbatim when
// printable, \u form when it is a control or format character, and
// the replacement character when the input held invalid UTF-8.
func escapeRune(r rune) string {
	switch {
	case r == utf8.RuneError:
		return string(utf8.RuneError)
	case unicode.IsPrint(r):
		return string(r)
	case r <= 0xFFFF:
		return `\u` + pad4(strconv.FormatInt(int64(r), 16))
	default:
		return `\u{` + strconv.FormatInt(int64(r), 16) + `}`
	}
}

// pad4 left-pads a hex string to the four digits \uXXXX requires.
func pad4(s string) string {
	if len(s) >= 4 {
		return s
	}
	return strings.Repeat("0", 4-len(s)) + s
}

// Bool renders a boolean literal.
func Bool(v bool) string {
	if v {
		return LiteralTrue
	}
	return LiteralFalse
}

// StringUnion renders values as a union of string-literal types —
// `'a' | 'b'` — which is how TypeScript spells a closed set of
// strings, and therefore how an enum of strings renders when a
// consumer wants a type rather than a runtime object.
//
// Empty input yields [TypeNever]. Returning the empty string would
// produce `type Empty = ;`, which does not parse.
func StringUnion(values []string) string {
	if len(values) == 0 {
		return TypeNever
	}
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, Quote(v))
	}
	return strings.Join(parts, " | ")
}
