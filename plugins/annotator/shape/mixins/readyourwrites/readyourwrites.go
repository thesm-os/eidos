// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package readyourwrites

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "readyourwrites"

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name}
}
