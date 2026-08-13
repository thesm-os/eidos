// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package poisonable

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "poisonable"

// ParamInduce is the KV key naming the callable that puts the subject
// into its failure state.
//
// A law selecting this mixin induces the state and then asserts the
// accessor agrees from then on. With nothing to call it observes the
// healthy state and reports every implementation as correct.
const ParamInduce = "induce"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamInduce, Kind: shape.KindCallable},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:   Name,
		Params: Params,
	}
}
