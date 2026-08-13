// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package windowed

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "windowed"

// ParamIncr is the KV key naming the callable that records an observation.
//
// A law selecting this mixin has to call it, and a stamp that names no
// callable leaves the binding's field nil — which does not weaken the
// check so much as remove it, since a law that never calls the method
// reports every implementation as correct.
const ParamIncr = "incr"

// ParamCount is the KV key naming the callable that reports the window's total.
//
// A law selecting this mixin has to call it, and a stamp that names no
// callable leaves the binding's field nil — which does not weaken the
// check so much as remove it, since a law that never calls the method
// reports every implementation as correct.
const ParamCount = "count"

// ParamWindow is the KV key naming the interval the result covers.
//
// A law advances a controlled clock past it and requires the count to drop.
// With no window it advances past zero, which either fails a correct
// implementation or passes vacuously depending on where zero lands.
//
// Opaque to the resolver: a quantity names neither a callable nor a var.
const ParamWindow = "window"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamIncr, Kind: shape.KindCallable},
	{Key: ParamCount, Kind: shape.KindCallable},
	{Key: ParamWindow, Kind: shape.KindOpaque},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:   Name,
		Params: Params,
	}
}
