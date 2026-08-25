// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package snapshotisolation recognises the snapshotisolation mixin — the assertion that
// a transaction observes one consistent snapshot, so no dirty write, dirty read or read skew is visible.
//
// Write skew is permitted and that is the point of the model, not an
// omission from it: two transactions each reading what the other is
// about to invalidate may both commit. A store ruling that out as well
// claims [serializable] instead, which is a different assertion rather
// than a stronger setting of this one.
//
// The distinction decides which anomaly checks apply. An
// anti-dependency-cycle check is correct against serializability and
// wrong here, where the cycle it looks for is legal — so a checker
// selected from this classification reddens a correct store.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin snapshotisolation
//
// `read=` names the callable a check observes through. Each anomaly
// this model rules out — and the write skew it deliberately permits
// — is defined by what a read sees, so naming it is what separates
// an isolation check from a concurrency smoke test.
//
// It does not conjure transaction boundaries: an interface with no
// begin/commit pair still cannot express a multi-statement
// transaction, and a consumer needing those reads them from the `tx`
// contract's roles.
package snapshotisolation
