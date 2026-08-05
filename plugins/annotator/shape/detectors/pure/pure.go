// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pure

import (
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
)

// Name is the canonical shape name this detector stamps. Consumers
// switching on [shape.Get] compare against this constant rather
// than the literal string so renames surface as compile errors.
const Name = "pure"

// Detector returns the [shape.Detector] this package contributes
// to the umbrella shape plugin. Register one instance per
// pipeline:
//
//	pipe.Use(shape.New().Detectors(pure.Detector()))
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 800,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang recognises a pure Go callable: no leading
// context, no error return, exactly one return value. Parameter
// count and types are unconstrained — the shape is about the
// return discipline, not the input shape.
func detectGolang(n node.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	if shape.GoHasContext(params) || shape.GoHasError(returns) {
		return shape.Match{}, false
	}
	if len(returns) != 1 {
		return shape.Match{}, false
	}
	return shape.Match{
		ValueType: shape.QName(returns[0].Type),
	}, true
}
