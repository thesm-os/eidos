// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package consumer stands in for a real module a generator's output
// is built inside, for the case where that output references
// third-party packages a bare temp module could not resolve.
package consumer

// Helper is the hand-written symbol the generated file below calls.
func Helper() int { return 7 }
