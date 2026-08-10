// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package aggregator

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps.
const Name = "aggregator"

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 350,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang accepts a callable with no non-context parameters
// and exactly one non-error return value, optionally accompanied
// by a trailing error.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	if len(shape.GoStripContext(params)) != 0 {
		return shape.Match{}, false
	}
	values := shape.GoStripError(returns)
	if len(values) != 1 {
		return shape.Match{}, false
	}
	return shape.Match{ValueType: shape.QName(values[0])}, true
}
