// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package predicate

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps.
const Name = "predicate"

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 820,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang accepts a callable taking nothing and returning a
// single bare `bool`.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	if len(params) != 0 || len(returns) != 1 {
		return shape.Match{}, false
	}
	if !shape.GoIsBool(returns[0].Type) {
		return shape.Match{}, false
	}
	return shape.Match{}, true
}
