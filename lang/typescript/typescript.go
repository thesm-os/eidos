// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript

// Language is the language identifier the TypeScript adapter answers
// to. Plugins dispatch their per-language declarations on this
// constant; the frontend stamps it on every package it produces and
// the backend answers to the same name, which is what lets an
// annotator ask about the language it is reading while a generator
// asks about the language the run renders.
const Language = "typescript"

// Visibility values for [MetaVisibility].
const (
	// VisibilityPublic is the default for a member with no modifier,
	// stamped only where the source wrote `public` explicitly.
	VisibilityPublic = "public"

	// VisibilityProtected is TypeScript's `protected` modifier.
	VisibilityProtected = "protected"

	// VisibilityPrivate is TypeScript's `private` modifier — the
	// compile-time form, which erases at runtime.
	VisibilityPrivate = "private"

	// VisibilityHard is a `#name` private field, which is enforced at
	// runtime by the language rather than by the type checker.
	// Distinct from [VisibilityPrivate] because the two differ in
	// what they guarantee and in how a member is spelled at a use
	// site.
	VisibilityHard = "hard-private"
)

// Heritage values for [MetaHeritage].
const (
	// HeritageExtends is an `extends` clause. On a class it names a
	// base class and there is at most one; on an interface it names
	// another interface and there may be several.
	HeritageExtends = "extends"

	// HeritageImplements is a class's `implements` clause.
	HeritageImplements = "implements"
)

// Accessor values for [MetaAccessor].
const (
	// AccessorGet is a `get` accessor — a method in the model whose
	// use site is a property read.
	AccessorGet = "get"

	// AccessorSet is a `set` accessor.
	AccessorSet = "set"
)

// The TypeScript type names this adapter spells by name rather than
// deriving, so a backend and a plugin cannot disagree about them.
const (
	// TypeVoid is the return type of a callable that returns nothing.
	TypeVoid = "void"

	// TypeNever is the type with no members — what an empty union
	// spells. Rendering the empty string instead would produce
	// `type Empty = ;`, which does not parse.
	TypeNever = "never"

	// TypeUnknown is the safe top type. `any` disables checking;
	// `unknown` admits every value and forces the consumer to narrow,
	// which is what a generated type should demand of a value it
	// cannot describe.
	TypeUnknown = "unknown"

	// LiteralTrue and LiteralFalse are the boolean literals, named
	// because they are spelled in several places — a literal
	// renderer, a zero value, a directive value's parse — and a typo
	// in any of them produces an identifier rather than a boolean.
	LiteralTrue  = "true"
	LiteralFalse = "false"

	// The scalar type names a literal is rendered against. Prefixed
	// Scalar rather than Type because [TypeString] is already the
	// function that spells a type, and a constant of that name would
	// shadow it.
	ScalarString  = "string"
	ScalarNumber  = "number"
	ScalarBigInt  = "bigint"
	ScalarBoolean = "boolean"

	// TypeNull and TypeUndefined are the two absent values.
	// TypeScript distinguishes them and `strictNullChecks` makes the
	// distinction load-bearing.
	TypeNull      = "null"
	TypeUndefined = "undefined"
)
