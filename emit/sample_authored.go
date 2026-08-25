// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import "go.thesmos.sh/eidos/core/meta"

// The authored half of [Sample].
//
// A language derives what a value of a type looks like, and for most
// types that is the whole answer. For some it is no answer at all: a
// declaration whose values have rules the structure does not state, and
// — the case with no way round it — an interface, which has no literal
// form, so a member of one loses every check that needed a value.
//
// Naming the function that produces one is the way in. It is stamped
// here rather than read by each consumer, because the consumers are
// whoever asks a language for a sample, and a stamp each of them has to
// remember to prefer is a stamp two of them will forget.
//
// Here rather than beside the annotator that writes it, for the reason
// [TypeInfo] is here: a language package cannot import the plugin
// façade, and the language is what reads these.

// The meta keys an authored sample is stamped under.
//
// Two per value, matching the shape a declared default takes: the
// symbol, and the package it resolves in. A single qualified string
// would need an encoding, and an encoding is a second thing to agree
// about.
//
//nolint:gochecknoglobals // meta key registration, immutable after init.
var (
	// MetaSample and MetaSamplePackage name the function producing a
	// value of the annotated type.
	MetaSample        = meta.EnsureKey("sample.value", meta.StringParser)
	MetaSamplePackage = meta.EnsureKey("sample.value.package", meta.StringParser)

	// MetaAlternate and MetaAlternatePackage name the function
	// producing a second value, distinct from the first.
	//
	// Separate from the pair above because a type may need one and not
	// the other: a derived first value is often fine where the second
	// has to differ from it in a way the derivation cannot know.
	MetaAlternate        = meta.EnsureKey("sample.alternate", meta.StringParser)
	MetaAlternatePackage = meta.EnsureKey("sample.alternate.package", meta.StringParser)
)

// AuthoredSample returns the function an author named for the type's
// value, and whether one was named.
//
// The package is always stamped beside the symbol, including where the
// function sits in the declaring package's own file: a consumer
// rendering the call has to register an import wherever it lands, and
// which package that is cannot be known when the stamp is written.
func AuthoredSample(bag *meta.Bag) (pkg, symbol string, ok bool) {
	return authored(bag, MetaSample, MetaSamplePackage)
}

// AuthoredAlternate returns the function an author named for the
// type's second value, and whether one was named.
func AuthoredAlternate(bag *meta.Bag) (pkg, symbol string, ok bool) {
	return authored(bag, MetaAlternate, MetaAlternatePackage)
}

// authored reads one symbol-and-package pair.
func authored(bag *meta.Bag, symbolKey, pkgKey meta.Key[string]) (pkg, symbol string, ok bool) {
	if bag == nil {
		return "", "", false
	}
	symbol, ok = symbolKey.Get(bag)
	if !ok || symbol == "" {
		return "", "", false
	}
	pkg, _ = pkgKey.Get(bag)
	return pkg, symbol, true
}

// AuthoredSampleOf returns the value an author named as a [Sample],
// and whether one was named.
//
// A call expression rather than a reference: what was named is a
// function, and a consumer writing its identifier where a value
// belongs emits the function itself. [Sample.Expr] carries it, which
// is the field's purpose — a value no type-and-text pair can spell.
func AuthoredSampleOf(bag *meta.Bag) (Sample, bool) {
	pkg, symbol, ok := AuthoredSample(bag)
	if !ok {
		return Sample{}, false
	}
	return Sample{Expr: NewCall(NewExternal(pkg, symbol))}, true
}

// AuthoredAlternateOf returns the second value an author named as a
// [Sample], and whether one was named.
func AuthoredAlternateOf(bag *meta.Bag) (Sample, bool) {
	pkg, symbol, ok := AuthoredAlternate(bag)
	if !ok {
		return Sample{}, false
	}
	return Sample{Expr: NewCall(NewExternal(pkg, symbol))}, true
}
