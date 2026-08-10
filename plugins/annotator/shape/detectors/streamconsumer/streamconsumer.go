// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package streamconsumer

import (
	langgo "go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps. Consumers
// switching on [shape.Get] compare against this constant rather
// than the literal string so renames surface as compile errors.
const Name = "streamconsumer"

// Keys are resolved through the process-wide registry rather than
// imported from `frontend/golang`, which the `plugins` depguard rule
// denies — the pattern `shape.frontendMarker` already uses.
// The Go vocabulary lives in lang/golang, which plugins may import
// — depguard denies only `frontend/`. Reading the declaration
// rather than re-registering the name by string is what makes a
// rename a build failure instead of a fact that is silently always
// absent.
//
//nolint:gochecknoglobals // cross-package registry-singleton keys
var (
	metaIsInterface = langgo.MetaIsInterface

	// MetaStreamType carries the consumed stream's type. It is not
	// [shape.MetaKeyType]: recording `io.Reader` as a key is the
	// false claim this shape exists to remove, since a generator
	// branching on key_type emits same-key-same-value assertions
	// that a drained stream satisfies vacuously.
	MetaStreamType = sdk.EnsureKey("shape.stream_type", sdk.StringParser)
)

// Detector returns the [shape.Detector] this package contributes
// to the umbrella shape plugin. Register one instance per
// pipeline:
//
//	pipe.Use(shape.New().Detectors(streamconsumer.Detector()))
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
			"golang": detectGolang,
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
	params, returns := shape.GoCallable(n)
	if !shape.GoHasContext(params) || !shape.GoHasError(returns) {
		return shape.Match{}, false
	}
	src := shape.GoStripContext(params)
	vals := shape.GoStripError(returns)
	if len(src) != 1 || len(vals) != 1 {
		return shape.Match{}, false
	}
	if !isStream(src[0].Type) {
		return shape.Match{}, false
	}
	return shape.Match{
		ValueType: shape.QName(vals[0]),
		StringStamps: []shape.StringStamp{
			{Key: MetaStreamType, Value: shape.QName(src[0].Type)},
		},
	}, true
}

// isStream reports whether t is an interface the callable consumes.
//
// An inline interface is decidable from the IR alone. A named one —
// `io.Reader`, the common case — is not: the node graph records a
// package and an identifier and nothing about what they resolve to,
// so this reads the frontend's [go.isInterface] stamp instead of
// resolving the name, which a plugin cannot do for types outside
// the loaded packages.
//
// Type parameters are excluded twice over: the frontend does not
// stamp them, and the IR discriminates them outright. A constraint
// is an interface, but `K comparable` is a key.
func isStream(t *sdk.TypeRef) bool {
	if t == nil || t.IsTypeParam() {
		return false
	}
	if t.IsAnonInterface() {
		return true
	}
	stamped, _ := metaIsInterface.Get(t.Meta())
	return stamped
}
