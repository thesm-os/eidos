// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import "go.thesmos.sh/eidos/node"

// The source model, re-exported unprefixed.
//
// A plugin reads the source graph far more often than it writes
// the emit one — every annotator only reads, and a generator
// reads before it writes — so the source spelling gets the bare
// name and the emit spelling carries the Emit prefix. Reversing
// that would tax the common case to spare the rarer one.
//
// Only the reading vocabulary is here. The graph is built by
// frontends and rewired once ([RewireOwners]); nothing in this
// file lets a plugin re-parent a node behind the frontend's back.

// Node is the interface every source declaration satisfies. It is
// what a hook, a predicate, or a helper that does not care which
// kind it holds takes as its parameter type.
type Node = node.Node

// BaseNode is the shared field block every concrete source kind
// embeds — position, docs, directives, metadata. Named by plugins
// that construct source nodes, which in practice means frontends;
// annotators and generators reach the same fields through the
// concrete kind.
type BaseNode = node.BaseNode

// The concrete source kinds, in the order a declaration nests:
// container, then declaration, then the parts of one.
type (
	// Package is a single source package — the unit a frontend
	// loads and the unit [FrontendContext.Pattern] expands to.
	Package = node.Package

	// File is one source file within a [Package]. Generators that
	// route output per input file read the origin's file rather
	// than assuming one file per package.
	File = node.File

	// Import is one import declaration on a [File].
	Import = node.Import

	// Struct is a record declaration. The kind most generators
	// key off, via [StructHook] or a store query.
	Struct = node.Struct

	// Interface is a contract a type satisfies without being
	// instantiable. Reached through [InterfaceHook]; note that a
	// constraint-only interface is also an Interface —
	// [IsConstraint] separates the two.
	//
	// Whether it declares methods, fields or both is the source
	// language's business: a Go interface is a method set, a
	// TypeScript one is a structural type that usually declares only
	// fields. A generator reading interfaces across languages walks
	// both slices.
	Interface = node.Interface

	// Method is a method declared on a [Struct], an [Interface],
	// or any other named type. Interface- and struct-declared
	// methods share the type; [Method.HasReceiver] tells them
	// apart.
	Method = node.Method

	// Field is one member of a [Struct].
	Field = node.Field

	// Function is a standalone function — a method reaches
	// [MethodHook] instead.
	Function = node.Function

	// Param is one parameter of a [Function] or [Method].
	Param = node.Param

	// Return is one result of a [Function] or [Method]. Named and
	// anonymous results share the type.
	Return = node.Return

	// TypeParam is one type parameter of a generic declaration.
	TypeParam = node.TypeParam

	// Constraint is the bound on a [TypeParam].
	Constraint = node.Constraint

	// TypeRef is the unified type reference — every place the
	// source names a type. Its [TypeRefKind] discriminator selects
	// the variant; read it before reaching for a variant-specific
	// field, since the unused fields are zero rather than absent.
	TypeRef = node.TypeRef

	// Alias is a type-alias declaration.
	Alias = node.Alias

	// Constant is a constant declaration.
	Constant = node.Constant

	// Variable is a package-level variable declaration.
	Variable = node.Variable

	// Enum is an enumeration — a named type with a closed set of
	// declared [EnumVariant] values, recognised by the frontend
	// rather than by a language keyword.
	Enum = node.Enum

	// EnumVariant is one declared value of an [Enum].
	EnumVariant = node.EnumVariant

	// Embed is one embedded type on a [Struct] or [Interface].
	Embed = node.Embed
)

// TypeRefKind discriminates the variant forms a [TypeRef] takes.
// Switch on it before reading variant-specific fields.
type TypeRefKind = node.TypeRefKind

// The [TypeRef] variants. A generator that renders a type must
// handle every one it can reach: an unhandled variant silently
// renders as its zero form rather than failing, which is a wrong
// output file rather than an error.
const (
	TypeRefNamed         = node.TypeRefNamed
	TypeRefPointer       = node.TypeRefPointer
	TypeRefSlice         = node.TypeRefSlice
	TypeRefArray         = node.TypeRefArray
	TypeRefMap           = node.TypeRefMap
	TypeRefFunc          = node.TypeRefFunc
	TypeRefTypeParam     = node.TypeRefTypeParam
	TypeRefAnonStruct    = node.TypeRefAnonStruct
	TypeRefAnonInterface = node.TypeRefAnonInterface
)

// Method-set resolution — flattening an [Interface]'s own
// declarations together with everything its embeds contribute.
type (
	// InterfaceResolver resolves an embedded [TypeRef] to the
	// [Interface] it names. [StoreReader.MethodSet] supplies one
	// bound to the run's store; a plugin resolving from its own
	// index writes its own.
	InterfaceResolver = node.InterfaceResolver

	// MethodSetResult is a resolved method set plus every embed
	// that contributed nothing. Check [MethodSetResult.OK] before
	// treating the answer as complete — a partial set renders as a
	// plausible but wrong double.
	MethodSetResult = node.MethodSetResult

	// MethodSetEntry pairs a resolved method with the top-level
	// embed it arrived through, so generated output can attribute
	// a method it did not find declared in front of it.
	MethodSetEntry = node.MethodSetEntry

	// MethodSetIssue is one embed that contributed no methods, and
	// why.
	MethodSetIssue = node.MethodSetIssue

	// MethodSetReason classifies a [MethodSetIssue]. The
	// distinction is actionable: an unresolved embed usually means
	// the run was narrow, while a non-interface or cyclic embed is
	// a source defect no wider run fixes.
	MethodSetReason = node.MethodSetReason
)

// Reasons an embed contributed no methods.
const (
	ReasonUnresolved   = node.ReasonUnresolved
	ReasonNonInterface = node.ReasonNonInterface
	ReasonCyclic       = node.ReasonCyclic
	ReasonGeneric      = node.ReasonGeneric
)

// MethodSet re-exports [node.MethodSet] — an interface's own
// declarations followed by each embed's contribution, in embed
// order.
//
//nolint:gochecknoglobals // alias re-export of a stable helper.
var MethodSet = node.MethodSet

// Name-keyed lookups over the slices the concrete kinds expose.
// Re-exported as a family: a plugin that reaches for one of these
// reaches for the rest within the same function.
//
//nolint:gochecknoglobals // alias re-exports of stable helpers.
var (
	// MethodByName finds a method by name in a method slice.
	MethodByName = node.MethodByName

	// Declares reports whether a method slice contains name.
	Declares = node.Declares

	// PointerReceiver reports whether the named method in the
	// slice is declared on a pointer receiver — the question that
	// decides whether a generated value satisfies an interface.
	PointerReceiver = node.PointerReceiver

	// FieldOfType finds the first field of a [Struct] whose type
	// is the named builtin.
	FieldOfType = node.FieldOfType

	// IsExportedName reports whether an identifier is exported.
	// Language-agnostic: it asks about the identifier, not about
	// the Go compiler's view of it.
	IsExportedName = node.IsExportedName

	// LocalName strips the package qualifier from a qualified
	// name.
	LocalName = node.LocalName

	// EmbedName returns an embed's referenced type name and
	// whether it was embedded by pointer.
	EmbedName = node.EmbedName

	// IsConstraint reports whether an [Interface] is a
	// constraint-only interface rather than a method set — the
	// check a generator emitting a double owes, since a constraint
	// has no implementation to double.
	IsConstraint = node.IsConstraint

	// AnonReturns builds an unnamed result list from types. The
	// emit-side counterpart is [EmitAnonReturns].
	AnonReturns = node.AnonReturns

	// ReturnTypes projects a result list onto its types.
	ReturnTypes = node.ReturnTypes
)

// RewireOwners re-exports [node.RewireOwners] — repopulates the
// Owner back-pointers across a freshly built [Package].
//
// A frontend that assembles a package without calling this hands
// downstream plugins a graph where every Owner is nil, and
// upward traversal silently yields nothing rather than failing.
// Annotators and generators never call it: the owners are already
// wired by the time their phase runs, and re-running it mid-phase
// races every concurrent reader of the graph.
//
//nolint:gochecknoglobals // alias re-export of a stable helper.
var RewireOwners = node.RewireOwners

// Source-tree traversal. Named with the Node prefix because [Walk]
// already belongs to the annotator hook driver — a plugin reaching
// for "walk" wants that one far more often than this.
type (
	// NodeVisitor is the callback [NodeWalk] invokes per node.
	// Returning nil prunes the subtree; returning the receiver
	// continues descent.
	NodeVisitor = node.Visitor

	// NodeVisitorFunc adapts a plain function to [NodeVisitor],
	// for a traversal with no state to carry between nodes.
	NodeVisitorFunc = node.VisitorFunc
)

// NodeWalk re-exports [node.Walk] — traverses a source subtree in
// declaration order. Use it for the ad-hoc descent a store query
// cannot express; the per-kind [Walk] hooks cover the common case
// and record reads for cache-key derivation, which this does not.
//
//nolint:gochecknoglobals // alias re-export of a stable helper.
var NodeWalk = node.Walk

// Companion re-exports [node.Companion] — the nullary function of a
// given name returning a given type, or nil where none is.
//
// The convention a generator reaches for when a value can be seeded
// from source the consumer already wrote: `UserDefaults()` beside
// `User`. The caller composes the identifier through the language,
// and gets back the declaration so it spells the reference its own
// output needs.
var Companion = node.Companion
