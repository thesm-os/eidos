// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	ts "github.com/tree-sitter/go-tree-sitter"
)

// maxSyntaxReports bounds the syntax errors reported per file.
//
// tree-sitter recovers from an error and keeps going, so one missing
// brace early in a file can leave every subsequent construct
// mis-parsed and produce a report per line. The first few name the
// real problem; the rest are its shadow.
const maxSyntaxReports = 5

// reportSyntaxErrors walks a parsed tree and attaches a positioned
// diagnostic for each syntax error it finds.
//
// Warn rather than Error, and the run continues. tree-sitter's error
// recovery means a file with a syntax error still yields usable
// declarations either side of it, and dropping the whole file would
// discard them. A consumer that wants strictness reads the
// diagnostics.
func (c *conv) reportSyntaxErrors() {
	root := c.p.root()
	if !root.HasError() {
		return
	}

	reported := 0
	cursor := root.Walk()
	defer cursor.Close()

	var walk func(n *ts.Node)
	walk = func(n *ts.Node) {
		if reported >= maxSyntaxReports {
			return
		}
		if n.IsError() || n.IsMissing() {
			c.ps.Warnf(c.p.posAt(n), "syntax error: %s", c.syntaxDetail(n))
			reported++
			return
		}
		if !n.HasError() {
			// Whole subtrees with no error inside are skipped rather
			// than walked; HasError on a parent is what makes this a
			// descent along the error path instead of a full traversal.
			return
		}
		for i := range n.ChildCount() {
			walk(n.Child(i))
		}
	}
	walk(root)

	if root.HasError() && reported >= maxSyntaxReports {
		c.ps.Warnf(c.p.posAt(root),
			"further syntax errors suppressed after %d", maxSyntaxReports)
	}
}

// syntaxDetail describes one error node for a diagnostic.
func (c *conv) syntaxDetail(n *ts.Node) string {
	if n.IsMissing() {
		return "missing " + n.Kind()
	}
	text := c.p.text(n)
	const maxQuoted = 40
	if len(text) > maxQuoted {
		text = text[:maxQuoted] + "…"
	}
	if text == "" {
		return "unparseable input"
	}
	return "unexpected " + text
}
