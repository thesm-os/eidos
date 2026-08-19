// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ttl

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "ttl"

// ParamDuration is the KV key naming how long an entry stays readable.
//
// Opaque to the resolver: a quantity names neither a callable nor a
// var. A lifetime nobody declared is a number the generator invented,
// and a law enforcing an invented lifetime fails implementations that
// are correct against the one their author meant.
const ParamDuration = "duration"

// ParamPut is the KV key naming the callable that stores an entry.
const ParamPut = "put"

// ParamRead is the KV key naming the callable that reads it back.
const ParamRead = "read"

// ParamNotFound is the KV key naming the sentinel the read reports
// once the entry has expired.
//
// A sentinel is a package-level var, so it resolves through the var
// scope rather than the callable one — see [shape.KindVar].
//
// Expiry-specific, not the read's general miss sentinel — that is the
// `notfound` mixin's, declared on the read itself. The two usually
// coincide, and a consumer deriving the expiry law should read this
// key first and fall back to the read's declared miss sentinel when
// absent. Declare both only when a lapsed read reports differently
// from a missing one.
const ParamNotFound = "notfound"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamDuration, Kind: shape.KindOpaque},
	{Key: ParamPut, Kind: shape.KindCallable},
	{Key: ParamRead, Kind: shape.KindCallable},
	{Key: ParamNotFound, Kind: shape.KindVar},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:   Name,
		Params: Params,
	}
}
