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

// ParamLifetime is the KV key naming the member of the stored value
// that carries its own lifetime.
//
// The other half of the pattern [ParamDuration] covers. A cache or a
// session store commonly gives each entry its own expiry rather than
// one the API fixes, and until this key existed such a store could
// say nothing: the declaration was complete, the law was the same
// sentence — an entry stops being readable once its lifetime has run
// out — and a consumer reading the directive could not tell where the
// lifetime lived. The remedy on offer was `duration=`, which asserts
// a fixed lifetime the store does not have.
//
// [shape.KindValueField], not [shape.KindMember]: a lifetime is a
// field of the value the write stores, not a method on a handle a
// callable answers. The resolver checks the name against that value's
// fields — pointer-stripped, promotion honoured — and rewrites a hit
// into the qualified form; a typo is reported where the author is,
// and a value type the run never loaded stamps unvalidated.
//
// Mutually exclusive with [ParamDuration]; see the Validate hook.
const ParamLifetime = "lifetime"

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
	{Key: ParamLifetime, Kind: shape.KindValueField},
	{Key: ParamPut, Kind: shape.KindCallable},
	{Key: ParamRead, Kind: shape.KindCallable},
	{Key: ParamNotFound, Kind: shape.KindVar},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:     Name,
		Params:   Params,
		Validate: validate,
	}
}

// validate refuses a lifetime declared twice over.
//
// [ParamDuration] and [ParamLifetime] answer the same question and
// answer it differently — one fixes a lifetime for every entry, the
// other reads it off the entry. A directive carrying both leaves a
// consumer to pick, and either choice makes the other key a line the
// author wrote that does nothing.
//
// Neither is required. A `ttl` with no lifetime at all still
// classifies the pair as expiring, which is a fact a reader wants
// even where no check can hold a call to a clock; refusing the bare
// form would rule out the classification for every store whose
// expiry is real and unstated.
func validate(attachments []shape.MixinAttachment) []shape.MixinViolation {
	var out []shape.MixinViolation
	for _, attached := range attachments {
		if attached.Params[ParamDuration] == "" || attached.Params[ParamLifetime] == "" {
			continue
		}
		out = append(out, shape.MixinViolation{
			Host: attached.Host,
			Message: "ttl accepts " + ParamDuration + "= or " + ParamLifetime +
				"=, not both: a lifetime is either fixed by the directive or " +
				"carried by the value, and naming both leaves which one governs unsaid",
		})
	}
	return out
}
