// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package ifmatch recognises the if-match conditional-writer
// contract — a writer that succeeds only when the existing record
// matches the supplied predicate.
//
// The predicate has two spellings, and which one is used decides how
// the value is treated. The `pred` param carries it as an expression
// and stays opaque: there is no callable to look up, so the resolver
// leaves it verbatim. The `match` role names a callable that decides
// the same thing, and is resolved like any other partner — qualified,
// reported when it names nothing in scope, and back-stamped onto the
// predicate so the pair is navigable from either side.
//
// They are separate keys rather than one because the treatments are
// incompatible. A single key carrying either form would leave the
// resolver deciding whether `Match` is a method or an expression that
// happens to be one identifier long, and guessing is what the role
// vocabulary exists to remove.
//
// `field=` names the member of the written value the predicate
// judges — the one a second write may differ in. The law's witness
// is two values that differ, and on a keyed record only the author
// knows which member rejection rides on: varying the key writes a
// different record rather than a rejected one, so a check that
// guesses wrong goes quietly vacuous. Optional — a writer without it
// is still what the contract names, and a law varying the member
// simply does not bind.
//
// The recognised directives are:
//
//	//+gen:contract if-match role=writer pred=Version==Expected
//	//+gen:contract if-match role=writer match=Match field=Body
package ifmatch
