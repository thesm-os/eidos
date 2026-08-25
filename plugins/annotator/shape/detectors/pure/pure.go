// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pure

import (
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps. Consumers
// switching on [shape.Get] compare against this constant rather
// than the literal string so renames surface as compile errors.
const Name = "pure"

// Detector returns the [shape.Detector] this package contributes
// to the umbrella shape plugin. Register one instance per
// pipeline:
//
//	pipe.WithAnnotator(shape.New().Detectors(pure.Detector()))
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 800,
		Detect: map[string]shape.DetectFunc{
			golang.Language: detectGolang,
		},
	}
}

// detectGolang recognises a pure Go callable: no leading
// context, no error return, exactly one return value. Parameter
// count and types are unconstrained — the shape is about the
// return discipline, not the input shape.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := golang.Callable(n)
	if golang.HasContext(params) || golang.HasError(returns) {
		return shape.Match{}, false
	}
	if len(returns) != 1 {
		return shape.Match{}, false
	}
	return shape.Match{
		ValueType: golang.QName(returns[0].Type),
	}, true
}
