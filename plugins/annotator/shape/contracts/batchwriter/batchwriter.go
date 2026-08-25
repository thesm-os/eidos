// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package batchwriter

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "batch-writer"

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
var Roles = []string{"writer", "reader"}

// ParamMode is the KV key naming how a partial batch behaves — the
// word a consumer reads to decide whether a failed entry aborts the
// rest.
//
// Opaque: the vocabulary belongs to whoever generates against it.
const ParamMode = "mode"

// Params enumerates the directive's opaque KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamMode, Kind: shape.KindOpaque},
}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles, Params: Params}
}
