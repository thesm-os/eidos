// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package indexed

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "indexed"

// ParamBy is the KV key naming the callable that reports the
// collection's size.
//
// A sibling callable, so a misspelling fails at the directive rather
// than as an out-of-range index in generated code. What it cannot do
// is supply the bound: the size is a runtime fact about the subject's
// seeded state, so a consumer calls the named method and draws inside
// what it answers.
const ParamBy = "by"

// Params enumerates the KV keys the directive accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamBy, Kind: shape.KindCallable},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
