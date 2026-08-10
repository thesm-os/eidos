// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package compositewriter

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps.
const Name = "compositewriter"

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 700,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang accepts a callable with exactly two non-context
// parameters and a single trailing `error` return.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	if !shape.GoHasError(returns) || len(shape.GoStripError(returns)) != 0 {
		return shape.Match{}, false
	}
	args := shape.GoStripContext(params)
	if len(args) != 2 {
		return shape.Match{}, false
	}
	return shape.Match{
		KeyType:   shape.QName(args[0].Type),
		ValueType: shape.QName(args[1].Type),
	}, true
}
