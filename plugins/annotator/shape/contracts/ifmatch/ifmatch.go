// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ifmatch

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "if-match"

// RoleWriter is the conditional write the contract is declared on.
const RoleWriter = "writer"

// RoleMatch is the callable deciding whether the write proceeds.
//
// Distinct from [ParamPred], which spells the same decision as an
// expression the resolver must not touch. The two cannot share a key:
// a value that is qualified and one that is left verbatim are
// different treatments, and a single key would leave the resolver
// guessing which it received.
const RoleMatch = "match"

// ParamPred is the KV key carrying the predicate as an expression.
//
// Opaque by design — `pred=Version==Expected` names no callable, so
// there is nothing for the resolver to look up and every attempt would
// report a correct directive as unresolved.
const ParamPred = "pred"

// ParamField is the KV key naming the member of the written value the
// predicate judges — the one a second write may differ in.
//
// The law is that a write is refused where the predicate turns its
// value down, and witnessing that takes two values that differ: one
// value written twice succeeds both times, because a store holding
// what it was given matches it. Which member the two may differ in is
// the author's knowledge alone — a keyed record's `Key` and `Body`
// are two strings, and varying the key writes a different record
// rather than a rejected one, so a derivation that guesses wrong goes
// quietly vacuous instead of failing.
//
// [shape.KindValueField], the mirror of `cas`'s `version=`: that one
// names the member the write's expectation rides, this one the member
// the predicate is about. The resolver checks the name against the
// written value's fields — pointer-stripped, promotion honoured — and
// rewrites a hit into the qualified form, so a typo is reported where
// the author is.
//
// Optional, on the terms `pool`'s `stats` role is: a conditional
// writer without one is still what this contract names, and a law
// varying the member simply does not bind.
const ParamField = "field"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleWriter, RoleMatch}

// Params enumerates the directive's KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamField, Kind: shape.KindValueField},
	{Key: ParamPred, Kind: shape.KindOpaque},
}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles, Params: Params}
}
