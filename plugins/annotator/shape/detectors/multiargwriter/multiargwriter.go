// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package multiargwriter

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps.
const Name = "multiargwriter"

// ArgTypes carries the full list of non-context parameter types
// stamped on a positive match.
//
//nolint:gochecknoglobals // registry-singleton key
var ArgTypes = sdk.NewKey("shape.multiargwriter.arg_types", sdk.StringListParser)

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 750,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang accepts a callable with three or more non-context
// parameters and a single trailing `error` return. The full
// argument-type list is stamped via [ArgTypes].
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	if !shape.GoHasError(returns) || len(shape.GoStripError(returns)) != 0 {
		return shape.Match{}, false
	}
	args := shape.GoStripContext(params)
	if len(args) < 3 {
		return shape.Match{}, false
	}
	qnames := make([]string, len(args))
	for i, a := range args {
		qnames[i] = shape.QName(a.Type)
	}
	return shape.Match{
		ListStamps: []shape.ListStamp{
			{Key: ArgTypes, Value: qnames},
		},
	}, true
}
