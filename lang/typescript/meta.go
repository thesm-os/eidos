// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript

import (
	"encoding/json"
	"fmt"

	"go.thesmos.sh/eidos/core/meta"
)

// Decorator is one decorator applied to a declaration.
//
// Args is the argument list in verbatim source form, parentheses
// included — `({ name: 'users' })` — and empty for a bare `@deco`
// with no call. Verbatim because a decorator's arguments are
// arbitrary expressions, and parsing them would mean a second
// expression model for the one case that needs it.
type Decorator struct {
	// Name is the decorator's identity, qualified where the source
	// qualified it: `Column`, or `ns.deco` for `@ns.deco()`. The last
	// segment alone would collide with any other decorator of that
	// name.
	Name string `json:"name"`

	// Args is the argument list, verbatim.
	Args string `json:"args,omitempty"`
}

// Overload is one overload signature declared for a function or
// method.
//
// TypeScript spells an overloaded callable as several bodiless
// signatures followed by one implementation. The signatures are what
// a caller may use; the implementation's own signature is not
// publicly callable and exists to cover all of them.
type Overload struct {
	// Text is the signature in verbatim source form, without the
	// trailing semicolon — `overloaded(a: string): void`.
	//
	// Verbatim rather than structured because an overload set is
	// alternative spellings of one callable, and the model has one
	// signature per declaration. Reproducing the whole parameter
	// model per alternative would duplicate most of the converter to
	// serve a shape the graph cannot hold anyway.
	Text string `json:"text"`
}

// overloadsParser decodes the JSON wire form of [MetaOverloads].
func overloadsParser(raw string) ([]Overload, error) {
	if raw == "" {
		return nil, nil
	}
	var out []Overload
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("lang/typescript: parse overloads: %w", err)
	}
	return out, nil
}

// decoratorsParser decodes the JSON wire form of [MetaDecorators].
func decoratorsParser(raw string) ([]Decorator, error) {
	if raw == "" {
		return nil, nil
	}
	var out []Decorator
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("lang/typescript: parse decorators: %w", err)
	}
	return out, nil
}

// The `ts.*` metadata vocabulary every TypeScript-speaking part of a
// pipeline shares: the frontend stamps it, the backend reads it,
// bridges from other source languages write it, and plugins query it.
//
// Declared here rather than in the frontend that produces most of it,
// because a meta key is interned by name and a consumer that cannot
// import the declaring package re-declares it by string instead —
// forfeiting the compile-time link to the declaration and the
// rename-safety that comes with it.
//
// lang/typescript is the one package every TypeScript-speaking
// consumer can import: depguard forbids a backend importing a
// frontend and forbids plugins importing either, and this is a leaf
// over node, emit and core that all three may depend on.
//
// # Why so much rides on keys
//
// TypeScript's declaration syntax carries modifiers the
// language-agnostic model has no field for — optionality, readonly,
// visibility, async — and type shapes it has no variant for: unions,
// intersections, tuples. Promoting any of them to a [node] field
// would put a TypeScript fact in the package every language shares.
var (
	// MetaDecorators lists the decorators applied to a declaration,
	// in source order.
	//
	// One ordered list rather than a key per decorator name, because
	// TypeScript makes both order and repetition meaningful. Decorator
	// expressions evaluate top-down and apply bottom-up, so `@A @B`
	// and `@B @A` compose differently — a guard running before a
	// transform is not the same as one running after. And the same
	// decorator may be applied more than once: a route documenting
	// several responses writes `@ApiResponse` per status code.
	//
	// A key per name loses both. Two orderings would read alike, and
	// the second application of a repeated decorator would overwrite
	// the first with nothing to indicate it had.
	MetaDecorators = meta.NewKey(
		"ts.decorators",
		decoratorsParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaDefiniteAssignment reports a `!` definite-assignment
	// assertion — `name!: string`.
	//
	// The author telling the compiler a field is initialised
	// somewhere it cannot see. It is not optional and it has no
	// initialiser, so without this key those two absences read as a
	// plain required field and a generator emitting a constructor
	// would have nothing to distinguish it.
	MetaDefiniteAssignment = meta.NewKey(
		"ts.definiteAssignment",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaReExport reports a [node.Import] produced by a re-export —
	// `export { X } from './y'` or `export * from './y'`.
	//
	// The module is a dependency and a contributor to this module's
	// public surface at once. Barrel files are built entirely from
	// these, so a frontend recording only the dependency would report
	// that such a file declares nothing.
	MetaReExport = meta.NewKey(
		"ts.reExport",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaReExportNames lists the names a re-export forwards, in
	// source order, each as written — `Y as Z` appears as "Y as Z".
	// Empty for a star re-export, which forwards everything.
	MetaReExportNames = meta.NewKey(
		"ts.reExportNames",
		meta.StringListParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaExported reports that the declaration was reached through
	// an `export_statement`.
	//
	// TypeScript has no naming rule for visibility — unlike Go, where
	// the identifier's first rune decides it — so this is the only
	// record of whether a declaration leaves its module.
	MetaExported = meta.NewKey(
		"ts.exported",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaDefaultExport reports an `export default` declaration. A
	// module has at most one, and it is imported without braces and
	// under a name the importing file chooses, so a generator
	// referencing it cannot derive the local name from the
	// declaration's own.
	MetaDefaultExport = meta.NewKey(
		"ts.defaultExport",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaAmbient reports a declaration inside `declare` — a type
	// asserted to exist elsewhere, with no implementation here.
	// Every declaration in a `.d.ts` file carries it.
	//
	// A generator emitting an implementation for an ambient
	// declaration is emitting something the author said already
	// exists.
	MetaAmbient = meta.NewKey(
		"ts.ambient",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaAbstract reports an `abstract` class or member.
	MetaAbstract = meta.NewKey(
		"ts.abstract",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaOptional reports a `?`-marked property or parameter.
	//
	// Distinct from a type union with `undefined`, which
	// [MetaNullable] carries: `x?: string` may be absent from an
	// object literal entirely, while `x: string | undefined` must be
	// present and may hold undefined. The distinction is what
	// `exactOptionalPropertyTypes` turns on, so collapsing the two
	// would generate types that fail under it.
	MetaOptional = meta.NewKey(
		"ts.optional",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaRest reports a tuple element or parameter carrying `...`.
	//
	// On a [node.Param] the model already has [node.Param.Variadic],
	// which the frontend sets; this carries the same fact for a tuple
	// element, which has no field for it.
	MetaRest = meta.NewKey(
		"ts.rest",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaReadonly reports a `readonly` property or index signature.
	MetaReadonly = meta.NewKey(
		"ts.readonly",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaStatic reports a `static` class member.
	MetaStatic = meta.NewKey(
		"ts.static",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaVisibility records an explicit access modifier — one of
	// [VisibilityPublic], [VisibilityProtected], [VisibilityPrivate]
	// or [VisibilityHard].
	//
	// Absent for a member with no modifier. Absent and `public` are
	// deliberately distinguishable: they mean the same thing to the
	// compiler, and a backend rendering the source back should not
	// invent a keyword the author omitted.
	MetaVisibility = meta.NewKey(
		"ts.visibility",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaParameterProperty reports a constructor parameter that also
	// declares a field — `constructor(public y: string)`.
	//
	// The declaration is in the parameter list but the member it
	// creates belongs to the class, so a consumer walking Fields and
	// a consumer walking the constructor's Params both need to know
	// which one it is.
	MetaParameterProperty = meta.NewKey(
		"ts.parameterProperty",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaAsync reports an `async` function or method.
	MetaAsync = meta.NewKey(
		"ts.async",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaGenerator reports a generator function — `function*`.
	MetaGenerator = meta.NewKey(
		"ts.generator",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaOverloads lists the overload signatures declared for a
	// function or method, in source order.
	//
	// An overloaded callable is one declaration in the model, not
	// several. TypeScript writes it as N bodiless signatures plus one
	// implementation, and the model keys a declaration by name — so N
	// separate Functions of the same name collide, which is what the
	// store reports as a duplicate qualified name.
	//
	// The surviving declaration carries the implementation's
	// signature in Params and Returns, and the alternatives here. For
	// an ambient overload set, which has no implementation, the first
	// signature is the declaration and the rest are alternatives.
	//
	// A consumer emitting a wrapper renders these signatures and
	// implements the one on the declaration.
	MetaOverloads = meta.NewKey(
		"ts.overloads",
		overloadsParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaAccessor records that a method is a property accessor —
	// [AccessorGet] or [AccessorSet].
	//
	// The model has no accessor kind, so an accessor arrives as a
	// Method. Rendering one as a plain method changes its use site
	// from `o.w` to `o.w()`.
	MetaAccessor = meta.NewKey(
		"ts.accessor",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaConstEnum reports a `const enum`, which is inlined at every
	// use site and emits no runtime object.
	MetaConstEnum = meta.NewKey(
		"ts.constEnum",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaIndexSignature carries an index signature in verbatim
	// source form — `[key: string]: T`.
	//
	// A struct's fields are a fixed set of names; an index signature
	// admits any name matching a key type, which the model has no
	// field for. Text rather than structure because a consumer either
	// renders it back or ignores it, and the parsed form would serve
	// neither better.
	MetaIndexSignature = meta.NewKey(
		"ts.indexSignature",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaConstructSignature carries a `new (…): T` signature from an
	// interface body, in verbatim source form. It describes how to
	// construct the type rather than a member the type has, so it is
	// not a method.
	MetaConstructSignature = meta.NewKey(
		"ts.constructSignature",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaHeritage records why a [node.Embed] is present —
	// [HeritageExtends] or [HeritageImplements].
	//
	// Stamped on the Embed rather than collected on the host, so the
	// fact travels with the entry it describes and a consumer
	// filtering embeds needs no parallel lookup.
	MetaHeritage = meta.NewKey(
		"ts.heritage",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaNullable reports that a type includes `null` or
	// `undefined`. See [MetaOptional] for why that is a different
	// claim from a property being optional.
	MetaNullable = meta.NewKey(
		"ts.nullable",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaTypeText carries the verbatim source text of a type
	// expression with no structured form in the model — conditional
	// types, mapped types, `keyof`, `typeof`, `infer`, and
	// template-literal types.
	//
	// The deliberate limit of the vocabulary. Modelling those
	// structurally means reproducing most of TypeScript's type system
	// inside a language-agnostic package; carrying the text lets a
	// backend round-trip the type and a plugin match on it, while
	// nothing claims to understand it.
	MetaTypeText = meta.NewKey(
		"ts.typeText",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaLiteralType carries the value of a literal type — the
	// `'a'` in `type Tag = 'a'`, or a number, boolean, `null` or
	// `undefined` used in type position.
	//
	// Stamped in verbatim source form, quotes included for a string,
	// so a backend round-trips the literal without re-deciding how to
	// spell it.
	MetaLiteralType = meta.NewKey(
		"ts.literalType",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaMapped reports an `object_type` that is a mapped type —
	// `{ [K in keyof T]: T[K] }` — rather than one carrying a plain
	// index signature.
	//
	// The two are the same node kind in the grammar and differ only
	// by the `in` inside the brackets, so a consumer distinguishing
	// them by shape would have to re-derive what the frontend already
	// determined.
	MetaMapped = meta.NewKey(
		"ts.mapped",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaConstructor reports a `new () => T` constructor type, which
	// arrives as a Func ref and is otherwise indistinguishable from a
	// plain function type.
	MetaConstructor = meta.NewKey(
		"ts.constructor",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaTypeParamDefault carries a generic parameter's default in
	// verbatim source form — the `= {}` in `<T extends object = {}>`.
	//
	// [node.Constraint] models a bound, which Go also has, and not a
	// default, which Go does not.
	MetaTypeParamDefault = meta.NewKey(
		"ts.typeParamDefault",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaInitialiser carries a property or variable initialiser in
	// verbatim source form.
	MetaInitialiser = meta.NewKey(
		"ts.initialiser",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaNamespace records the dotted namespace path a declaration
	// was hoisted out of. Namespaces have no model kind, so their
	// members are flattened into the package and this is what
	// preserves the qualifier a use site needs.
	MetaNamespace = meta.NewKey(
		"ts.namespace",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaModuleSpecifier carries an import's specifier exactly as
	// written, so `./user`, `../lib/user` and `@scope/user` stay
	// distinguishable after [node.Import.Path] has been normalised.
	MetaModuleSpecifier = meta.NewKey(
		"ts.moduleSpecifier",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaTypeOnly reports an import or export that carries only type
	// information — `import type { T } from './t'` — and is erased
	// from the emitted JavaScript.
	//
	// A backend that merges a type-only import into a value import
	// changes what the module does at runtime, which is why this is
	// recorded rather than derived.
	MetaTypeOnly = meta.NewKey(
		"ts.typeOnly",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key
)
