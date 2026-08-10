// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package streamreader

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps.
const Name = "streamreader"

// Variant carries which iterator type the callable returns —
// `"seq"` for `iter.Seq[V]` or `"seq2"` for `iter.Seq2[V, …]`.
//
//nolint:gochecknoglobals // registry-singleton key
var Variant = sdk.NewKey("shape.streamreader.variant", sdk.StringParser)

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 1000,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang accepts a callable with at most one non-context
// parameter (the optional input key) and exactly one return: an
// `iter.Seq[V]` or `iter.Seq2[V, …]` reference. The variant is
// stamped via [Variant] so consumers can discriminate the two
// iterator shapes.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	if len(returns) != 1 {
		return shape.Match{}, false
	}
	variant := ""
	elem := shape.GoIterSeqElem(returns[0].Type)
	if elem != nil {
		variant = "seq"
	} else if k, _ := shape.GoIterSeq2Args(returns[0].Type); k != nil {
		elem = k
		variant = "seq2"
	}
	if elem == nil {
		return shape.Match{}, false
	}
	keys := shape.GoStripContext(params)
	if len(keys) > 1 {
		return shape.Match{}, false
	}
	match := shape.Match{
		ValueType: shape.QName(elem),
		StringStamps: []shape.StringStamp{
			{Key: Variant, Value: variant},
		},
	}
	if len(keys) == 1 {
		match.KeyType = shape.QName(keys[0].Type)
	}
	return match, true
}
