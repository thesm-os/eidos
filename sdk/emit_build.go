// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import "go.thesmos.sh/eidos/emit"

// The emit construction vocabulary — the type references,
// expressions, and statements a generator assembles its output
// from.
//
// Constructor names are carried over from [emit] verbatim. There
// is no source-model counterpart to collide with, and a plugin
// author who already knows `emit.NewLiteralString` should not have
// to learn a second spelling of it. The families are re-exported
// whole rather than by measured use: a generator that reaches for
// one literal constructor reaches for its siblings in the next
// line, and a facade covering four of them leaves the [emit]
// import in place for the fifth, which defeats the point.

// Type references — how an emit value names a type. Which one to
// use is decided by where the type lives, and getting it wrong
// produces a rendered file with a missing or wrong import rather
// than an error.
//
//nolint:gochecknoglobals // alias re-exports of stable factories.
var (
	// Builtin names a true language builtin (`string`, `int`).
	// No import, no target.
	Builtin = emit.Builtin

	// External names a type in another package by import path.
	// The Go backend registers the import when it renders the ref,
	// so a plugin never adds an [EmitImport] for one of these.
	External = emit.External

	// Internal names another emit entity produced in this same
	// run. Resolution is deferred to render time, so the target
	// may still be under construction when the ref is made — which
	// is what makes mutually referential output expressible.
	Internal = emit.Internal

	// Ptr wraps a ref as a pointer to it.
	Ptr = emit.Ptr

	// SliceOf wraps a ref as a variable-length sequence of it.
	SliceOf = emit.SliceOf

	// ArrayOf wraps a ref as a fixed-length sequence of it.
	ArrayOf = emit.ArrayOf

	// MapOf builds an associative container ref from key and
	// value refs.
	MapOf = emit.MapOf

	// FuncOf builds a function-type ref from parameter and result
	// refs.
	FuncOf = emit.FuncOf

	// Union builds a union ref from its terms — the constraint
	// shape a generic type parameter is bounded by.
	Union = emit.Union

	// AnonStructOf builds an inline struct-type ref. Its fields
	// are [AnonField] values, not [EmitField]: an inline struct
	// has no declaration site to route a slot contribution to.
	AnonStructOf = emit.AnonStructOf
)

// EmitAnonReturns re-exports [emit.AnonReturns] — an unnamed
// result list built from refs. The source-side counterpart keeps
// the bare name [AnonReturns].
//
//nolint:gochecknoglobals // alias re-export of a stable factory.
var EmitAnonReturns = emit.AnonReturns

// Expressions — the value-side vocabulary. Every constructor
// returns an [Expr] the Go backend renders through `renderExpr`,
// which is also where the imports an [External] ref needs get
// registered; an expression assembled by string concatenation
// instead skips that and renders an unimported identifier.
//
// [NewExternal] lives in emit.go, alongside the [Ref] and [Expr]
// contracts it was first re-exported for.
//
//nolint:gochecknoglobals // alias re-exports of stable factories.
var (
	// NewIdent is a bare identifier — a local, a parameter, a
	// name already in scope at the render site.
	NewIdent = emit.NewIdent

	// NewField is a selector on a receiver expression (`x.Name`).
	NewField = emit.NewField

	// NewIndex is an index expression (`x[i]`).
	NewIndex = emit.NewIndex

	// NewSlice is a slice expression (`x[lo:hi:cap]`). Nil bounds
	// render as omitted.
	NewSlice = emit.NewSlice

	// NewCall calls a callee expression with arguments.
	NewCall = emit.NewCall

	// NewCallGeneric calls a callee with explicit type arguments,
	// for the cases inference cannot carry.
	NewCallGeneric = emit.NewCallGeneric

	// NewMethodCall calls a named method on a receiver.
	NewMethodCall = emit.NewMethodCall

	// NewAddr takes the address of a target (`&x`).
	NewAddr = emit.NewAddr

	// NewDeref dereferences a target (`*x`).
	NewDeref = emit.NewDeref

	// NewParen parenthesises an inner expression. Needed where
	// precedence would otherwise change the meaning — the
	// constructors do not insert parentheses on the plugin's
	// behalf.
	NewParen = emit.NewParen

	// NewUnary applies a prefix operator to an operand.
	NewUnary = emit.NewUnary

	// NewBinary joins two operands with an infix operator.
	NewBinary = emit.NewBinary

	// NewTypeAssert asserts a receiver to a type.
	NewTypeAssert = emit.NewTypeAssert

	// NewComposite is a positional composite literal of a type.
	NewComposite = emit.NewComposite

	// NewCompositeKeyed is a keyed composite literal — the form
	// to prefer for a struct, since a positional one silently goes
	// wrong when the target gains a field.
	NewCompositeKeyed = emit.NewCompositeKeyed

	// NewFuncLit is a function literal with its own parameters,
	// results, and body.
	NewFuncLit = emit.NewFuncLit

	// NewMake is a make expression over a type.
	NewMake = emit.NewMake

	// NewNew is a new expression over a type.
	NewNew = emit.NewNew

	// NewRawExpr is verbatim text. The escape hatch: nothing
	// inspects it, no import is registered from it, and no
	// backend can reformat it. Reach for it only when the shape
	// has no constructor.
	NewRawExpr = emit.NewRawExpr
)

// Literals. Typed per literal kind rather than one Any-taking
// constructor, so a value rendering as the wrong literal form
// (an int as a float, a string unquoted) is a compile error in
// the plugin rather than a diff in the generated file.
//
//nolint:gochecknoglobals // alias re-exports of stable factories.
var (
	NewLiteralString = emit.NewLiteralString
	NewLiteralInt    = emit.NewLiteralInt
	NewLiteralUint   = emit.NewLiteralUint
	NewLiteralFloat  = emit.NewLiteralFloat
	NewLiteralBool   = emit.NewLiteralBool
	NewLiteralRune   = emit.NewLiteralRune
	NewLiteralNil    = emit.NewLiteralNil

	// NewLiteralRaw is verbatim literal text, for a form the
	// typed constructors do not cover. Carries the same caveats
	// as [NewRawExpr].
	NewLiteralRaw = emit.NewLiteralRaw
)

// Statements — the body-side vocabulary.
//
//nolint:gochecknoglobals // alias re-exports of stable factories.
var (
	// NewBlock groups statements into one.
	NewBlock = emit.NewBlock

	// NewExprStmt promotes an expression to a statement — a call
	// whose result is discarded.
	NewExprStmt = emit.NewExprStmt

	// NewAssign assigns values to targets with an operator
	// (`=`, `:=`, `+=`).
	NewAssign = emit.NewAssign

	// NewVarStmt declares a local variable with an optional type
	// and initialiser.
	NewVarStmt = emit.NewVarStmt

	// NewConstStmt declares a local constant.
	NewConstStmt = emit.NewConstStmt

	// NewReturn returns the given values.
	NewReturn = emit.NewReturn

	// NewIf is a conditional with no else branch.
	NewIf = emit.NewIf

	// NewIfElse is a conditional with both branches.
	NewIfElse = emit.NewIfElse

	// NewIfInit is a conditional with an init statement — the
	// `if err := f(); err != nil` shape.
	NewIfInit = emit.NewIfInit

	// NewFor is a condition-only loop.
	NewFor = emit.NewFor

	// NewForFull is a loop with init, condition, and post.
	NewForFull = emit.NewForFull

	// NewForRange is a range loop. Empty key or value names
	// render as omitted rather than as the blank identifier.
	NewForRange = emit.NewForRange

	// NewSwitch is a switch over a condition.
	NewSwitch = emit.NewSwitch

	// NewSwitchInit is a switch with an init statement.
	NewSwitchInit = emit.NewSwitchInit

	// NewCase is one case clause of a switch.
	NewCase = emit.NewCase

	// NewDefault is the default clause of a switch.
	NewDefault = emit.NewDefault

	// NewBreak breaks, optionally to a label.
	NewBreak = emit.NewBreak

	// NewContinue continues, optionally to a label.
	NewContinue = emit.NewContinue

	// NewLabel labels an inner statement.
	NewLabel = emit.NewLabel

	// NewDefer defers a call.
	NewDefer = emit.NewDefer

	// NewGo starts a call in a new goroutine.
	NewGo = emit.NewGo

	// NewRenderStmt embeds a whole emit node as a statement, so a
	// nested declaration renders in place through the backend's
	// normal path rather than being flattened to text.
	NewRenderStmt = emit.NewRenderStmt

	// NewRawStmt is verbatim statement text. Same caveats as
	// [NewRawExpr]: unformatted, uninspected, and invisible to
	// import registration.
	NewRawStmt = emit.NewRawStmt
)
