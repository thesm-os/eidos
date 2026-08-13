// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package answeringwriter recognises a write that answers the stored
// state — one non-context parameter and two results, the first the
// parameter's own type beside an error.
//
// The recognised Go signature is:
//
//	func (r *Repo) Store(ctx context.Context, v V) (V, error)
//
// The shape [writer] documents as a receipt-returning write and
// deliberately refuses: that detector requires no non-error result, so
// this form fell to [reader] and had its parameter recorded as a key.
// A caller ordering writes by a store-assigned stamp needs the
// difference — a `(ctx, V) error` writer answers nothing, so the stamp
// dies with the call and the value the store actually kept is never
// observed.
//
// Drawn by a package-qualified parameter type equalling the first
// result type. Both halves are load-bearing. Identity alone takes every
// coincidentally-typed read with it — a string-keyed string cache, a
// codec's transformers, a classifier, a loader — none of which answers
// a stored state; the qualifier separates them, since a predeclared
// type carries no package and the state this detector exists for is a
// declared type carrying identity and stamps.
//
// What it still takes from [reader] is a read keyed by a struct and
// returning that same struct. That shape is genuinely undecidable from
// the signature alone, and [shape.DirectiveName] pins it where it
// occurs.
//
// Named for what it does rather than `upserter`, which is already a
// contract in this vocabulary — a writer paired with a reader under
// last-write-wins. The two usually co-occur and are not the same fact:
// this is a signature, that is a protocol.
//
// A positive detection stamps:
//
//	shape      = "answeringwriter"
//	value_type = the parameter's qualified type
package answeringwriter
