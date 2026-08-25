// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go/ast"
	"strings"
)

// docLinesFromCommentGroup extracts the doc-comment content from a
// [go/ast.CommentGroup] in the line-oriented form
// [node.BaseNode.DocLines] expects — one entry per logical line,
// comment markers stripped, trailing whitespace preserved when it
// reflects intentional formatting (e.g. code blocks).
//
// `//` comments contribute one entry per `//` line. Block comments
// (`/* … */`) split on newlines; the opening `/*` and closing `*/`
// markers are stripped, and a single optional leading space after
// `//` (or after `* ` for continuation lines) is collapsed to match
// the Go convention.
func docLinesFromCommentGroup(g *ast.CommentGroup) []string {
	if g == nil {
		return nil
	}
	// go/parser guarantees g.List has at least one entry; the
	// resulting slice is non-empty whenever the input was non-nil.
	var out []string
	for _, c := range g.List {
		out = append(out, splitCommentLines(c.Text)...)
	}
	return out
}

// splitCommentLines returns the line-oriented content of a single
// [go/ast.Comment.Text]. Handles both `//`-style and `/* … */`-style
// comments; trailing newlines and leading comment markers are
// stripped. go/parser guarantees every Comment.Text starts with one
// of those two markers.
func splitCommentLines(text string) []string {
	if strings.HasPrefix(text, "//") {
		return []string{stripLineMarker(text)}
	}
	body := strings.TrimSuffix(strings.TrimPrefix(text, "/*"), "*/")
	return splitBlockBody(body)
}

// stripLineMarker removes the leading `//` (and optionally a single
// following space) from a `//`-style comment text.
func stripLineMarker(text string) string {
	body := strings.TrimPrefix(text, "//")
	return strings.TrimPrefix(body, " ")
}

// splitBlockBody splits a block-comment body into doc-line entries.
// Continuation lines that start with `* ` (the typical multi-line
// comment indentation) have that prefix stripped.
func splitBlockBody(body string) []string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		trim := strings.TrimLeft(l, " \t")
		switch {
		case strings.HasPrefix(trim, "* "):
			lines[i] = strings.TrimPrefix(trim, "* ")
		case trim == "*":
			lines[i] = ""
		default:
			lines[i] = strings.TrimSpace(l)
		}
	}
	return trimEmptyTail(lines)
}

// trimEmptyTail drops trailing empty entries from lines. Block
// comments commonly include a closing-marker blank line that would
// otherwise leak into the doc model.
func trimEmptyTail(lines []string) []string {
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
