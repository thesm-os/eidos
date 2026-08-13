// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package conservative

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "conservative"

// ParamField is the KV key naming the quantity the operation conserves.
//
// Named because a consumer reaching for it otherwise writes
// `Params[0]`, which is a position rather than a key.
const ParamField = "field"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamField, Kind: shape.KindOpaque},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
