// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture exercises interface conversion with explicit
// methods (params + returns) plus an embedded interface.
package fixture

// Closer mirrors io.Closer to keep the fixture self-contained.
type Closer interface {
	// Close releases any resources the implementation holds.
	Close() error
}

// Reader is a small interface that embeds Closer and adds one
// method with a non-trivial signature.
type Reader interface {
	Closer

	// Read fills p and returns the number of bytes read plus an
	// optional error.
	Read(p []byte) (n int, err error)
}
