// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package readerwithbool

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps.
const Name = "readerwithbool"

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 840,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang accepts a callable with exactly one non-context
// parameter and exactly two returns: a value followed by a bare
// bool. No error return.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	keys := shape.GoStripContext(params)
	if len(keys) != 1 || len(returns) != 2 {
		return shape.Match{}, false
	}
	if !shape.GoIsBool(returns[1].Type) {
		return shape.Match{}, false
	}
	return shape.Match{
		KeyType:   shape.QName(keys[0].Type),
		ValueType: shape.QName(returns[0].Type),
	}, true
}
