// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package serializable

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "serializable"

// ParamRead is the KV key naming the callable a check reads through
// when looking for an anomaly.
//
// Every anomaly this model rules out is defined by what a read
// observes, so a check without one can assert nothing about
// isolation — only that concurrent calls did not error.
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
