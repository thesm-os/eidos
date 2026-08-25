// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go/token"

	"go.thesmos.sh/eidos/core/position"
)

// posOf converts a [go/token.Pos] into the language-agnostic
// [position.Pos] used throughout the eidos node model. The supplied
// [token.FileSet] is the same file set the Go parser used to load
// the position; passing the wrong file set produces a synthetic
// position rather than a misleading file:line:col.
//
// A zero [go/token.Pos] (NoPos) translates to a zero [position.Pos]
// — both signal "no source location available" in their respective
// models.
func posOf(fset *token.FileSet, p token.Pos) position.Pos {
	if !p.IsValid() {
		return position.Pos{}
	}
	pos := fset.Position(p)
	return position.AtOffset(pos.Filename, pos.Line, pos.Column, pos.Offset)
}
