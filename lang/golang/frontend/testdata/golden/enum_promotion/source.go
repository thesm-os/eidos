// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture exercises the typed-iota → Enum promotion path,
// including iota-value stamping on each variant.
package fixture

// Status is the named type the const block below promotes to an
// [node.Enum].
type Status int

const (
	// StatusActive is the live state.
	StatusActive Status = iota
	// StatusPaused is the temporarily-suspended state.
	StatusPaused
	// StatusArchived is the terminal state.
	StatusArchived
)
