// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture exercises channel-direction metadata. The
// converter stamps go.isChannel plus go.chanDir and go.chanElem
// onto every chan-typed reference; bidirectional, send-only, and
// recv-only channels each carry a distinct direction string.
package fixture

// Pipes bundles all three channel directions so the golden pins
// the full set without spreading the assertion across multiple
// fixtures.
type Pipes struct {
	// Both is a bidirectional channel of ints.
	Both chan int
	// Out is a send-only channel of strings.
	Out chan<- string
	// In is a recv-only channel of booleans.
	In <-chan bool
}
