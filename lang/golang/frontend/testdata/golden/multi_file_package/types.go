// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture spans more than one .go file so the converter's
// multi-file package assembly is exercised. types.go declares the
// shared types; ops.go defines methods that operate on them. The
// resulting node.Package must merge declarations from both files
// into a single, deterministic package node.
package fixture

// User is the domain object referenced by helpers across the package.
type User struct {
	ID   int
	Name string
}

// Repo is a tiny in-memory collection of users.
type Repo struct {
	Users []User
}
