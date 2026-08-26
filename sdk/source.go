// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import (
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// SourceRules re-exports [plugin.SourceRules] — what a generator needs
// to know about the language a declaration was written in.
//
// The read-side counterpart to the template and filename providers,
// which answer for the language a run renders. A plugin declares both
// halves together through [Builder.For] and reads this one back
// through [Base.Source], asking about the language of the package in
// front of it — see [LanguageOf].
type SourceRules = plugin.SourceRules

// EnumRules re-exports [plugin.EnumRules] — what a language answers
// about a declared enumeration.
//
// A capability rather than a requirement: a plugin asks for it by
// asserting against the rules [Base.SourceOf] handed back, and a
// language that does not describe enumerations simply does not
// satisfy it.
type EnumRules = plugin.EnumRules

// ErrorRules re-exports [plugin.ErrorRules] — what a language answers
// about its error protocol. Optional, on the same terms as
// [EnumRules].
type ErrorRules = plugin.ErrorRules

// SigRules re-exports [plugin.SigRules] — what a language answers
// about a callable's signature, for a generator that renders one.
// Optional and found by assertion, like [EnumRules] and [ErrorRules].
type SigRules = plugin.SigRules

// The three halves [SourceRules] is composed from.
//
// Re-exported because a helper does not always want the whole of it: a
// function that only projects types takes [TypeRules] and states in
// its signature that it reads nothing else, which is the narrower
// contract and the more useful one to a reader. Naming that half was
// the one thing left that sent a caller past this package into
// [plugin] — everything else there is already aliased here.
//
// [SourceRules] stays the name a plugin declares and reads back
// through [Base.SourceOf]; these are for the functions it hands the
// result to.
type (
	// DeclarationRules is what a language answers about a declaration
	// and the file it was written in.
	DeclarationRules = plugin.DeclarationRules

	// TypeRules is what a language answers about a type: its shape, a
	// sample value of it, and how a type parameter is spelled.
	TypeRules = plugin.TypeRules

	// NamingRules is how a language spells a derived name.
	NamingRules = plugin.NamingRules
)

// SigInfo re-exports [emit.SigInfo], the projection [SigRules.SigOf]
// answers with.
//
// Carried under the emit prefix like every other output-model alias
// here: a projection built against a source shape never renders, and
// the two fail silently when confused.
type SigInfo = emit.SigInfo

// SigParam re-exports [emit.SigParam], one parameter of a [SigInfo].
type SigParam = emit.SigParam

// SigReturn re-exports [emit.SigReturn], one return slot of a
// [SigInfo].
type SigReturn = emit.SigReturn

// EnumForm re-exports [emit.EnumForm] — where a variant's textual
// form comes from.
type EnumForm = emit.EnumForm

// The two forms, re-exported so a plugin branching on one need not
// import the emit package to name it.
const (
	EnumFormIdentifier = emit.EnumFormIdentifier
	EnumFormValue      = emit.EnumFormValue
)

// EnumInfo re-exports [emit.EnumInfo] — what a language answers about
// one declared enumeration.
type EnumInfo = emit.EnumInfo

// EnumText re-exports [emit.EnumText] — one variant's identifier and
// the literal its textual form renders as.
type EnumText = emit.EnumText

// ErrorInfo re-exports [emit.ErrorInfo] — what a language answers
// about one declaration taking part in its error protocol.
type ErrorInfo = emit.ErrorInfo

// ErrorMember re-exports [emit.ErrorMember] — one member of an error
// declaration, with what a check can write into it.
type ErrorMember = emit.ErrorMember

// Resolver re-exports [node.Resolver] — what a named type is, over
// the declarations a run loaded.
//
// An alias rather than a restatement. Two named interfaces with
// identical methods are still two types, so a language helper written
// against its own spelling would not satisfy a contract written
// against a second one, however identical they read. [StoreReader]
// satisfies it, so a plugin passes the reader it was handed and
// writes no adapter.
type Resolver = node.Resolver

// TypeShape re-exports [emit.TypeShape] — how a declared type is
// structured, in terms every language shares.
type TypeShape = emit.TypeShape

// The shapes, re-exported so a plugin branching on one need not
// import the emit package to name it.
const (
	ShapeScalar   = emit.ShapeScalar
	ShapeSequence = emit.ShapeSequence
	ShapeBytes    = emit.ShapeBytes
	ShapeMapping  = emit.ShapeMapping
	ShapeSet      = emit.ShapeSet
	ShapeOptional = emit.ShapeOptional
)

// TypeInfo re-exports [emit.TypeInfo] — what a language answers about
// one declared type.
type TypeInfo = emit.TypeInfo

// Member re-exports [emit.Member] — one settable part of a
// declaration.
type Member = emit.Member

// Sample re-exports [emit.Sample] — a value a generated check writes,
// together with the type it is written against.
type Sample = emit.Sample

// SampleRefusal re-exports [emit.SampleRefusal] — why a sample could
// not be derived.
type SampleRefusal = emit.SampleRefusal

// The meta keys an authored sample value is stamped under,
// re-exported so the annotator that writes them need not import the
// emit package to name them.
//
// Read by the language rather than by a generator: [SourceRules]
// prefers an authored value over the one it would derive, so a
// consumer asking for a sample gets it without knowing these exist.
//
//nolint:gochecknoglobals // re-exported meta key registrations.
var (
	MetaSample           = emit.MetaSample
	MetaSamplePackage    = emit.MetaSamplePackage
	MetaAlternate        = emit.MetaAlternate
	MetaAlternatePackage = emit.MetaAlternatePackage

	// The witness a type parameter is instantiated at, read by
	// [SourceRules.Witnesses] on the same terms: authored first,
	// derived where nothing was authored.
	MetaWitness        = emit.MetaWitness
	MetaWitnessPackage = emit.MetaWitnessPackage
)

// The accessors over those keys, re-exported for a plugin that reads
// back what it stamped.
//
// A generator has no reason to call these: the language prefers an
// authored value over the one it derives, so asking for a sample is
// how a consumer gets one. They are here for the annotator's own
// tests, and for a plugin that reports on what a declaration carries.
//
//nolint:gochecknoglobals // re-exported function values.
var (
	AuthoredSample      = emit.AuthoredSample
	AuthoredAlternate   = emit.AuthoredAlternate
	AuthoredSampleOf    = emit.AuthoredSampleOf
	AuthoredAlternateOf = emit.AuthoredAlternateOf
	AuthoredWitness     = emit.AuthoredWitness
	AuthoredWitnessRef  = emit.AuthoredWitnessRef
)

// The refusal reasons, re-exported so a plugin reading one need not
// import the emit package to name it.
const (
	RefusedNone       = emit.RefusedNone
	RefusedNoResolver = emit.RefusedNoResolver
	RefusedDepth      = emit.RefusedDepth
	RefusedUnresolved = emit.RefusedUnresolved
	RefusedNoLiteral  = emit.RefusedNoLiteral
)
