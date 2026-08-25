// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package atomic

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
// Consumers iterating [shape.Mixins] compare against this
// constant rather than the literal string so renames surface as
// compile errors.
const Name = "atomic"

// ParamRead is the KV key naming the callable that reads the state
// back, which is how a check sees whether a failed call left
// anything behind.
//
// Without it the claim licenses only "call it and it succeeds" — the
// smoke check, which passes against an implementation that half-
// completes and reports nothing. Pair with the `poisonable` mixin to
// name what makes the call fail in the first place; this names what
// looks afterwards.
const ParamRead = "read"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamRead, Kind: shape.KindCallable},
}

// Mixin returns the [shape.Mixin] this package contributes to
// the umbrella shape plugin. Register one instance per
// pipeline:
//
//	pipe.WithAnnotator(shape.New().Mixins(atomic.Mixin()))
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
