// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package lookup

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps.
const Name = "lookup"

// MetaType carries the qualified type of the Meta return slot
// (the second non-bool value in the (V, Meta, bool) return
// triple). Empty when no lookup is detected.
//
//nolint:gochecknoglobals // registry-singleton key
var MetaType = sdk.NewKey("shape.lookup.meta_type", sdk.StringParser)

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 850,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang accepts a callable with exactly one non-context
// parameter and exactly three returns: a value, metadata, and a
// bare bool sentinel. No error return. The metadata type is
// stamped via [MetaType] alongside the universal triple.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	keys := shape.GoStripContext(params)
	if len(keys) != 1 || len(returns) != 3 {
		return shape.Match{}, false
	}
	if !shape.GoIsBool(returns[2].Type) {
		return shape.Match{}, false
	}
	return shape.Match{
		KeyType:   shape.QName(keys[0].Type),
		ValueType: shape.QName(returns[0].Type),
		StringStamps: []shape.StringStamp{
			{Key: MetaType, Value: shape.QName(returns[1].Type)},
		},
	}, true
}
