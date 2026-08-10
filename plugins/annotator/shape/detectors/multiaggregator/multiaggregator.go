// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package multiaggregator

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps.
const Name = "multiaggregator"

// ValueTypes carries the full list of non-error return types
// stamped on a positive match. The primary value also lands on
// the universal [shape.MetaValueType] for cross-shape uniformity.
//
//nolint:gochecknoglobals // registry-singleton key
var ValueTypes = sdk.NewKey("shape.multiaggregator.value_types", sdk.StringListParser)

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 600,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang accepts a callable with no non-context parameters
// and two or more non-error returns followed by a trailing
// error. The full non-error return list is stamped via
// [ValueTypes] so consumers can recover every value type.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	if len(shape.GoStripContext(params)) != 0 || !shape.GoHasError(returns) {
		return shape.Match{}, false
	}
	values := shape.GoStripError(returns)
	if len(values) < 2 {
		return shape.Match{}, false
	}
	qnames := make([]string, len(values))
	for i, v := range values {
		qnames[i] = shape.QName(v)
	}
	return shape.Match{
		ValueType: qnames[0],
		ListStamps: []shape.ListStamp{
			{Key: ValueTypes, Value: qnames},
		},
	}, true
}
