// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package serializable recognises the serializable mixin — the
// assertion that concurrent transactions are equivalent to some serial
// order, so no anomaly is observable at all.
//
// Distinct from [snapshotisolation], and deliberately a sibling rather
// than a level on it. Snapshot isolation prevents dirty writes, dirty
// reads and read skew, and *permits write skew*: two transactions each
// reading what the other is about to invalidate, both committing. That
// permission is the model's defining property and the whole reason a
// system wanting serializability asks for more than a snapshot.
//
// So the two are different claims rather than one claim with a knob. A
// store is not snapshot-isolated at level serializable, and a `level=`
// param would make the mixin's name contradict its own documentation.
//
// The distinction is load-bearing downstream: an anti-dependency-cycle
// check is correct against this claim and wrong against a snapshot,
// where the cycle it looks for is permitted. Selecting one checker
// from the other classification reddens a correct store, with no
// escape — declaring the claim would be declaring the check that
// contradicts it.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin serializable
package serializable
