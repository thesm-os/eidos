// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture exercises type-set constraint interfaces. The
// converter stamps go.isConstraintInterface plus go.constraintTerms
// for each union-of-terms entry, with `~` rendered as an "approx"
// flag on the term.
package fixture

// Numeric is a type-set constraint: any signed or unsigned integer,
// including their named-type derivatives (the `~` prefix).
type Numeric interface {
	~int | ~int32 | ~int64 | ~uint | ~uint32 | ~uint64
}

// Stringish accepts string and any named type whose underlying type
// is string. Exercises a smaller union plus the approx marker.
type Stringish interface {
	~string
}
