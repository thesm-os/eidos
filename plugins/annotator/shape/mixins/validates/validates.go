// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package validates

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "validates"

// ParamFn is the KV key naming the validator the annotated subject runs.
//
// Named because a consumer reaching for it otherwise writes
// `Params[0]`, which is a position rather than a key: reordering
// the list — or adding a second parameter ahead of this one —
// silently changes what every such call site reads.
const ParamFn = "fn"

// ParamInvalid is the KV key naming a declared value the validator
// refuses.
//
// The law reads two ways and each needs one. Forwards — a value the
// validator rejects must be refused by the call — needs the rejected
// value outright. Backwards — whatever the call accepted, the
// validator must accept too — binds without one and is worthless: a
// derived value the validator happens to accept answers nil whatever
// the call did, so the check engages, stays green with the screening
// deleted from the subject, and tests nothing. A check that cannot
// fail is worse than one that refuses to bind. Guessing does not
// close it either; the zero value is the obvious candidate, and a
// validator that accepts it turns the same check vacuous.
//
// A package-level var, so it resolves through the var scope and
// stamps qualified — see [shape.KindVar]. The value is the author's
// to construct because only they know what their validator refuses;
// what the directive contributes is the name.
//
// Optional, on the terms `pool`'s `stats` role is: a validated
// callable without one is still what the mixin names, and a law
// handing the value to the call simply does not bind.
const ParamInvalid = "invalid"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamFn, Kind: shape.KindCallable},
	{Key: ParamInvalid, Kind: shape.KindVar},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:   Name,
		Params: Params,
	}
}
