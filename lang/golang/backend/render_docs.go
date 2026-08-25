// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import "strings"

// renderDocs converts a doc-comment-line slice into the canonical
// Go-comment-block text rendered above a declaration. Each line is
// prefixed with "// " unless it already begins with "//" — typically
// a compile-time directive such as "//go:embed", "//go:build", or
// "//nolint:foo" — in which case the line renders verbatim. Empty
// input returns the empty string so callers can place it directly
// above a declaration without introducing whitespace for
// undocumented entities.
//
// Generators that mix human docs and directive lines place the
// directives last so the rendered ordering is "docs first, then
// directives, then declaration" per Go convention.
//
// `renderDocs` is one of the reserved core funcmap entries — plugin
// overrides for this name are rejected at Build time.
func renderDocs(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	for _, line := range lines {
		if strings.HasPrefix(line, "//") {
			b.WriteString(line)
		} else {
			b.WriteString("// ")
			b.WriteString(line)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
