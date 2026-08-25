// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture exercises go.receiverIsPointer on methods. The
// converter stamps the flag whenever the receiver expression is
// `*T`; value-receiver methods on the same type carry no marker.
package fixture

// Counter holds a single tally. Two methods share the receiver
// type so the golden pins both the pointer-receiver positive case
// and the value-receiver negative case in one fixture.
type Counter struct {
	N int
}

// Bump increments the counter. Pointer receiver: stamps
// go.receiverIsPointer = true.
func (c *Counter) Bump() { c.N++ }

// Read returns the current count. Value receiver: must NOT stamp
// go.receiverIsPointer (its absence is the assertion).
func (c Counter) Read() int { return c.N }
