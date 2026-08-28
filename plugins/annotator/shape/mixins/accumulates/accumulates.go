// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package accumulates

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "accumulates"

// ParamObserve is the KV key naming the read the effect is counted
// through.
//
// The claim is that N calls have N observable effects, and
// `observable` is the load-bearing word. [idempotent]'s probe
// settles its own claim without looking at anything — a second
// identical call is accepted rather than refused, which the call's
// error answers — so inverting that assertion gives "the second call
// is refused", a different sentence from this one. A callable whose
// effect is out of band answers the same error twice whether it
// compounded or coalesced, so a check that only calls twice binds,
// goes green against a subject that silently deduplicates, and tests
// nothing.
//
// A different axis from the count, which stays the check's own
// choice — see [Mixin]. How many times to call is arithmetic any
// caller can do; where to look afterwards is a sibling only the
// author can pick out, and reading the wrong one asks about a value
// the calls never touched, where a compounding subject and a
// coalescing one report the same nothing.
//
// [shape.KindCallable], resolved through the host's own scope like
// `sideeffect`'s `observe=` and `atomic`'s `read=`, which name their
// observers against the same gap. Optional: a compounding callable
// without one is still what the mixin names, and the counting law
// simply does not bind.
//
// [idempotent]: go.thesmos.sh/eidos/plugins/annotator/shape/mixins/idempotent
const ParamObserve = "observe"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamObserve, Kind: shape.KindCallable},
}

// Mixin returns the [shape.Mixin] this package contributes.
//
// One param, and the contrast with retrysucceeds' `attempts=` still
// holds on the axis it was about: an accumulation observed at any N
// of calls proves this claim, so the count is the check's choice —
// while a convergence bound is part of what a retrysucceeds author
// asserts, and so must be declared. What [ParamObserve] adds is not
// a count but the observation, which no check can choose for itself.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
