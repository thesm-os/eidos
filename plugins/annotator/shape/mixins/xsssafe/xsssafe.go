// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package xsssafe

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "xsssafe"

// ParamUnsafe is the KV key naming a declared value carrying what
// must not survive — markup that would render if the output reached
// an HTML context unescaped.
//
// The law's witness is a value the derivation cannot produce: drawn
// samples carry nothing dangerous, so a subject that escapes nothing
// passes on them, and the check reads as "this output is escaped" to
// anyone looking at the run report. The same trap validates closed
// with `invalid=`, proven there by deleting the subject's screening
// and watching the check stay green. Only the author knows which
// bracket or attribute their sink is dangerous to; the directive
// contributes the name.
//
// A package-level var, resolved through the var scope and stamped
// qualified — see [shape.KindVar]. Optional: an escaping callable
// without one is still what the mixin names, and a law handing the
// value to the call simply does not bind.
const ParamUnsafe = "unsafe"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamUnsafe, Kind: shape.KindVar},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
