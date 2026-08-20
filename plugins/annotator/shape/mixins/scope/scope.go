// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package scope

import (
	"fmt"
	"slices"

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
// callable that carries the scope — the same job `axis=` does on
// the partition mixin. Naming it is what lets a check vary the
// scope while holding everything else fixed; without it the
// isolation half of the claim licenses nothing.
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
// Partition's reasoning, verbatim: a misspelled axis stamps like any
// other opaque value, so without this the generated check varies
// nothing and passes against every implementation — the silent shape
// the axis exists to prevent. Absence is not reported; the bare form
// classifies and a consumer without an axis declines the isolation
// check.
func validateAxis(attachments []shape.MixinAttachment) []shape.MixinViolation {
	var out []shape.MixinViolation
	for _, attached := range attachments {
		axis, given := attached.Params[ParamAxis]
		if !given || axis == "" {
			continue
		}
		params, _ := shape.GoCallable(attached.Host)
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
