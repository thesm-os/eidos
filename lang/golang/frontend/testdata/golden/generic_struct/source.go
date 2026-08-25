// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture exercises generic type parameters and their
// embedded-bound constraints.
package fixture

// Stringer mirrors fmt.Stringer for the bound below.
type Stringer interface {
	String() string
}

// Box is a generic container constrained by Stringer.
type Box[T Stringer] struct {
	Value T
}
