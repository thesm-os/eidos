// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package reader

import (
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
)

// Name is the canonical shape name this detector stamps. Consumers
// switching on [shape.Get] compare against this constant rather
// than the literal string so renames surface as compile errors.
const Name = "reader"

// Detector returns the [shape.Detector] this package contributes
// to the umbrella shape plugin. Register one instance per
// pipeline:
//
//	pipe.Use(shape.New().Detectors(reader.Detector()))
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 420,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang recognises the canonical Go reader signature:
// exactly one non-context input parameter, exactly one non-error
// return value, plus a trailing `error` return. The leading
// `context.Context` parameter is optional — both `(ctx, K)` and
// `(K)` forms detect.
//
// A parameter that cannot serve as a key is refused — see
// [keyable]. The stamp's whole content is that the parameter *is* a
// key; generators derive same-key-same-value, read-after-write and
// deterministic re-read from it, and a parameter with no equality
// makes all three vacuous rather than false.
func detectGolang(n node.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	if !shape.GoHasError(returns) {
		return shape.Match{}, false
	}
	keys := shape.GoStripContext(params)
	values := shape.GoStripError(returns)
	if len(keys) != 1 || len(values) != 1 {
		return shape.Match{}, false
	}
	if !keyable(keys[0].Type) {
		return shape.Match{}, false
	}
	return shape.Match{
		KeyType:   shape.QName(keys[0].Type),
		ValueType: shape.QName(values[0]),
	}, true
}

// keyable reports whether t can carry the reader shape's key
// contract: two calls with equal keys must be able to mean
// something. Slices, maps and functions have no equality in Go at
// all, so "the same key" is not expressible for them. An inline
// interface is refused for the adjacent reason — a stream passed as
// `interface{ Read(...) }` re-reads to the zero value, so a
// same-key-same-value assertion passes without testing anything.
//
// Two limits are deliberate.
//
// It is narrower than the problem: a *named* interface, `io.Reader`
// being the common case, is opaque here. The node IR records a
// package and an identifier and nothing about what they resolve to,
// so it still detects as a reader. Closing that needs the frontend
// to stamp interface-ness on the ref; this covers the inline form
// only, and the check is written so that adding the named case is a
// single clause here.
//
// It is broader than strictly necessary: interface values compare
// fine when their dynamic types do, so a lookup keyed on
// `interface{ String() string }` is legitimate and refused anyway.
// That trade is intentional. Refusing a valid key costs a missing
// stamp, which a `+gen:shape` directive can restore; accepting an
// invalid one costs a wrong stamp that downstream tooling acts on
// with no signal that anything is off.
func keyable(t *node.TypeRef) bool {
	if t == nil {
		return false
	}
	switch {
	case t.IsSlice(), t.IsMap(), t.IsFunc():
		return false
	case t.IsAnonInterface():
		return false
	default:
		return true
	}
}
