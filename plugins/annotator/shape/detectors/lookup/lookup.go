// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package lookup

import (
	"go.thesmos.sh/eidos/lang/golang"
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
			golang.Language: detectGolang,
		},
	}
}

// detectGolang accepts a callable with exactly one non-context
// parameter and exactly three returns: a value, metadata, and a
// bare bool sentinel. No error return. The metadata type is
// stamped via [MetaType] alongside the universal triple.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := golang.Callable(n)
	keys := golang.StripContext(params)
	if len(keys) != 1 || len(returns) != 3 {
		return shape.Match{}, false
	}
	if !golang.IsBool(returns[2].Type) {
		return shape.Match{}, false
	}
	return shape.Match{
		KeyType:   golang.QName(keys[0].Type),
		ValueType: golang.QName(returns[0].Type),
		StringStamps: []shape.StringStamp{
			{Key: MetaType, Value: golang.QName(returns[1].Type)},
		},
	}, true
}
