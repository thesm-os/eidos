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
// The check this licenses is the idempotence probe with the
// assertion inverted — call twice, observe the effect twice — and
// needs nothing that probe does not already have.
//
// The recognised directive is:
//
//	//+gen:mixin accumulates
//
// [idempotent]: go.thesmos.sh/eidos/plugins/annotator/shape/mixins/idempotent
package accumulates
