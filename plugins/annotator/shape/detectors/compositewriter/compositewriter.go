// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package compositewriter

import (
	"go.thesmos.sh/eidos/lang/golang"
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
			golang.Language: detectGolang,
		},
	}
}

// detectGolang accepts a callable with exactly two non-context
// parameters and a single trailing `error` return.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := golang.Callable(n)
	if !golang.HasError(returns) || len(golang.StripErrorTypes(returns)) != 0 {
		return shape.Match{}, false
	}
	args := golang.StripContext(params)
	if len(args) != 2 {
		return shape.Match{}, false
	}
	return shape.Match{
		KeyType:   golang.QName(args[0].Type),
		ValueType: golang.QName(args[1].Type),
	}, true
}
