// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package runtimelib stands in for a generator's runtime library — the
// module generated output imports and a test therefore has to make
// importable. Its contents matter only in that a generated file can
// reference them.
package runtimelib

// Marker returns a value generated code can assign, so an import of
// this package is load-bearing rather than blank.
func Marker() string { return "runtimelib" }
