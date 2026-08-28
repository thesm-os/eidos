// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pagination

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "pagination"

// RoleReader is the callable answering one page.
const RoleReader = "reader"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleReader}

// ParamCursor is the KV key naming the field carrying the position a
// page resumes from.
//
// [shape.KindValueField]: the resolver checks the name against the
// fields of the type the reader answers and rewrites a hit into the
// qualified form. It was opaque, which named the field and left it
// unreachable — the same half-measure `if-match`'s `field=` was, and
// with the same cost. What a paginated reader owes is continuation:
// a page fetched with the cursor the previous page answered carries
// on rather than repeating. Reaching step two of that check means
// selecting the member off the answered value, and an opaque string
// can be neither selected nor typed, so the claim degrades to "the
// reader answers twice" — which a subject that ignores the cursor
// and serves page one forever satisfies.
//
// Still the author's to name. A page may carry several string
// members and only they know which one the protocol cursors on;
// what the kind adds is that the name is now checked and reachable.
const ParamCursor = "cursor"

// Params enumerates the directive's KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamCursor, Kind: shape.KindValueField},
}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles, Params: Params}
}
