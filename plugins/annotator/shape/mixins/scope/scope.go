// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package scope

import (
	"fmt"
	"slices"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
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
// host's parameter list, because a pointer that is wrong in
// documentation misleads exactly as long as one in a check would.
const ParamAxis = "axis"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamName, Kind: shape.KindOpaque},
	{Key: ParamAxis, Kind: shape.KindOpaque},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:     Name,
		Params:   Params,
		Validate: validateAxis,
	}
}

// validateAxis reports an axis that does not name a parameter of the
// annotated callable.
//
// A misspelled axis stamps like any other opaque value and points a
// reader — human or tooling — at a parameter that is not there.
// Absence is not reported: name= alone classifies, and an axis is
// only owed where an author wants the carrier named.
func validateAxis(attachments []shape.MixinAttachment) []shape.MixinViolation {
	var out []shape.MixinViolation
	for _, attached := range attachments {
		axis, given := attached.Params[ParamAxis]
		if !given || axis == "" {
			continue
		}
		params, _ := golang.Callable(attached.Host)
		named := slices.ContainsFunc(params, func(p *sdk.Param) bool {
			return p != nil && p.Name == axis
		})
		if named {
			continue
		}
		out = append(out, shape.MixinViolation{
			Host: attached.Host,
			Message: fmt.Sprintf(
				"%s=%q names no parameter of the annotated callable", ParamAxis, axis,
			),
		})
	}
	return out
}
