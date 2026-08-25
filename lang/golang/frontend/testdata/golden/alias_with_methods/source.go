// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture exercises methods on a named-type alias —
// `type Seconds int` carries a method set that the converter must
// route to the Alias node (not orphan).
package fixture

// Seconds is a basic-underlying named type.
type Seconds int

// Mul scales the receiver by by and returns the result. Demonstrates
// a value-receiver method on a non-struct named type.
func (s Seconds) Mul(by int) Seconds { return s * Seconds(by) }
