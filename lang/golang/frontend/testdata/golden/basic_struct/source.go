// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture is a golden-file fixture for the frontend's
// integration tests. The source intentionally stays minimal so the
// expected JSON is small enough to review by eye.
package fixture

// User is a basic struct with one named field.
type User struct {
	// Name is the user's display name.
	Name string
}
