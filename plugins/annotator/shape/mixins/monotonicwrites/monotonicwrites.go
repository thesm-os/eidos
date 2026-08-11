// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package monotonicwrites

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "monotonicwrites"

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name}
}
