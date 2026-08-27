// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package eventually recognises the eventually mixin — the
// assertion that the annotated callable's effect is observable
// eventually rather than immediately (eventual consistency).
//
// The law is "settle, then observe", and it needs all three parts
// named: `settle=` drives convergence, `sync=` reports whether it has
// arrived, and `observe=` is the read whose answer the effect is
// expected to reach. The first two are both about getting to
// quiescence — without the third the sentence has no observation, so a
// subject naming only them states the property and derives no check
// from it.
//
// All three are optional. A publisher whose effect arrives eventually
// is what this classifies whether or not a partner is named; a law
// reading one simply does not bind.
//
// The recognised directives are:
//
//	//+gen:mixin eventually
//	//+gen:mixin eventually settle=Settle sync=Synced observe=Items
package eventually
