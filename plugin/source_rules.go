// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugin

import (
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// SourceRules is what a generator needs to know about the language a
// declaration was written in.
//
// The read-side counterpart to [TemplateProvider] and
// [FilenameProvider], which answer for the language a run renders. A
// plugin declaring support for a language declares both halves
// together, and a generator uses this one to project a declaration
// into the neutral shape its templates render — which is what lets the
// projection be written once and tested without a backend.
//
// # Which language a caller asks about
//
// Language names are one namespace. A frontend stamps the name of the
// language it parsed onto every package it produces, and a backend
// answers to the same name — so `"golang"` means Go on both sides.
// What differs is which language a caller looks up:
//
//   - A generator projecting a declaration asks about the language of
//     the package that declaration came from.
//   - A backend asks about the language the run renders.
//
// In a run parsing Go and rendering TypeScript those give different
// answers out of the same declaration, which is the point. A plugin
// that looked up the render language to read a Go struct tag would be
// reading source with the wrong language's rules, and both keys are
// strings, so nothing would report it.
//
// # What belongs here
//
// A question whose answer differs per language and that more than one
// generator asks. The type model is the case: a builder, a double and
// a fixture generator all ask what shape a field's type is and what a
// value of it looks like, and each answering privately is how three
// plugins come to disagree about one declaration.
//
// A question only one generator asks belongs to that generator. The
// vocabulary here is deliberately narrow, since a method added is one
// every language that already implements this has to answer.
//
// # What a language may also answer
//
// [EnumRules] and [ErrorRules] are asked about declarations not every
// language has, so a language declares them by satisfying them and a
// generator finds them by asserting:
//
//	er, ok := rules.(sdk.EnumRules)
//
// The same arrangement [TemplateReplacer] and [ImportAwareFuncs] use,
// and for the same reason: required here, they would be methods a
// language with no enumerations has to answer by refusing. A generator
// that asks and is refused reports the language once and generates
// nothing for it, which is the honest outcome — better than a
// projection built from a default nobody chose.
type SourceRules interface {
	DeclarationRules
	TypeRules
	NamingRules
}

// EnumRules is what a language answers about a declared enumeration.
//
// Optional: a language with no enumerated declarations does not
// implement it, and a generator asking finds out by asserting rather
// than by reading an empty answer back.
type EnumRules interface {
	// EnumOf projects a declared enumeration into the neutral form —
	// what each variant renders as, which of them is the zero, what an
	// undeclared value looks like, and what the set as a whole
	// forbids.
	//
	// The constants are the run's loose constant declarations, which
	// is what lets the projection report variants declared outside the
	// type's own package. A caller with none passes nil and gets an
	// empty [emit.EnumInfo.Foreign]; a caller holding the reader
	// passes what it holds.
	//
	// One call rather than a question per fact, because the answers
	// are derived from one another: the form decides where the text
	// comes from, the texts decide whether two collide, and the values
	// decide what lies outside them. Asked separately, a language
	// answers the same question twice and the two drift.
	EnumOf(e *node.Enum, constants []*node.Constant) emit.EnumInfo
}

// ErrorRules is what a language answers about its error protocol.
//
// Optional, for the same reason as [EnumRules]: how a value reports
// failure differs enough between languages that a shared required
// method would be one most of them answer by refusing.
type ErrorRules interface {
	// SentinelName spells the identifier a declared error value is
	// named under — Go's `Err` prefix giving `ErrNotFound` from
	// `NotFound`.
	SentinelName(base string) string

	// IsSentinelName reports whether an identifier follows that
	// convention, which is how a generator finds the declared errors
	// it was pointed at.
	//
	// The inverse of [ErrorRules.SentinelName] and declared beside it,
	// because a generator that emits under one convention and a
	// detector that looks for another is the same rule written twice —
	// disagreeing the first time either is corrected.
	IsSentinelName(ident string) bool

	// ErrorOf projects a declaration into the error contract it
	// carries, reporting false for one that carries none.
	//
	// Matched on the protocol rather than on the name. A declaration
	// with a same-named member that does not satisfy the protocol is
	// not an error, and a check calling it as one does not compile —
	// which puts the failure in the consuming repository rather than a
	// diagnostic in this one.
	//
	// The resolver reaches whatever the declaration folds in, so a
	// family of errors sharing one embedded base is projected from the
	// contract it inherits rather than from the members it happens to
	// restate.
	ErrorOf(s *node.Struct, r node.Resolver) (emit.ErrorInfo, bool)
}

// DeclarationRules is how a declaration written in one language is
// read: what a value in a directive or a tag names, where the
// declaration sits, and which of its members a constructor can set.
type DeclarationRules interface {
	// ResolveValue splits a value written in a directive or a tag into
	// the package it names and the symbol within it, reporting an
	// empty package for a plain literal.
	//
	// The file supplies whatever scope a qualifier resolves against,
	// and may be nil for a positionless declaration — which reports an
	// error rather than guessing against some other file's imports.
	ResolveValue(f *node.File, value string) (pkg, symbol string, err error)

	// Tag returns the named tag entry on a field and whether the field
	// carried it. A language without tags answers false.
	Tag(f *node.Field, key string) (string, bool)

	// FileOf returns the file within pkg that declared n, or nil when
	// the run recorded none.
	//
	// The step from a declaration to the imports in scope for it. A
	// position carries a path while a package keys its files by
	// basename, so a lookup composed at a call site is one truncation
	// away from always missing — silently, which reads as source that
	// imports nothing.
	FileOf(pkg *node.Package, n node.Node) *node.File

	// Settable returns the members of a declaration a constructor in
	// another package can set, in declaration order.
	//
	// Wider than the declared field list wherever a language promotes
	// members into a type. Go embeds, so a struct embedding another
	// has a member named for the embedded type that a composite
	// literal sets as a unit; a language without promotion answers
	// with its visible fields and nothing else.
	Settable(s *node.Struct) []emit.Member
}

// TypeRules is what a language answers about a declared type: the
// structure it has, the values it admits, its zero, and the generic
// parameters it carries.
type TypeRules interface {
	// TypeOf classifies a declared type into the shape vocabulary and
	// lifts whatever inner types that shape has.
	//
	// The resolver reaches named types; pass the reader the plugin was
	// handed. A nil resolver answers for the language's own built-in
	// spellings and reports [emit.ShapeScalar] for anything it cannot
	// reach, which is the conservative answer — one plain setter, one
	// plain assignment.
	TypeOf(t *node.TypeRef, r node.Resolver) emit.TypeInfo

	// SamplesOf returns two distinct values of a type, for a generated
	// check to write.
	//
	// Two rather than one because a check comparing against a single
	// value passes whenever the subject already held it, and what it
	// held is not always knowable. The hint names the declaration the
	// value belongs to, so a value appearing in a failure message says
	// where it came from.
	//
	// Both empty when the type admits no distinguishable literal — ask
	// [emit.Sample.OK] rather than comparing against the zero value.
	SamplesOf(t *node.TypeRef, hint string, r node.Resolver) (sample, alternate emit.Sample)

	// TypeParams lifts a declaration's generic parameter list into the
	// emit form a backend renders, and TypeArgs the same list in use
	// position. Both empty for a declaration with no parameters.
	TypeParams(s *node.Struct) []*emit.TypeParam
	TypeArgs(s *node.Struct) string

	// Witnesses returns one concrete type per parameter, or nil when
	// any parameter carries a constraint that cannot be reasoned
	// about.
	//
	// All-or-nothing, because an entry point instantiates the whole
	// list at once: a witness for one parameter is worth nothing
	// without one for the rest. Nil is the caller's signal that no
	// check can name the types it would run at.
	Witnesses(params []*node.TypeParam) []emit.Ref

	// WitnessArgs renders the witnesses in use position — `[string,
	// int]` — or empty when there are none.
	//
	// Beside [TypeRules.Witnesses] rather than derived from it,
	// because composing the list is a spelling: which brackets, which
	// separator, and whether the arguments are written at all. A
	// generator building the text itself writes Go's answer into
	// every language's output.
	WitnessArgs(params []*node.TypeParam) string

	// ZeroLiteral returns the language's spelling of a type's zero
	// value, and whether one could be derived.
	//
	// What a generator compares a declared default against. A default
	// that *is* the zero makes a check assert `0 == 0`, which passes
	// against a constructor that ignored the declaration entirely —
	// the one thing that check exists to notice. The spellings differ
	// per language, `nil` against `None` among them, so a core
	// carrying its own table would be answering for one.
	ZeroLiteral(t *node.TypeRef, r node.Resolver) (string, bool)
}

// NamingRules is how a language spells an identifier a generator
// composes from parts.
type NamingRules interface {
	// TypeName joins parts into the identifier a generated type is
	// declared under — `User` and `Builder` giving `UserBuilder`.
	//
	// The word is the generator's vocabulary; the spelling is the
	// language's. Go joins in PascalCase and exports by the leading
	// rune, another language lowercases and separates. A generator
	// concatenating the parts itself hard-codes one language's answer
	// into a core that names no language.
	TypeName(parts ...string) string

	// ConstructorName spells the identifier of the function that
	// builds a value of the named type.
	//
	// Go's `New` prefix is a convention rather than a rule, and it is
	// not every language's: a generator composing it would emit
	// `NewUser` into a language whose constructors are called
	// something else entirely.
	ConstructorName(base string) string
}
