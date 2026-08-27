// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package scope

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
)

// Name is the canonical mixin name this package stamps.
const Name = "scope"

// ParamName is the KV key naming the scope the effect is confined
// to — request, session, tenant. Opaque: a scope name is a word,
// not a declaration.
const ParamName = "name"

// ParamAxis is the KV key naming the parameter of the annotated
// callable that carries the scope.
//
// Validated documentation, not a check-enabler: the checkable form
// of the boundary claim is partition's, which names an observer
// beside the axis. The pointer is still validated against the
// host's parameter list — [shape.KindParam], which retired the
// Validate hook that carried the same check by hand — because a
// pointer that is wrong in documentation misleads exactly as long
// as one in a check would.
const ParamAxis = "axis"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamName, Kind: shape.KindOpaque},
	{Key: ParamAxis, Kind: shape.KindParam},
}

// Mixin returns the [shape.Mixin] this package contributes.
//
// No Validate hook: the one thing it checked — the axis naming a
// parameter of the host — is what [shape.KindParam] declares, where
// forgetting the check is impossible rather than invisible.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
