// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package naming

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// ErrInvalidInitialism is returned by [Caser.WithInitialisms] when a
// candidate initialism does not satisfy the contract: non-empty,
// composed of ASCII upper-case letters or digits, and starting with a
// letter (e.g. "URL", "UTF8", "HTTP2"). Lower-case letters and any
// other rune are rejected.
var ErrInvalidInitialism = errors.New(
	"naming: initialism must be non-empty, start with an upper-case ASCII letter, and contain only upper-case letters and digits",
)

// CommonInitialisms is the canonical list of initialisms recognised by
// [Default]. It mirrors the Go style guide and the staticcheck stylecheck
// linter so identifiers round-trip predictably (e.g. "url_path" →
// Pascal → "URLPath", not "UrlPath").
//
// Consumers that need a different set construct a custom Caser via
// [New] + [Caser.WithInitialisms].
var CommonInitialisms = []string{
	"ACL", "API", "ASCII", "CPU", "CSS", "DNS", "EOF", "GUID",
	"HTML", "HTTP", "HTTPS", "ID", "IP", "JSON", "LHS", "QPS",
	"RAM", "RHS", "RPC", "SLA", "SMTP", "SQL", "SSH", "TCP",
	"TLS", "TTL", "UDP", "UI", "UID", "UUID", "URI", "URL", "UTF8",
	"VM", "XML", "XMPP", "XSRF", "XSS",
}

// Caser holds case-conversion configuration, primarily the set of
// recognised initialisms. The zero value is unusable; construct with
// [Default] or [New].
//
// Caser values are immutable once constructed. [Caser.WithInitialisms]
// returns a new Caser, leaving the receiver unchanged. This makes it
// safe to share a Caser across goroutines without locking.
//
// # Idempotence
//
// Every style is idempotent over identifiers built from ASCII letters
// and the recognised separators: converting an already-converted name
// returns it unchanged. That covers the input a frontend derives from
// source identifiers, which is what these functions exist for.
//
// Outside that domain a first application can move a word boundary
// that the second then reads differently, so f(f(x)) may differ from
// f(x). Every style reaches a fixed point by the second application —
// f(f(f(x))) always equals f(f(x)) — so the divergence is bounded at
// one step rather than oscillating. Three worked cases:
//
//	Pascal("aA1")        -> "AA1" -> "Aa1"
//	Pascal("aÉ")         -> "AÉ"  -> "Aé"
//	ScreamingSnake("ßa") -> "ßA"   -> "ß_A"
//
// In the first, splitting "aA1" yields the words "a" and "A1", which
// Pascal renders "AA1"; re-splitting that reads a leading acronym run
// instead. The others turn on runes whose case mapping changes which
// side of a boundary they fall.
//
// This is documented rather than repaired because the repair is not
// backwards compatible: these functions name generated identifiers, so
// changing their output renames symbols in consumers' generated code.
// Callers converting non-ASCII input, or feeding already-converted
// names back through a style, should convert once from the original
// source name. [FuzzCaser_Idempotence] pins both halves of this
// contract.
type Caser struct {
	initialisms map[string]struct{}
}

// Default returns a Caser pre-loaded with [CommonInitialisms]. The same
// pointer is returned on every call; callers must not assume otherwise
// but the value is safe to share.
func Default() *Caser { return defaultCaser }

// New returns a fresh Caser with no initialisms recognised. Words are
// split and converted purely by case and separator transitions; "url"
// becomes "Url" rather than "URL" under Pascal.
func New() *Caser {
	return &Caser{initialisms: map[string]struct{}{}}
}

// WithInitialisms returns a new Caser that recognises the given
// initialisms in addition to whatever the receiver already recognised.
//
// Each initialism must be non-empty and consist solely of upper-case
// ASCII letters; otherwise [ErrInvalidInitialism] is returned wrapping
// the offending value.
func (c *Caser) WithInitialisms(words ...string) (*Caser, error) {
	out := &Caser{initialisms: maps.Clone(c.initialisms)}
	if out.initialisms == nil {
		out.initialisms = map[string]struct{}{}
	}
	for _, w := range words {
		if !isValidInitialism(w) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidInitialism, w)
		}
		out.initialisms[w] = struct{}{}
	}
	return out, nil
}

// Initialisms returns the recognised initialisms in alphabetical order.
// The returned slice is a fresh copy; callers may modify it freely.
func (c *Caser) Initialisms() []string {
	out := slices.Collect(maps.Keys(c.initialisms))
	slices.Sort(out)
	return out
}

// isInitialism reports whether the upper-case form of w is registered.
// w is expected to already be upper-case; the caller normalises.
func (c *Caser) isInitialism(upperW string) bool {
	_, ok := c.initialisms[upperW]
	return ok
}

// isAllUpperASCII reports whether every byte of s is an upper-case
// ASCII letter. The caller guarantees s is non-empty; an empty input
// is undefined.
func isAllUpperASCII(s string) bool {
	for i := range len(s) {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return true
}

// isValidInitialism reports whether s is a valid initialism: non-empty,
// composed of ASCII upper-case letters or digits, with the first byte
// being a letter (so "8K" or "8" are rejected; "UTF8" and "HTTP2" are
// accepted).
func isValidInitialism(s string) bool {
	if s == "" {
		return false
	}
	if s[0] < 'A' || s[0] > 'Z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return true
}

// defaultCaser is the singleton returned by Default. It is built without
// re-validating CommonInitialisms because that list is hard-coded and a
// dedicated test (TestCommonInitialisms_AreValid) asserts each entry
// passes the same validation that WithInitialisms applies.
var defaultCaser = withInitialismsUnchecked(CommonInitialisms)

// withInitialismsUnchecked is the validation-skipping fast path for
// constructing a Caser from a list of trusted initialisms. Used only
// for [defaultCaser] construction; external callers go through
// [Caser.WithInitialisms].
func withInitialismsUnchecked(words []string) *Caser {
	out := &Caser{initialisms: make(map[string]struct{}, len(words))}
	for _, w := range words {
		out.initialisms[w] = struct{}{}
	}
	return out
}

// titleWord returns w with the first rune upper-cased and the rest
// lower-cased, except that an all-upper input is left untouched and
// any word whose upper-cased form is a recognised initialism is
// upper-cased in full. The caller guarantees w is non-empty.
//
// This is the building block of [Caser.Pascal] and [Caser.Camel]
// (for non-leading words).
func (c *Caser) titleWord(w string) string {
	upper := strings.ToUpper(w)
	if c.isInitialism(upper) {
		return upper
	}
	if isAllUpperASCII(w) {
		return w
	}
	runes := []rune(w)
	runes[0] = upperRune(runes[0])
	for i := 1; i < len(runes); i++ {
		runes[i] = lowerRune(runes[i])
	}
	return string(runes)
}
