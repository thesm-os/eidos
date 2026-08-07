// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/core/naming"
)

// keywords is Go's reserved-word set.
//
// A generator deriving an identifier from a source name reaches one
// eventually — a proto field called `type`, a column called `range`,
// a JSON key called `func` — and the result does not compile. The
// set is closed by the language spec, so a literal map is the whole
// implementation.
//
//nolint:gochecknoglobals // closed language-defined set
var keywords = map[string]struct{}{
	"break": {}, "case": {}, "chan": {}, "const": {}, "continue": {},
	"default": {}, "defer": {}, "else": {}, "fallthrough": {}, "for": {},
	"func": {}, "go": {}, "goto": {}, "if": {}, "import": {},
	"interface": {}, "map": {}, "package": {}, "range": {}, "return": {},
	"select": {}, "struct": {}, "switch": {}, "type": {}, "var": {},
}

// predeclared is Go's predeclared identifier set — types, constants,
// the zero value, and the builtin functions.
//
// Shadowing one is legal Go, which is exactly why it belongs here
// rather than beside [keywords]: a parameter named `len` or `error`
// compiles and then breaks the next line that wanted the builtin.
// The failure is a type error somewhere else in the generated body,
// which is a poor place to learn that a field was called `copy`.
//
//nolint:gochecknoglobals // closed language-defined set
var predeclared = map[string]struct{}{
	// Types.
	"any": {}, "bool": {}, "byte": {}, "comparable": {}, "complex64": {},
	"complex128": {}, "error": {}, "float32": {}, "float64": {}, "int": {},
	"int8": {}, "int16": {}, "int32": {}, "int64": {}, "rune": {},
	"string": {}, "uint": {}, "uint8": {}, "uint16": {}, "uint32": {},
	"uint64": {}, "uintptr": {},
	// Constants and the zero value.
	"true": {}, "false": {}, "iota": {}, "nil": {},
	// Builtin functions.
	"append": {}, "cap": {}, "clear": {}, "close": {}, "complex": {},
	"copy": {}, "delete": {}, "imag": {}, "len": {}, "make": {},
	"max": {}, "min": {}, "new": {}, "panic": {}, "print": {},
	"println": {}, "real": {}, "recover": {},
}

// IsKeyword reports whether s is a Go reserved word.
func IsKeyword(s string) bool {
	_, ok := keywords[s]
	return ok
}

// IsPredeclared reports whether s is a Go predeclared identifier —
// a builtin type, constant, `nil`, or a builtin function.
//
// Distinct from [IsKeyword] because the consequences differ.
// Shadowing a predeclared name is legal and compiles; a keyword in
// the same position does not parse.
func IsPredeclared(s string) bool {
	_, ok := predeclared[s]
	return ok
}

// SafeIdent returns name adjusted so it is usable as a Go
// identifier in a value position — a parameter, a local, a field.
//
// [naming.Identifier] sanitises the runes and says so explicitly:
// reserved words are "not handled here — callers that need that
// behaviour layer it on top, typically inside a language-specific
// frontend or backend helper". This is that layer.
//
// A reserved or predeclared name gains a trailing underscore, which
// is the convention Go's own generators use and the one adjustment
// that cannot itself collide with a keyword. Everything else is
// returned unchanged, so an identifier that was already fine keeps
// the spelling its source gave it — renaming beyond necessity would
// break the correspondence a reader relies on.
func SafeIdent(name string) string {
	sanitised := naming.Identifier(name)
	if IsKeyword(sanitised) || IsPredeclared(sanitised) {
		return sanitised + "_"
	}
	return sanitised
}

// UniqueIdent returns SafeIdent(name) adjusted so it collides with
// nothing in taken.
//
// Suffixed with an increasing digit rather than another underscore,
// because a caller resolving several collisions in one scope would
// otherwise produce `x_`, `x__`, `x___` — indistinguishable at a
// glance in a failure message, and each one still a candidate for
// the next collision.
//
// The result is checked against the reserved sets again after
// suffixing, so a caller cannot be handed a name that is safe from
// the scope but not from the language.
func UniqueIdent(name string, taken ...string) string {
	base := SafeIdent(name)
	used := make(map[string]struct{}, len(taken))
	for _, t := range taken {
		used[t] = struct{}{}
	}
	candidate := base
	for i := 2; ; i++ {
		_, clash := used[candidate]
		if !clash && !IsKeyword(candidate) && !IsPredeclared(candidate) {
			return candidate
		}
		candidate = base + strconv.Itoa(i)
	}
}

// ReceiverIdent returns the identifier a generated method binds its
// receiver to, avoiding anything in taken.
//
// Go's convention is a short lower-case abbreviation of the type
// name rather than a word like `self` or `this`, and the first
// letter is what the standard library uses almost everywhere. The
// parameters a method declares are the collision risk worth passing
// in: a receiver shadowed by a parameter of the same letter is a
// compile error in generated code, and the source has no say in
// which letter either takes.
func ReceiverIdent(typeName string, taken ...string) string {
	initial := ""
	for _, r := range typeName {
		if r >= 'A' && r <= 'Z' {
			initial = strings.ToLower(string(r))
			break
		}
		if r >= 'a' && r <= 'z' {
			initial = string(r)
			break
		}
	}
	if initial == "" {
		initial = "r"
	}
	return UniqueIdent(initial, taken...)
}

// PackageName returns the Go package clause an import path resolves
// to.
//
// The last path segment, except that a major-version suffix is not
// a package name: `example.com/foo/v2` is package `foo`, and taking
// the trailing segment yields `v2` — a clause that compiles and
// names the wrong thing in every reference to it. Digits-after-v is
// the whole rule; a segment genuinely called `v2` is
// indistinguishable and correctly loses, since the module convention
// is what a reader assumes.
//
// Empty input returns empty; the caller decides the fallback,
// because a package with no derivable name is a routing question
// rather than a naming one.
func PackageName(importPath string) string {
	path := strings.TrimSuffix(importPath, "/")
	if path == "" {
		return ""
	}
	segments := strings.Split(path, "/")
	last := segments[len(segments)-1]
	if len(segments) > 1 && isMajorVersion(last) {
		last = segments[len(segments)-2]
	}
	return naming.Identifier(last)
}

// isMajorVersion reports whether a path segment is a Go module
// major-version suffix — `v` followed by digits, `v2` upward.
//
// `v0` and `v1` are excluded because the module convention omits
// them: a path ending in `/v1` names a directory, not a version.
func isMajorVersion(segment string) bool {
	if len(segment) < 2 || segment[0] != 'v' {
		return false
	}
	n, err := strconv.Atoi(segment[1:])
	return err == nil && n >= 2
}

// IsInternal reports whether an import path is unreachable from
// outside its own subtree — Go's `internal/` rule.
//
// A generator emitting a reference into another package's internal
// tree produces output that compiles where it was generated and
// fails for anyone who imports it. Checking is cheaper than the
// report.
func IsInternal(importPath string) bool {
	for segment := range strings.SplitSeq(importPath, "/") {
		if segment == "internal" {
			return true
		}
	}
	return false
}
