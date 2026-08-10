// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package mutator

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps.
const Name = "mutator"

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 300,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang accepts the void-return mutator signatures:
// exactly one non-context parameter, zero return values. Strips
// the `*V` pointer wrapping when present so the stamped value
// type names the underlying element.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	if len(returns) != 0 {
		return shape.Match{}, false
	}
	values := shape.GoStripContext(params)
	if len(values) != 1 {
		return shape.Match{}, false
	}
	valueType := values[0].Type
	if elem := shape.GoPointerElem(valueType); elem != nil {
		valueType = elem
	}
	return shape.Match{ValueType: shape.QName(valueType)}, true
}
