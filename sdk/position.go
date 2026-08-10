// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import "go.thesmos.sh/eidos/core/position"

// Source positions — what makes a diagnostic point at a line the
// user can open.

// Pos identifies a byte location in a source file. Every node and
// every emit value carries one; a plugin reporting a problem
// passes the offending declaration's `Pos()` rather than the zero
// value, which is the difference between a diagnostic the user can
// act on and one they cannot.
//
// Value type — copy freely. The zero value means "no position
// known" and renders as an unpositioned diagnostic.
type Pos = position.Pos

// Range spans two [Pos] values, Start inclusive and End
// exclusive. Construct through [NewRange], which rejects a span
// crossing files rather than letting it render as a nonsensical
// location.
type Range = position.Range

// Position constructors. A plugin normally propagates a position
// it was given; these are for the frontend recording one and for
// the generator that has no source line to point at.
//
//nolint:gochecknoglobals // alias re-exports of stable factories.
var (
	// At builds a position from file, line, and column.
	At = position.At

	// AtOffset adds the byte offset, for tooling that needs to
	// slice the original file.
	AtOffset = position.AtOffset

	// Synthetic marks a position as belonging to generated
	// output rather than to a source file. Use it instead of the
	// zero value for a purely generated artifact: the zero value
	// says "unknown", this says "there is no source line, by
	// construction".
	Synthetic = position.Synthetic

	// NewRange builds a validated [Range].
	NewRange = position.NewRange
)
