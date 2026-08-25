// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/core/position"
)

// posAt converts a tree-sitter node's start location into a
// [position.Pos] against the parsed file.
//
// tree-sitter counts rows and columns from zero; [position.Pos]
// follows go/token and counts from one, while Offset stays zero-based
// in both. The conversion is therefore not symmetric across the three
// fields, which is the whole reason it lives in one function.
//
// A nil node yields a Pos naming the file and nothing else, which is
// what [position.Pos] documents as "the file is known even if no line
// is" — better for a diagnostic than the zero value, which reads as
// no position at all.
func (p *parsed) posAt(n *ts.Node) position.Pos {
	if n == nil {
		return position.Pos{File: p.path}
	}
	start := n.StartPosition()
	return position.AtOffset(
		p.path,
		int(start.Row)+1,
		int(start.Column)+1,
		int(n.StartByte()),
	)
}
