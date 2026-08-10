// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package lifecycle

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps. Consumers
// switching on [shape.Get] compare against this constant rather
// than the literal string so renames surface as compile errors.
const Name = "lifecycle"

// Detector returns the [shape.Detector] this package contributes
// to the umbrella shape plugin. Register one instance per
// pipeline:
//
//	pipe.WithAnnotator(shape.New().Detectors(lifecycle.Detector()))
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 200,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang recognises the canonical Go lifecycle signature:
// a single `context.Context` parameter and a single `error`
// return, with no other parameters or returns.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	if !shape.GoHasContext(params) || !shape.GoHasError(returns) {
		return shape.Match{}, false
	}
	rest := shape.GoStripContext(params)
	results := shape.GoStripError(returns)
	if len(rest) != 0 || len(results) != 0 {
		return shape.Match{}, false
	}
	return shape.Match{}, true
}
