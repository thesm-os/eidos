// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package leakfree

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "leakfree"

// ParamOpen is the KV key naming the callable that acquires the resource.
//
// A law selecting this mixin has to call it, and a stamp that names no
// callable leaves the binding's field nil — which does not weaken the
// check so much as remove it, since a law that never calls the method
// reports every implementation as correct.
const ParamOpen = "open"

// ParamClose is the KV key naming the callable that releases it.
//
// A law selecting this mixin has to call it, and a stamp that names no
// callable leaves the binding's field nil — which does not weaken the
// check so much as remove it, since a law that never calls the method
// reports every implementation as correct.
const ParamClose = "close"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamOpen, Kind: shape.KindCallable},
	{Key: ParamClose, Kind: shape.KindCallable},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:   Name,
		Params: Params,
	}
}
