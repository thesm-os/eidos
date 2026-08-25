// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package snapshotisolation

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "snapshotisolation"

// ParamRead is the KV key naming the callable a check reads through
// when looking for an anomaly.
//
// Dirty read, dirty write and read skew are each defined by what a
// read observes, so the param is what makes them checkable — and
// what lets a check confirm write skew is *permitted* rather than
// treating its appearance as a failure.
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
