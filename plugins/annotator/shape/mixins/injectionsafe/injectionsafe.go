// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package injectionsafe

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "injectionsafe"

// ParamRead is the KV key naming the callable that reads the stored
// value back.
//
// The claim is that hostile input is stored as data rather than
// executed as syntax, and the difference is only visible on the way
// out: a check writes a payload that would be syntax to the
// interpreter downstream and reads it back to find it intact.
const ParamRead = "read"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamRead, Kind: shape.KindCallable},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
