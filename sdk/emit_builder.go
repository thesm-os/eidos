// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import emitbuilder "go.thesmos.sh/eidos/emit/builder"

// The fluent construction surface — the builders a generator
// assembles a whole package through.
//
// [Provenance] is the entry point and already lives in emit.go;
// what was missing is everything it hands out. Every nesting step
// passes a builder to a callback, and a plugin cannot write that
// callback without naming its parameter type, so the family
// crosses whole rather than by measured use — a façade covering
// the four builders today's generators reach for leaves the
// framework import in place for the fifth.
//
// The expression and statement shorthands in that package
// deliberately do not cross. `builder.Str`, `builder.Ident` and
// `builder.Return` are a second spelling of [NewLiteralString],
// [NewIdent] and [NewReturn], which emit_build.go already carries;
// re-exporting both would teach two vocabularies for one model,
// and the collisions it would force ([Return], [Tag],
// [Constraint], [Default] are all taken here) are the symptom
// rather than the reason.

// PackageBuilder is the root of a construction chain: the builder
// a [Provenance]'s `Package()` / `Anchor()` return, and the one
// whose `Build()` reports the errors every nested step recorded.
// Nested builders have no Err of their own — a callback that
// fails deep inside a method body surfaces at the top, so a plugin
// checks once.
type PackageBuilder = emitbuilder.PackageBuilder

// FileBuilder configures a whole [EmitFile] entity the plugin owns
// outright. Distinct from `PackageBuilder.File()`, which only
// selects the output tag subsequent declarations route to.
type FileBuilder = emitbuilder.FileBuilder

// Declaration builders — one per top-level kind a package can
// hold. Each carries the Target, Pos, Origin, Docs and Directive
// setters that decide where the declaration lands and which source
// node a diagnostic about it points at.
type (
	// StructBuilder builds an [EmitStruct] with its fields,
	// embeds, methods and type parameters.
	StructBuilder = emitbuilder.StructBuilder

	// InterfaceBuilder builds an [EmitInterface]. Its Method
	// callback receives the same [MethodBuilder] a struct's does;
	// the body it sets is simply dropped. Its Field callback is for
	// a language whose interface declares data alongside behaviour —
	// TypeScript's is the case, and a Go generator never calls it.
	InterfaceBuilder = emitbuilder.InterfaceBuilder

	// FunctionBuilder builds a package-level [EmitFunction].
	FunctionBuilder = emitbuilder.FunctionBuilder

	// EnumBuilder builds an [EmitEnum] over an underlying ref,
	// with its variants.
	EnumBuilder = emitbuilder.EnumBuilder

	// AliasBuilder builds an [EmitAlias]. A true alias
	// (`type X = Y`) cannot carry methods; attempting it records
	// [ErrAliasMethodForbidden] rather than emitting Go that will
	// not compile.
	AliasBuilder = emitbuilder.AliasBuilder

	// ConstantBuilder builds an [EmitConstant].
	ConstantBuilder = emitbuilder.ConstantBuilder

	// VariableBuilder builds an [EmitVariable].
	VariableBuilder = emitbuilder.VariableBuilder

	// ImportBuilder configures one import on a [FileBuilder] —
	// alias and doc lines. Imports an [External] ref implies are
	// registered by the backend and never built here.
	ImportBuilder = emitbuilder.ImportBuilder
)

// Member builders — the sub-elements a declaration owns. These
// exist as separate types because each carries an `Owner`
// back-pointer the builder wires as the callback returns; a member
// assembled as a bare struct literal instead is one the emit store
// cannot route a slot contribution to.
type (
	// MethodBuilder builds an [EmitMethod] — receiver,
	// parameters, results, type parameters, body.
	MethodBuilder = emitbuilder.MethodBuilder

	// FieldBuilder builds an [EmitField] on a struct.
	FieldBuilder = emitbuilder.FieldBuilder

	// ParamBuilder builds an [EmitParam]. Reached through the
	// optional callback on a Param call, which is where the
	// variadic marker and per-parameter directives are set.
	ParamBuilder = emitbuilder.ParamBuilder

	// TypeParamBuilder builds an [EmitTypeParam] and its
	// constraint.
	TypeParamBuilder = emitbuilder.TypeParamBuilder

	// EmbedBuilder builds an [EmitEmbed] on a struct or
	// interface.
	EmbedBuilder = emitbuilder.EmbedBuilder

	// EnumVariantBuilder builds an [EmitEnumVariant].
	EnumVariantBuilder = emitbuilder.EnumVariantBuilder
)

// ChainBuilder accumulates a left-to-right expression chain. The
// structured constructors nest right-to-left, which inverts the
// reading order of a fluent call sequence; this reads in source
// order and terminates in the same [Expr] the others produce.
type ChainBuilder = emitbuilder.ChainBuilder

// Chain re-exports [emitbuilder.Chain] — a fresh [ChainBuilder]
// seeded with its left-most expression.
//
//nolint:gochecknoglobals // alias re-export of a stable factory.
var Chain = emitbuilder.Chain

// InsertPos says where a cross-cutting contribution lands in its
// target slot. The Insert… methods on [Provenance] all take one,
// so without it half the contribution surface this façade already
// exposes cannot be called through it.
type InsertPos = emitbuilder.InsertPos

// The insert positions. Qualified against their bare spellings in
// [emitbuilder]: [At] here already names the position constructor,
// and a bare Before beside [BeforeNodesHook] reads as the wrong
// thing entirely. The family is renamed together so it still reads
// as one.
//
// Prefer [InsertBefore] / [InsertAfter] over [InsertAt]: indices
// shift as other plugins append, so a positional intent pinned to
// a number is one another plugin's contribution silently breaks.
//
//nolint:gochecknoglobals // alias re-exports of stable factories.
var (
	// InsertPrepend places the contribution at the slot's start.
	InsertPrepend = emitbuilder.Prepend

	// InsertAt places the contribution at a numeric index.
	InsertAt = emitbuilder.At

	// InsertBefore places the contribution immediately before the
	// item whose [EmitProvenance] ID equals the anchor.
	InsertBefore = emitbuilder.Before

	// InsertAfter mirrors [InsertBefore].
	InsertAfter = emitbuilder.After
)

// Type-argument lifting. A generic declaration is referenced by
// its name plus its own parameters restated as arguments, and the
// parameters live in two different models depending on whether the
// generator read the declaration from source or built it itself.
// Both lifters produce the same [Ref] slice [ApplyTypeArgs] takes.
//
//nolint:gochecknoglobals // alias re-exports of stable helpers.
var (
	// TypeArgsFromNodeParams lifts source-model type parameters.
	TypeArgsFromNodeParams = emitbuilder.TypeArgsFromNodeParams

	// TypeArgsFromEmitParams lifts emit-model type parameters.
	TypeArgsFromEmitParams = emitbuilder.TypeArgsFromEmitParams

	// ApplyTypeArgs instantiates a ref with the lifted arguments,
	// returning it unchanged when there are none — so a caller
	// handles the generic and non-generic cases in one path.
	ApplyTypeArgs = emitbuilder.ApplyTypeArgs
)

// Construction failure modes. Nested builders record rather than
// return, so these reach a plugin from `PackageBuilder.Build()` or
// from a slot append; compare with `errors.Is`.
//
//nolint:gochecknoglobals // alias re-exports of stable sentinels.
var (
	// ErrNilHost reports a slot append or insert against a nil
	// host.
	ErrNilHost = emitbuilder.ErrNilHost

	// ErrUnsupportedHost reports a contribution aimed at a
	// declaration kind that owns no such slot.
	ErrUnsupportedHost = emitbuilder.ErrUnsupportedHost

	// ErrNilOrigin reports an emit value queued without the
	// source node it was derived from, which would leave its
	// diagnostics pointing nowhere.
	ErrNilOrigin = emitbuilder.ErrNilOrigin

	// ErrAliasMethodForbidden reports a method declared on a true
	// alias.
	ErrAliasMethodForbidden = emitbuilder.ErrAliasMethodForbidden

	// ErrUnknownInsertPos reports an [InsertPos] the slot layer
	// has no case for — unreachable through the constructors
	// above, and present so a future variant fails loudly.
	ErrUnknownInsertPos = emitbuilder.ErrUnknownInsertPos
)
