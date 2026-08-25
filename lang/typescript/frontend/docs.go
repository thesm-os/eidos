// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/node"
)

// attachDocs finds the comment block documenting n and records its
// content and any directives it carries on base.
//
// The comment is the run of `comment` siblings immediately preceding
// the declaration, walking upward through wrappers: a doc comment
// sits above `export`, not between it and the class, so the search
// starts from the outermost node the declaration is nested in.
func (c *conv) attachDocs(base *node.BaseNode, n *ts.Node) {
	comments := precedingComments(outermost(n))
	if len(comments) == 0 {
		return
	}

	for _, cm := range comments {
		text := c.p.text(cm)
		base.DocLines = append(base.DocLines, commentLines(text)...)

		parsed, err := c.parser.ParseComment(text, c.p.posAt(cm))
		if err != nil {
			c.ps.Warnf(c.p.posAt(cm), "malformed directive: %v", err)
			continue
		}
		base.DirectiveList = append(base.DirectiveList, parsed...)
	}
}

// outermost walks up through the wrappers a declaration may be nested
// in — `export`, `export default`, `declare` — and returns the
// highest one, which is where a preceding comment attaches.
//
// Bounded by kind rather than by walking to the root: any other
// parent means the declaration is not top-level, and its comment is
// whatever precedes it in place.
func outermost(n *ts.Node) *ts.Node {
	for {
		parent := n.Parent()
		if parent == nil {
			return n
		}
		switch parent.Kind() {
		case "export_statement", "ambient_declaration":
			n = parent
		default:
			return n
		}
	}
}

// precedingComments returns the unbroken run of comment siblings
// immediately above n, in source order.
//
// The run stops at the first non-comment sibling, so a comment
// documenting the previous declaration does not attach to this one.
// It does not stop at a blank line: tree-sitter does not model
// whitespace, and TypeScript convention does not treat a blank line
// as detaching a JSDoc block the way gofmt does for Go.
func precedingComments(n *ts.Node) []*ts.Node {
	var out []*ts.Node
	for sib := n.PrevSibling(); sib != nil && sib.Kind() == "comment"; sib = sib.PrevSibling() {
		out = append(out, sib)
	}
	// Collected upward; reverse so the caller reads them top to bottom.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// commentLines converts one comment's source text into the
// line-oriented form [node.BaseNode.DocLines] expects — one entry per
// logical line with the markers stripped.
func commentLines(text string) []string {
	if strings.HasPrefix(text, "//") {
		return []string{strings.TrimPrefix(strings.TrimPrefix(text, "//"), " ")}
	}

	body := strings.TrimSuffix(strings.TrimPrefix(text, "/*"), "*/")
	// A JSDoc block opens `/**`, so the first `*` is part of the
	// marker rather than of the first line's content.
	body = strings.TrimPrefix(body, "*")

	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		trimmed := strings.TrimLeft(l, " \t")
		switch {
		case strings.HasPrefix(trimmed, "* "):
			out = append(out, strings.TrimPrefix(trimmed, "* "))
		case trimmed == "*":
			out = append(out, "")
		default:
			out = append(out, strings.TrimSpace(l))
		}
	}
	return trimEmptyEdges(out)
}

// trimEmptyEdges drops leading and trailing blank entries, which a
// block comment's own newlines produce and which carry no content.
func trimEmptyEdges(lines []string) []string {
	start, end := 0, len(lines)
	for start < end && lines[start] == "" {
		start++
	}
	for end > start && lines[end-1] == "" {
		end--
	}
	return lines[start:end]
}
