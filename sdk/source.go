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

// The refusal reasons, re-exported so a plugin reading one need not
// import the emit package to name it.
const (
	RefusedNone       = emit.RefusedNone
	RefusedNoResolver = emit.RefusedNoResolver
	RefusedDepth      = emit.RefusedDepth
	RefusedUnresolved = emit.RefusedUnresolved
	RefusedNoLiteral  = emit.RefusedNoLiteral
)
