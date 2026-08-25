// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package streamconsumer

import (
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps. Consumers
// switching on [shape.Get] compare against this constant rather
// than the literal string so renames surface as compile errors.
const Name = "streamconsumer"

// MetaStreamType carries the consumed stream's type. It is not
// [shape.MetaKeyType]: recording `io.Reader` as a key is the false
// claim this shape exists to remove, since a generator branching on
// key_type emits same-key-same-value assertions that a drained
// stream satisfies vacuously.
//
//nolint:gochecknoglobals // cross-package registry-singleton key
var MetaStreamType = sdk.EnsureKey("shape.stream_type", sdk.StringParser)

// Detector returns the [shape.Detector] this package contributes
// to the umbrella shape plugin. Register one instance per
// pipeline:
//
//	pipe.WithAnnotator(shape.New().Detectors(streamconsumer.Detector()))
//
// Priority 470 places it above `reader` (420), which would
// otherwise claim these signatures and label the stream a key, and
// below `writer` (500), whose accept set is disjoint. It does not
// compete with `pointerreader` (450), which requires a single
// return and so rejects the trailing error first.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 470,
		Detect: map[string]shape.DetectFunc{
			golang.Language: detectGolang,
		},
	}
}

// detectGolang recognises a callable that consumes a stream: a
// context, exactly one further parameter of interface type, and
// exactly one value alongside a trailing error.
//
// The context is required deliberately. `func(io.Reader) (V, error)`
// is too ambiguous to claim — constructors and helpers share it —
// and the cost of requiring ctx is a missed stamp on an unusual
// form, against mislabelling a common one.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := golang.Callable(n)
	if !golang.HasContext(params) || !golang.HasError(returns) {
		return shape.Match{}, false
	}
	src := golang.StripContext(params)
	vals := golang.StripErrorTypes(returns)
	if len(src) != 1 || len(vals) != 1 {
		return shape.Match{}, false
	}
	if !isStream(src[0].Type) {
		return shape.Match{}, false
	}
	return shape.Match{
		ValueType: golang.QName(vals[0]),
		StringStamps: []shape.StringStamp{
			{Key: MetaStreamType, Value: golang.QName(src[0].Type)},
		},
	}, true
}

// isStream reports whether t is an interface the callable consumes.
//
// An inline interface is decidable from the node graph alone. A named
// one — `io.Reader`, the common case — is not: the graph records a
// package and an identifier and nothing about what they resolve to,
// so [golang.IsInterface] reads the frontend's stamp rather than
// resolving the name, which a plugin cannot do for types outside the
// loaded packages.
//
// Type parameters are excluded twice over: the frontend does not
// stamp them, and the graph discriminates them outright. A constraint
// is an interface, but `K comparable` is a key.
func isStream(t *sdk.TypeRef) bool {
	if t == nil || t.IsTypeParam() {
		return false
	}
	return t.IsAnonInterface() || golang.IsInterface(t)
}
