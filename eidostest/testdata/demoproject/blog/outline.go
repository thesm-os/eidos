// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package blog

// Outline is the redirection fixture: its builder is routed out of the
// blog package entirely, into blog/build as package outlinebuild.
//
// The combination is what makes it load-bearing. A redirected builder
// exercises three things no in-place fixture reaches:
//
//   - The generated file declares a package name that differs from
//     its own directory, so any reference into it needs an explicit
//     import alias rather than the one derived from the path.
//   - Every reference back to this package — the built type, its
//     field types — must acquire a qualifier the in-place case elides.
//   - `defaults=` names a factory beside the annotated struct with a
//     bare identifier, which only compiles from another package if
//     the generator resolves it to a package rather than emitting it
//     verbatim.
//
// OutlineDefaults is exported precisely because the builder moves. An
// unexported factory would be unreachable from the redirected
// package, which is a legitimate authoring error the compiler
// reports — but not one this fixture exists to demonstrate.
//
// +gen:builder defaults=OutlineDefaults out=build/ pkg=outlinebuild
type Outline struct {
	// Title is the working headline.
	Title string

	// Tags classify the draft. A named slice element type forces the
	// redirected builder to qualify the element, not just the struct.
	Tags []OutlineLabel

	// WordCount tracks length.
	WordCount int
}

// OutlineLabel is a package-local named string used as a slice element on
// Outline, so the redirected builder has to qualify a type it would
// otherwise render bare.
type OutlineLabel string

// OutlineDefaults seeds the generated NewDraftWithDefaults constructor.
// Named by the `defaults=` key on Outline's builder directive.
func OutlineDefaults() Outline {
	return Outline{Title: "untitled", WordCount: 0}
}
