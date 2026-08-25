// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package blog

// Status enumerates the publication states an Article moves through.
// The typed-iota declaration exercises the backend's enum-rendering
// path (typed underlying + iota promotion) on a fixture type that
// other fixture entities reference. The `+gen:enum` directive opts
// the type in for the production enum plugin's textual surface —
// String, ParseStatus and its refusal, StatusValues, IsValid and the
// MarshalText / UnmarshalText pair — and a paired file of checks.
//
// The variants deliberately carry no `Status` prefix. Stripping one
// is the rule for a numeric enum, and a set that has none exercises
// the case where the rule finds nothing to strip.
//
//+gen:enum
type Status int

const (
	// Draft indicates the article is still being written.
	Draft Status = iota

	// Published indicates the article is live.
	Published

	// Archived indicates the article is no longer current.
	Archived
)
