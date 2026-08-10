// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package batchreader

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps.
const Name = "batchreader"

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 950,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang accepts a callable whose only non-context
// parameter is a trailing variadic `...K`, and whose only
// non-error return is a slice `[]V`.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	if !shape.GoHasError(returns) {
		return shape.Match{}, false
	}
	values := shape.GoStripError(returns)
	if len(values) != 1 {
		return shape.Match{}, false
	}
	elem := shape.GoSliceElem(values[0])
	if elem == nil {
		return shape.Match{}, false
	}
	args := shape.GoStripContext(params)
	if len(args) != 1 {
		return shape.Match{}, false
	}
	variadic := shape.GoTrailingVariadic(args)
	if variadic == nil {
		return shape.Match{}, false
	}
	return shape.Match{
		KeyType:   shape.QName(variadic.Type),
		ValueType: shape.QName(elem),
	}, true
}
