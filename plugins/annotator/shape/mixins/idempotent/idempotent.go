// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package idempotent

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
// Consumers iterating [shape.Mixins] compare against this
// constant rather than the literal string so renames surface as
// compile errors.
const Name = "idempotent"

// Mixin returns the [shape.Mixin] this package contributes to
// the umbrella shape plugin. Register one instance per
// pipeline:
//
//	pipe.WithAnnotator(shape.New().Mixins(idempotent.Mixin()))
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name}
}
