// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package monotonicwrites

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "monotonicwrites"

// ParamVersion is the KV key naming the member of the read or written
// value that carries the ordering stamp.
//
// The guarantee is defined against an ordering oracle — a logical
// clock, a row version, the global write order — and a law replaying a
// trace has to read it off each operation. Nothing in a signature says
// which member that is, and two candidates of the same type are
// ordinary.
//
// Opaque, like the cas contract's version=: the value names a field or
// a zero-argument method of the value type rather than a callable in
// scope or a package-level var, and neither resolver scope reaches a
// member of a type. Validating it would need a scope resolved against
// another declaration's type, which is the same mechanism deferred for
// a watcher's handle methods.
const ParamVersion = "version"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamVersion, Kind: shape.KindOpaque},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
