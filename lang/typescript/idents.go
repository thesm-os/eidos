// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript

import (
	"strconv"
	"strings"
	"unicode"
)

// reserved is every identifier a generated declaration may not bind.
//
// Three groups, folded into one set because the code choosing a name
// cannot know which context it will land in:
//
//   - Reserved everywhere (ECMAScript §12.7.2).
//   - Reserved under strict mode, which every ES module is — so these
//     bind no more safely than the first group.
//   - TypeScript's type-position keywords. `string` is a legal value
//     binding, but a generated `const string = 1` shadows the type
//     for the rest of the file and the error surfaces at a use site
//     nothing generated.
//
// Contextual keywords that bind safely — `as`, `from`, `of`, `get`,
// `set`, `async` — are deliberately absent. Renaming them would make
// identifiers ugly for nothing; TypeScript resolves them by position.
var reserved = map[string]struct{}{ //nolint:gochecknoglobals // immutable lookup table
	"break": {}, "case": {}, "catch": {}, "class": {}, "const": {},
	"continue": {}, "debugger": {}, "default": {}, "delete": {}, "do": {},
	"else": {}, "enum": {}, "export": {}, "extends": {}, "false": {},
	"finally": {}, "for": {}, "function": {}, "if": {}, "import": {},
	"in": {}, "instanceof": {}, "new": {}, "null": {}, "return": {},
	"super": {}, "switch": {}, "this": {}, "throw": {}, "true": {},
	"try": {}, "typeof": {}, "var": {}, "void": {}, "while": {}, "with": {},

	"implements": {}, "interface": {}, "let": {}, "package": {},
	"private": {}, "protected": {}, "public": {}, "static": {}, "yield": {},

	"any": {}, "bigint": {}, "boolean": {}, "never": {}, "number": {},
	"object": {}, "string": {}, "symbol": {}, "undefined": {}, "unknown": {},
}

// IsReserved reports whether name is an identifier a generated
// declaration must not bind.
func IsReserved(name string) bool {
	_, ok := reserved[name]
	return ok
}

// IsValidIdent reports whether name is a well-formed TypeScript
// identifier: a leading letter, `$` or `_`, then letters, digits, `$`
// or `_`.
//
// Unicode letters and digits are accepted, matching the language —
// TypeScript follows UAX #31's ID_Start and ID_Continue, which the
// unicode predicates approximate closely enough for names derived
// from source.
//
// Being reserved does not make a name invalid: `class` is well-formed
// and merely unbindable. [Ident] combines both checks.
func IsValidIdent(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 && !identStart(r) {
			return false
		}
		if i > 0 && !identPart(r) {
			return false
		}
	}
	return true
}

// identStart reports whether r may open an identifier.
func identStart(r rune) bool {
	return r == '$' || r == '_' || unicode.IsLetter(r)
}

// identPart reports whether r may continue one.
func identPart(r rune) bool {
	return identStart(r) || unicode.IsDigit(r)
}

// Ident sanitises s into an identifier a declaration can bind:
// invalid runes become `_`, a leading digit gains a `_` prefix, and a
// reserved word gains a trailing `_`.
//
// Trailing rather than leading, for two reasons. It is what
// TypeScript codebases already write for this collision (`class_`),
// and it survives a second pass unchanged, since `class_` is not
// itself reserved. A leading `_` would not: it reads as a deliberate
// privacy marker, which is a different claim than "this name
// collided".
func Ident(s string) string {
	if s == "" {
		return "_"
	}
	var b strings.Builder
	b.Grow(len(s) + 1)
	for i, r := range s {
		switch {
		case i == 0 && identStart(r):
			b.WriteRune(r)
		case i == 0 && unicode.IsDigit(r):
			b.WriteByte('_')
			b.WriteRune(r)
		case i > 0 && identPart(r):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if IsReserved(out) {
		return out + "_"
	}
	return out
}

// PropertyKey renders name as it must appear left of a property
// signature: bare when it is a well-formed identifier, quoted
// otherwise — which is what carries `content-type` or `2xx` into a
// valid object type.
//
// A reserved word renders bare on purpose. Property position is not
// binding position: `{ class: string }` is valid TypeScript, and
// quoting would change the key's spelling, which is the one thing a
// property key may not do.
func PropertyKey(name string) string {
	if IsValidIdent(name) {
		return name
	}
	return Quote(name)
}

// UniqueIdent returns [Ident] of name, adjusted until it collides
// with nothing in taken.
//
// A generated body binds several identifiers in one scope —
// parameters, a captured result, a receiver — and two of them
// sharing a name is a redeclaration TypeScript rejects. The
// positional fallback is what collides in practice: a signature
// written `(arg0: string, x: number)` names its first parameter
// exactly what the second would fall back to.
//
// Numbered from 2, so the first collision reads `x2` rather than
// `x1` — which would suggest an `x0` that does not exist.
func UniqueIdent(name string, taken ...string) string {
	base := Ident(name)
	used := make(map[string]struct{}, len(taken))
	for _, t := range taken {
		used[t] = struct{}{}
	}
	candidate := base
	for i := 2; ; i++ {
		if _, clash := used[candidate]; !clash && !IsReserved(candidate) {
			return candidate
		}
		candidate = base + strconv.Itoa(i)
	}
}
