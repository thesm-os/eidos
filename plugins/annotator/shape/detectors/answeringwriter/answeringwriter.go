// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package answeringwriter

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this package stamps.
const Name = "answeringwriter"

// Priority places this detector above [reader] and below
// [pointerreader].
//
// Above reader because every signature this draws is one reader would
// otherwise claim, recording the written value as a key. Below
// pointerreader because that shape returns a bare pointer with no
// error and cannot collide. Nothing between the two is disturbed.
const Priority = 440

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: Priority,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang recognises one non-context parameter answered by its
// own type beside an error.
//
// Type identity is the whole rule. Accepting any single non-error
// result would take every reader with it, which is the failure the
// [writer] detector's own comment records from the other direction:
// a permissive rule at a higher priority claimed reads and labelled
// the key as the written value.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	if !shape.GoHasError(returns) {
		return shape.Match{}, false
	}
	values := shape.GoStripContext(params)
	results := shape.GoStripError(returns)
	if len(values) != 1 || len(results) != 1 {
		return shape.Match{}, false
	}
	written := shape.QName(values[0].Type)
	if written == "" || written != shape.QName(results[0]) {
		return shape.Match{}, false
	}
	return shape.Match{ValueType: written}, true
}
