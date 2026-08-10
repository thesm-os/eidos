// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package readernoerror

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps.
const Name = "readernoerror"

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 400,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang accepts a callable with exactly one non-context
// parameter and exactly one non-error return.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	keys := shape.GoStripContext(params)
	values := shape.GoStripError(returns)
	if len(keys) != 1 || len(returns) != 1 || len(values) != 1 {
		return shape.Match{}, false
	}
	return shape.Match{
		KeyType:   shape.QName(keys[0].Type),
		ValueType: shape.QName(values[0]),
	}, true
}
