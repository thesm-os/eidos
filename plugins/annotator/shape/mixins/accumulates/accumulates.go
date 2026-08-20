// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package accumulates

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "accumulates"

// Mixin returns the [shape.Mixin] this package contributes.
//
// Param-less, and the contrast with retrysucceeds' `attempts=` is
// deliberate: an accumulation observed at any N of calls proves this
// claim, so the count is the check's choice — while a convergence
// bound is part of what a retrysucceeds author asserts, and so must
// be declared.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name}
}
