// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package batchwriter recognises the batch-writer contract — a
// writer accepting a batch of records with a configured failure
// mode. The `mode` param is opaque (typically `all-or-nothing`
// or `best-effort`); the resolver leaves the value unchanged.
//
// `all-or-nothing` is an assertion about state after a failure —
// one bad record leaves nothing behind — and checking it needs two
// things: a way to read "nothing behind", and a failure to leave it.
// The optional `reader` role names the first. `refused=` names the
// second — a declared value the writer turns down, since a derived
// draw is built to be accepted and never reaches the failure the
// mode is about. Without either half the mode is a claim a consumer
// declines to check, recorded as such rather than assumed.
//
// The recognised directives are:
//
//	//+gen:contract batch-writer role=writer mode=all-or-nothing reader=Get refused=BadEntry
//	//+gen:contract batch-writer role=writer mode=best-effort
package batchwriter
