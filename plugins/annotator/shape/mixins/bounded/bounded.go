// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package bounded

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "bounded"

// ParamLimit is the KV key naming the upper bound the annotated subject holds to.
//
// Named because a consumer reaching for it otherwise writes
// `Params[0]`, which is a position rather than a key: reordering
// the list — or adding a second parameter ahead of this one —
// silently changes what every such call site reads.
const ParamLimit = "limit"

// ParamMin is the KV key naming the floor the resource consumption stays at or above.
//
// [ParamLimit] is the ceiling and says nothing about the other end. Zero is a
// sound floor for a counting shape and wrong for anything signed, so it is
// declared rather than assumed.
//
// Opaque to the resolver: a quantity names neither a callable nor a var.
const ParamMin = "min"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []string{ParamMin, ParamLimit}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
