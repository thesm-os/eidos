// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package notfound recognises the not-found mixin — the promise that
// a draw of an absent key reports the named sentinel, rather than
// answering the zero value.
//
// This is the reader's own miss sentinel, unconditioned. Every other
// sentinel in the vocabulary is scoped to a condition another shape
// owns — expiry ([ttl]), deletion ([deleteremoves]), rollback
// (`transaction`) — so a plain reader had no way to say what its
// misses report, and a consumer deriving a miss check had to assume
// the opposite: that an absent key answers the zero value.
//
// The condition-scoped sentinels stay where they are, because they
// name different facts that usually coincide: a store whose expired
// reads report differently from its missing ones declares both. A
// consumer wanting one sentinel for both reads the condition's own
// param first and falls back to this one.
//
// The sentinel is required, not optional — see the validator on
// [Mixin]. The recognised directive is:
//
//	//+gen:mixin notfound sentinel=ErrNotFound
package notfound
