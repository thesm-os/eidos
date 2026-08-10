// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import "go.thesmos.sh/eidos/emit"

// The emit model, re-exported with the Emit prefix.
//
// Every name here also names a source kind carrying a different
// value, and the two fail silently when confused: an emit value
// built against a source shape never renders, and a source query
// against an emit shape never matches. The prefix is what stops a
// plugin author writing one and receiving the other. Names with
// no source counterpart ([Stmt], [Tag], the ref shapes) keep
// their emit spelling — there is nothing to distinguish them
// from.

// The concrete emit kinds, mirroring the source model kind for
// kind so a generator translating a declaration writes a
// field-by-field mapping rather than a projection.
type (
	// EmitPackage is a generated package — the root a generator
	// adds to the emit view.
	EmitPackage = emit.Package

	// EmitFile is one generated file within an [EmitPackage].
	// Backends group by [EmitTarget], not by this, so a plugin
	// rarely builds one directly.
	EmitFile = emit.File

	// EmitImport is one import on an [EmitFile]. The Go backend
	// registers imports from the refs it renders, so a plugin
	// building refs through [External] does not add these by hand.
	EmitImport = emit.Import

	// EmitStruct is a generated record declaration.
	EmitStruct = emit.Struct

	// EmitInterface is a generated method-set declaration.
	EmitInterface = emit.Interface

	// EmitMethod is a generated method.
	EmitMethod = emit.Method

	// EmitField is one member of an [EmitStruct]. Carries the tags
	// slot cross-cutting tag plugins contribute into.
	EmitField = emit.Field

	// EmitFunction is a generated standalone function.
	EmitFunction = emit.Function

	// EmitParam is one parameter of an [EmitFunction] or
	// [EmitMethod].
	EmitParam = emit.Param

	// EmitReturn is one result. Mixing named and unnamed results
	// in one list is rejected with [ErrMixedNamedReturns] rather
	// than rendered as invalid source.
	EmitReturn = emit.Return

	// EmitTypeParam is one type parameter of a generic emit
	// declaration.
	EmitTypeParam = emit.TypeParam

	// EmitConstraint is the bound on an [EmitTypeParam].
	EmitConstraint = emit.Constraint

	// EmitTypeRef is a reference to another emit entity in the
	// same run. Built via [Internal] — the reference resolves at
	// render time, so the target need not be complete when the
	// ref is made.
	EmitTypeRef = emit.TypeRef

	// EmitAlias is a generated type alias.
	EmitAlias = emit.Alias

	// EmitConstant is a generated constant declaration.
	EmitConstant = emit.Constant

	// EmitVariable is a generated package-level variable.
	EmitVariable = emit.Variable

	// EmitEnum is a generated enumeration.
	EmitEnum = emit.Enum

	// EmitEnumVariant is one value of an [EmitEnum].
	EmitEnumVariant = emit.EnumVariant

	// EmitEmbed is one embedded type on an [EmitStruct] or
	// [EmitInterface].
	EmitEmbed = emit.Embed
)

// Emit shapes with no source counterpart, keeping their emit
// spelling.
type (
	// Stmt is one statement in a generated body. Built through the
	// New… constructors rather than by hand: the fields a shape
	// does not use are zero rather than absent, so an
	// inconsistently populated Stmt renders as something plausible
	// and wrong.
	Stmt = emit.Stmt

	// Tag is one key/value entry in an [EmitField]'s struct-tag
	// block. Cross-cutting tag plugins append these into the
	// field's tags slot rather than overwriting the host's own
	// tag.
	Tag = emit.Tag

	// BuiltinRef names a true language builtin — no import, no
	// target. Built via [Builtin].
	BuiltinRef = emit.BuiltinRef

	// ExternalRef names a type in another package, carrying the
	// import path the backend registers at render time. Built via
	// [External].
	ExternalRef = emit.ExternalRef

	// CompositeRef is a compound shape (pointer, slice, array,
	// map, func, union, anonymous struct) over inner refs. Built
	// via [Ptr], [SliceOf], [ArrayOf], [MapOf], [FuncOf], [Union],
	// [AnonStructOf].
	CompositeRef = emit.CompositeRef

	// AnonField is one named field of an anonymous-struct
	// [CompositeRef]. Deliberately not an [EmitField]: an inline
	// struct has no declaration site, so it has no slots for a
	// cross-cutting contribution to land in.
	AnonField = emit.AnonField

	// UnionTerm is one member of a union [CompositeRef], recording
	// whether the source term carried Go's `~` prefix.
	UnionTerm = emit.UnionTerm

	// EmitProvenance is the record of who appended a contribution
	// into a [Slot], at which authority, from where. Backends
	// order and attribute contributions by it. Named with the
	// prefix because [Provenance] is the builder a plugin stamps
	// contributions *through*; this is the stamp it leaves.
	EmitProvenance = emit.Provenance
)

// Emit-side kind discriminators, the values a plugin constrains a
// [Slot] on and a plugin-defined emit kind sits alongside.
//
// Prefixed for the same reason the types are: every name below
// also exists in [NodeKindPackage]'s set with a different value,
// and a slot constrained on the wrong one accepts nothing without
// reporting anything.
const (
	EmitKindPackage     = emit.KindPackage
	EmitKindFile        = emit.KindFile
	EmitKindImport      = emit.KindImport
	EmitKindStruct      = emit.KindStruct
	EmitKindInterface   = emit.KindInterface
	EmitKindMethod      = emit.KindMethod
	EmitKindField       = emit.KindField
	EmitKindFunction    = emit.KindFunction
	EmitKindVariable    = emit.KindVariable
	EmitKindConstant    = emit.KindConstant
	EmitKindEnum        = emit.KindEnum
	EmitKindEnumVariant = emit.KindEnumVariant
	EmitKindAlias       = emit.KindAlias
	EmitKindEmbed       = emit.KindEmbed
	EmitKindParam       = emit.KindParam
	EmitKindReturn      = emit.KindReturn
	EmitKindTypeParam   = emit.KindTypeParam
	EmitKindTag         = emit.KindTag

	EmitKindTypeRef      = emit.KindTypeRef
	EmitKindExternalRef  = emit.KindExternalRef
	EmitKindBuiltinRef   = emit.KindBuiltinRef
	EmitKindCompositeRef = emit.KindCompositeRef

	EmitKindStmt = emit.KindStmt
	EmitKindExpr = emit.KindExpr
	EmitKindSlot = emit.KindSlot
)

// Error sentinels the emit model returns. Wrap with errors.Is; a
// plugin that appends into a slot it does not own is the one that
// needs to tell these apart.
var (
	// ErrSlotElementType is returned when an append presents a
	// value whose kind the slot does not accept. The usual cause
	// is a source kind used where an emit kind was meant.
	ErrSlotElementType = emit.ErrSlotElementType

	// ErrProvenanceNotFound is returned when a lookup names a
	// contribution ID no append ever stamped.
	ErrProvenanceNotFound = emit.ErrProvenanceNotFound

	// ErrMixedNamedReturns is returned when a result list mixes
	// named and unnamed entries — a shape no target language
	// renders, caught at construction rather than at compile time
	// in the generated file.
	ErrMixedNamedReturns = emit.ErrMixedNamedReturns
)

// Emit-tree traversal. Prefixed for the same reason [NodeWalk] is:
// [Walk] is the annotator hook driver.
type (
	// EmitVisitor is the callback [EmitWalk] invokes per emit
	// node. Returning nil prunes the subtree.
	EmitVisitor = emit.Visitor

	// EmitVisitorFunc adapts a plain function to [EmitVisitor].
	// Shares its shape with [NodeVisitorFunc] so one traversal
	// body can serve both trees.
	EmitVisitorFunc = emit.VisitorFunc
)

// EmitWalk re-exports [emit.Walk] — traverses a generated subtree
// in declaration order. A finalise-phase generator inspecting what
// earlier generators produced uses this; a generator building its
// own output already holds the values.
//
//nolint:gochecknoglobals // alias re-export of a stable helper.
var EmitWalk = emit.Walk
