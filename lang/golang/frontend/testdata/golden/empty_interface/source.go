// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture exercises empty-interface detection. A bare
// `interface{}` declaration must carry go.isEmptyInterface on the
// resulting [node.Interface]; an interface with explicit methods
// must NOT.
package fixture

// Any is the classic empty interface — accepted by anything,
// constrains nothing. Drives the go.isEmptyInterface stamping path.
type Any interface{}

// Doer has at least one explicit method so the converter does not
// stamp go.isEmptyInterface. Included so the golden pins the
// negative case alongside the positive one.
type Doer interface {
	Do() error
}
