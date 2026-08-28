// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package batchwriter

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "batch-writer"

// RoleWriter is the callable writing a batch.
const RoleWriter = "writer"

// RoleReader is the observation partner confirming what a batch
// left behind. Optional — see [Roles] for why.
const RoleReader = "reader"

// Roles enumerates the contract's role vocabulary.
//
// `reader` names the observation partner that confirms what a batch
// left behind — the same pairing persister and upserter require, made
// optional here: a batch writer without one is still a batch writer,
// and `mode=all-or-nothing` simply becomes a claim a consumer
// declines to check rather than guesses at. A best-effort writer
// never needs it, which is what makes the role optional rather than
// merely unenforced.
//
// A role rather than a `read=` param, though deleteremoves spells the
// same pairing as a param: a mixin has no role vocabulary to put a
// partner in, and a contract does. The role form back-stamps, so a
// consumer holding the reader finds the batch-writer it confirms —
// a param records the pairing on the writer alone.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleWriter, RoleReader}

// ParamMode is the KV key naming how a partial batch behaves — the
// word a consumer reads to decide whether a failed entry aborts the
// rest.
//
// Opaque: the vocabulary belongs to whoever generates against it.
const ParamMode = "mode"

// ParamRefused is the KV key naming a declared value the writer
// turns down.
//
// The other half of what the `reader` role supplies. `mode=atomic`
// says a refused write leaves nothing behind, and checking that
// takes three steps: write a value the subject refuses, read the key
// back, require it absent. The role gave the check its second and
// third steps; nothing gave it the first, because a derived draw is
// built to be accepted — it is what every other claim writes — so a
// check using one never reaches the failure the mode is about and
// asserts only that a good write succeeds.
//
// A package-level var of [shape.KindVar], on the terms the validates
// mixin's `invalid=` is, and scoped to the writer: the reader
// refuses nothing. Only the author knows what their writer turns
// down; the directive contributes the name. Optional — the bare form
// still classifies, and a consumer that cannot state the law without
// it declines to state it, recorded as such rather than assumed.
const ParamRefused = "refused"

// Params enumerates the directive's KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamMode, Kind: shape.KindOpaque},
	{Key: ParamRefused, Kind: shape.KindVar, Role: RoleWriter, Counterexample: true},
}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles, Params: Params}
}
