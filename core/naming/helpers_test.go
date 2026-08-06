// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package naming_test

import (
	"slices"
	"strings"
	"testing"
)

// separators mirrors the separator set consumed by the splitter (the
// isSeparator predicate in words.go). It is duplicated here rather than
// exported from the package on purpose: a test that read the production
// predicate could not notice a change to it, and the separator set is
// part of the package's promise to its callers.
const separators = "_-. \t/"

// stripSeparators is the naive reference for the content Words must
// conserve — every rune of s that is not a separator, in input order.
//
// It knows nothing about case boundaries, which is the point: it can
// agree with the splitter about *what* survives while staying wholly
// independent of *where* the splitter decides words begin. Both this
// function and the splitter iterate runes, so both map an invalid UTF-8
// byte to U+FFFD identically; the comparison is therefore about content
// conservation, not about encoding handling.
func stripSeparators(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if strings.ContainsRune(separators, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// asciiLettersAndSeparators reports whether s is drawn solely from ASCII
// letters and separator runes.
//
// This is the domain over which every style converter is strictly
// idempotent; outside it two documented defects break the property (see
// FuzzCaser_Idempotence for both, with counterexamples). Exhaustive
// enumeration of this domain to length 3 over all 52 ASCII letters plus
// every separator, and to length 4 over an initialism-heavy subset,
// shows zero violations for all eight styles.
func asciiLettersAndSeparators(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case strings.ContainsRune(separators, r):
		default:
			return false
		}
	}
	return true
}

// assertEqualSlices fails the test if got and want differ in length or
// any element. The diagnostic message identifies the first divergence.
func assertEqualSlices(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("slice mismatch:\n got:  %#v\n want: %#v", got, want)
	}
}

// assertEqualString is the string analogue, kept symmetrical with
// assertEqualSlices for readability.
func assertEqualString(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("string mismatch:\n got:  %q\n want: %q", got, want)
	}
}
