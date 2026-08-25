// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture exercises the constraint shapes the converter
// resolves through a different arm than a plain named bound.
//
// `generic_struct` already covers the common case — a type
// parameter bound by a named interface that declares methods. That
// interface embeds nothing, so the converter's embedded-type walk
// never runs for it. The declarations here reach the remaining
// arms: an embedded named interface, an approximation term written
// directly on the parameter, an inline union, and the bare `any`
// bound.
package fixture

// Stringer is the bound the embedding constraint below reuses.
type Stringer interface {
	String() string
}

// Printable is a constraint interface whose only content is an
// embedded named interface — the shape that drives the converter's
// embedded-type-ref arm.
type Printable interface {
	Stringer
}

// Boxed is bound by a constraint that embeds another interface
// rather than declaring methods of its own.
type Boxed[T Printable] struct {
	Value T
}

// Approx writes an approximation term directly on the type
// parameter rather than behind a named constraint interface.
type Approx[T ~int] struct {
	Value T
}

// Unioned writes a union directly on the type parameter.
type Unioned[T int | string] struct {
	Value T
}

// Anything uses the predeclared `any` bound, which resolves to an
// empty constraint carrying no terms.
type Anything[T any] struct {
	Value T
}
