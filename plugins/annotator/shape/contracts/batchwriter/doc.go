// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package batchwriter recognises the batch-writer contract — a
// writer accepting a batch of records with a configured failure
// mode. The `mode` param is opaque (typically `all-or-nothing`
// or `best-effort`); the resolver leaves the value unchanged.
//
// `all-or-nothing` is an assertion about state after a failure —
// one bad record leaves nothing behind — and checking it needs a
// way to read "nothing behind". The optional `reader` role names
// it. Without one the mode is a claim a consumer declines to
// check, recorded as such rather than assumed.
//
// The recognised directives are:
//
//	//+gen:contract batch-writer role=writer mode=all-or-nothing reader=Get
//	//+gen:contract batch-writer role=writer mode=best-effort
package batchwriter
