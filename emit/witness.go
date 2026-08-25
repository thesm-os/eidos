// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import "go.thesmos.sh/eidos/core/meta"

// The authored witness: which concrete type a generated entry point
// instantiates a type parameter at.
//
// A language derives one where the constraint's type set is knowable
// without loading the package that declares it — `any` and
// `comparable` and little else. A parameter bounded by anything
// further is a reference into a package the generator never read, so
// no witness can be derived and every check over the declaration is
// withheld: a test function cannot take type parameters, so there is
// nothing to name the types it would run at.
//
// Naming the type is the way in, and it is the author's to name — the
// constraint says which types are admissible and only they know which
// one is representative.
//
// Stamped per parameter rather than per declaration, because that is
// what it is a fact about: a two-parameter declaration takes two
// answers, and a consumer reading a parameter has the bag in hand.
//
// Here rather than beside the annotator that writes it, for the reason
// [TypeInfo] is here: a language package cannot import the plugin
// façade, and the language is what reads these.

// The meta keys an authored witness is stamped under.
//
// Two, matching the shape an authored sample and a declared default
// both take: the symbol, and the package it resolves in. A single
// qualified string would need an encoding, and an encoding is a second
// thing to agree about.
//
//nolint:gochecknoglobals // meta key registration, immutable after init.
var (
	// MetaWitness names the type a parameter is instantiated at.
	MetaWitness = meta.EnsureKey("witness.type", meta.StringParser)

	// MetaWitnessPackage is the package that type resolves in, empty
	// for one needing no import.
	MetaWitnessPackage = meta.EnsureKey("witness.type.package", meta.StringParser)
)

// AuthoredWitness returns the type an author named for a parameter,
// and whether one was named.
func AuthoredWitness(bag *meta.Bag) (pkg, symbol string, ok bool) {
	return authored(bag, MetaWitness, MetaWitnessPackage)
}

// AuthoredWitnessRef returns the authored witness as a reference, and
// whether one was named.
//
// [Builtin] where no package was stamped and [External] otherwise,
// which is the distinction that decides whether the rendered file
// registers an import. A witness is written into an instantiation, so
// a qualified one that rendered as a bare name would name a package
// the file never imported.
func AuthoredWitnessRef(bag *meta.Bag) (Ref, bool) {
	pkg, symbol, ok := AuthoredWitness(bag)
	if !ok {
		return nil, false
	}
	if pkg == "" {
		return Builtin(symbol), true
	}
	return External(pkg, symbol), true
}
