// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package accumulates recognises the accumulates mixin — the
// assertion that repeated invocations compound: N calls have N
// observable effects, and a subject that coalesced them would be
// wrong.
//
// The second position on the effect axis, beside [idempotent] — two
// claims, not a claim and its negation. A callable carrying neither
// has not been considered; only a stamped position is a contract, and
// the two states look identical from outside until one is written.
// Do not reach for a negated `idempotent`: a mixin appears only where
// someone wrote the directive, so absence already means "not
// claimed", and the directive denies negation for exactly that
// reason.
//
// The check this licenses reads the effect, adds twice, and reads
// again: N calls, N effects. That is not the idempotence probe with
// its assertion inverted, though it was described that way here
// until someone tried to write it — inverting that probe gives "the
// second call is refused", which is a different claim. A callable
// whose effect is out of band answers the same error twice whether
// it compounded or coalesced, so a check that only calls twice binds
// and cannot fail.
//
// `observe=` names the read the effect is counted through. Optional,
// and without it the law does not bind — which is the honest
// outcome, since the observation is a sibling only the author can
// pick out.
//
// The recognised directives are:
//
//	//+gen:mixin accumulates
//	//+gen:mixin accumulates observe=Total
//
// [idempotent]: go.thesmos.sh/eidos/plugins/annotator/shape/mixins/idempotent
package accumulates
