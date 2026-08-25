// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package streamreader

import (
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps.
const Name = "streamreader"

// The values [Variant] carries, naming which iterator the callable
// returns.
//
// Spelled here rather than taken from [golang.Iterator]'s own
// constants: this is a stamp consumers read back, so its spelling is
// this package's API and must not move when the classifier's internal
// naming does.
const (
	VariantSeq  = "seq"
	VariantSeq2 = "seq2"
)

// Variant carries which iterator type the callable returns —
// [VariantSeq] for `iter.Seq[V]` or [VariantSeq2] for
// `iter.Seq2[V, …]`.
//
//nolint:gochecknoglobals // registry-singleton key
var Variant = sdk.NewKey("shape.streamreader.variant", sdk.StringParser)

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 1000,
		Detect: map[string]shape.DetectFunc{
			golang.Language: detectGolang,
		},
	}
}

// detectGolang accepts a callable with at most one non-context
// parameter (the optional input key) and exactly one return: an
// `iter.Seq[V]` or `iter.Seq2[V, …]` reference. The variant is
// stamped via [Variant] so consumers can discriminate the two
// iterator shapes.
//
// Both variants stamp the sequence's first type argument as the
// value, which is what a caller collects in either form.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := golang.Callable(n)
	if len(returns) != 1 {
		return shape.Match{}, false
	}
	variant := variantOf(returns[0].Type)
	if variant == "" {
		return shape.Match{}, false
	}
	keys := golang.StripContext(params)
	if len(keys) > 1 {
		return shape.Match{}, false
	}
	match := shape.Match{
		ValueType: golang.QName(golang.IteratorElem(returns[0].Type)),
		StringStamps: []shape.StringStamp{
			{Key: Variant, Value: variant},
		},
	}
	if len(keys) == 1 {
		match.KeyType = golang.QName(keys[0].Type)
	}
	return match, true
}

// variantOf names the sequence t is, or empty when it is not one.
//
// Matched on the stdlib package and arity through
// [golang.IteratorOfType] rather than on the frontend's stamp: that
// one is keyed on the declaration, and this asks about a return slot.
func variantOf(t *sdk.TypeRef) string {
	switch golang.IteratorOfType(t) {
	case golang.SeqIterator:
		return VariantSeq
	case golang.Seq2Iterator:
		return VariantSeq2
	default:
		return ""
	}
}
