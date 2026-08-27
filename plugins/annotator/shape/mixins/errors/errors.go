// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package errors

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "errors"

// Mixin returns the [shape.Mixin] this package contributes.
// Documentary, which this package's own documentation has said in
// prose since it shipped: the mixin changes how a reader treats the
// error returns and licenses nothing falsifiable by itself. A
// consumer reporting the classifications no rule reached should not
// list this one, and the flag is how that travels without being
// transcribed.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Documentary: true}
}
