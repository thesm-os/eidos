// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pointerreader

import (
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps.
const Name = "pointerreader"

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 450,
		Detect: map[string]shape.DetectFunc{
			golang.Language: detectGolang,
		},
	}
}

// detectGolang accepts a callable with exactly one non-context
// parameter and exactly one pointer-typed return (no error).
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := golang.Callable(n)
	keys := golang.StripContext(params)
	if len(keys) != 1 || len(returns) != 1 {
		return shape.Match{}, false
	}
	elem := golang.PointerElem(returns[0].Type)
	if elem == nil {
		return shape.Match{}, false
	}
	return shape.Match{
		KeyType:   golang.QName(keys[0].Type),
		ValueType: golang.QName(elem),
	}, true
}
