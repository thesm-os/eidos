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
// Opaque, like theirs: the value names a field or a zero-argument
// method of the value type rather than a callable in scope or a
// package-level var, and reaching a member of a *value* type is not
// what [shape.KindMember] resolves — that answers a role's returned
// handle.
const ParamVersion = "version"

// Params enumerates the KV keys the directive accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamVersion, Kind: shape.KindOpaque},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
