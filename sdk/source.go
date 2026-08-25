// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

// SourceRules is how declarations written in one language are read.
//
// The read-side half of [LanguageSupport]. A plugin declaring support
// for a language is declaring two things that travel together: how to
// render that language, and how to read declarations written in it. A
// generator uses the first, an annotator the second, and a plugin
// doing both uses one declaration for both.
//
// # Which language a caller asks about
//
// Language names are one namespace. A frontend stamps the name of the
// language it parsed onto every package it produces, and a backend
// answers to the same name — so `"golang"` means Go on both sides.
// What differs is which language a caller looks up:
//
//   - An annotator asks about the language of the package in front of
//     it, through [LanguageOf].
//   - A generator or backend asks about the language the run renders.
//
// In a run parsing Go and rendering TypeScript those give different
// answers out of the same declaration, which is the point. A plugin
// that looked up the render language to read a Go struct tag would be
// reading source with the wrong language's rules, and both keys are
// strings, so nothing would report it.
//
// # Why an interface and not a struct of funcs
//
// [LanguageSupport]'s other fields are data the pipeline reads.
// These are questions with answers a language package already
// implements, so an interface lets `lang/golang` satisfy it with the
// methods it has rather than restating each as a field. The interface
// is deliberately small: a question only one plugin asks belongs in
// that plugin's own language binding, not here, because a method
// added here has to be answered by every language that already
// implements it.
type SourceRules interface {
	// ResolveValue splits a value written in a directive or a tag into
	// the package it names and the symbol within it, reporting an
	// empty package for a plain literal.
	//
	// The file supplies whatever scope a qualifier resolves against,
	// and may be nil for a positionless declaration — which reports an
	// error rather than guessing against some other file's imports.
	ResolveValue(f *File, value string) (pkg, symbol string, err error)

	// Tag returns the named tag entry on a field and whether the field
	// carried it. A language without tags answers false.
	Tag(f *Field, key string) (string, bool)

	// FileOf returns the file within pkg that declared n, or nil when
	// the run recorded none.
	//
	// The step from a declaration to the imports in scope for it. A
	// position carries a path while a package keys its files by
	// basename, so a lookup composed at a call site is one truncation
	// away from always missing — silently, which reads as source that
	// imports nothing.
	FileOf(pkg *Package, n Node) *File
}
