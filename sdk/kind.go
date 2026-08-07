// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import (
	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/node"
)

// Kind is the structural discriminator both [node.Node] and
// [emit.Node] return from their Kind method, and the type
// [DirectiveSchema.AppliesTo] scopes against.
//
// Example targeting a directive to source structs and
// interfaces:
//
//	sdk.NewDirective("repo").
//	    On(sdk.NodeKindStruct).
//	    On(sdk.NodeKindInterface).
//	    Build()
type Kind = kind.Kind

// Source-node kinds, the values [SchemaBuilder.On] scopes a
// directive against.
//
// # Why the NodeKind prefix
//
// Every one of these names also exists in [emit], carrying a
// different value — source `KindStruct` is "struct" and emit's is
// "emit.struct". Re-exporting either set unprefixed would let an
// author reach for one and silently receive the other, and both
// halves of that mistake fail quietly: a slot constrained on a
// source kind accepts nothing an emit builder produces, and a
// directive scoped to an emit kind matches no source node, so the
// plugin simply never fires. Neither reports anything.
//
// The prefix follows the same rule already applied to [EmitNode]
// and [EmitTarget]: a name meaningful on both sides of the
// pipeline is qualified, and one meaningful on only one side
// ([Ref], [Expr]) is not.
//
// Emit-side kinds are deliberately absent. They belong to the
// emit-construction surface, where the [emit] package already owns
// a coherent namespace, and a plugin declaring its own emit kind
// writes its own constant regardless.
const (
	NodeKindPackage     = node.KindPackage
	NodeKindFile        = node.KindFile
	NodeKindImport      = node.KindImport
	NodeKindStruct      = node.KindStruct
	NodeKindInterface   = node.KindInterface
	NodeKindMethod      = node.KindMethod
	NodeKindField       = node.KindField
	NodeKindFunction    = node.KindFunction
	NodeKindParam       = node.KindParam
	NodeKindReturn      = node.KindReturn
	NodeKindTypeParam   = node.KindTypeParam
	NodeKindTypeRef     = node.KindTypeRef
	NodeKindAlias       = node.KindAlias
	NodeKindConstant    = node.KindConstant
	NodeKindVariable    = node.KindVariable
	NodeKindEnum        = node.KindEnum
	NodeKindEnumVariant = node.KindEnumVariant
	NodeKindEmbed       = node.KindEmbed
)
