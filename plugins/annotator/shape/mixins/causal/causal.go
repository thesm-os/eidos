// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package causal

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "causal"

// ParamVersion is the KV key naming the member of the read or written
// value that carries the ordering stamp.
//
// The same param the four session guarantees carry, and for the same
// reason: a law reading a trace per client orders operations by a
// store-assigned stamp — a logical clock, a row version, the global
// write order — and nothing in a signature says which member holds it.
//
// [shape.KindValueField], like theirs: the resolver checks the name
// against the value type's fields — the answered value, or the
// written one for a host answering nothing — and rewrites a hit into
// the qualified form every resolved kind takes. A typo is reported
// where the author is; a value type the run never loaded stamps
// unvalidated.
//
// A field, never a method, for the reason the session mixins give: the
// cas stamp assigns the member, and no method form can sit on the left
// of that assignment.
const ParamVersion = "version"

// Params enumerates the KV keys the directive accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamVersion, Kind: shape.KindValueField},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
