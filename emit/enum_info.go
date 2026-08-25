// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

// The vocabulary a generator carries between a language and its
// templates when the declaration is an enumeration.
//
// [TypeInfo]'s counterpart for a second kind of declaration, and here
// for the same two reasons: a language package cannot import the
// plugin façade, and a neutral core must not import a language
// package. Both name these.
//
// Beside [Enum] rather than folded into it. That is the emit model —
// an enumeration a generator is *writing* — while this is what a
// language answers about one it *read*, which is a different set of
// facts with a different lifetime.
//
// What an enumeration owes is not derivable from its variants alone.
// Which of them is the type's zero, what an undeclared value renders
// as, whether a variant's textual form comes from its identifier or
// from the value it was declared with — each is a question about the
// language the declaration was written in, and each has been answered
// two different ways by two generators in one workspace.

// EnumForm classifies where a variant's textual form comes from.
type EnumForm string

// The two forms. [EnumFormIdentifier] is the zero value, so a
// projection that never classified reads as "the identifier is all
// there is" — the answer that holds wherever the declared value is a
// number, which is the common case.
const (
	// EnumFormIdentifier derives the text from the variant's
	// identifier. The only form available where the declared value
	// carries no text of its own: rendering a numeric variant as `1`
	// says less than its name does.
	EnumFormIdentifier EnumForm = ""

	// EnumFormValue takes the text from the declared value, which for
	// a textual enumeration *is* the textual form.
	//
	// Worth distinguishing because deriving the identifier instead
	// still round-trips against its own parser — so a check testing
	// only the generated pair passes, while every value arriving from
	// outside the program fails to parse.
	EnumFormValue EnumForm = "value"
)

// EnumText is one variant's identifier paired with the literal its
// textual form renders as.
//
// A literal rather than the bare text: an authored override is
// arbitrary text, and a template concatenating quotes around one
// carrying a quote produces a literal that truncates at the first.
// Quoting is the language's rule, so the language applies it.
type EnumText struct {
	// Name is the variant's identifier.
	Name string

	// Text is its textual form as a literal, ready to render.
	Text string
}

// EnumInfo is what a language answers about one declared
// enumeration.
//
// Every field a generator needs to decide what it may honestly
// assert. The absent answers are empty rather than flagged, because
// each has the same meaning for a caller: there is no value to write,
// so the check that would have used it is not emitted. A check
// written against a guess asserts something the declaration does not
// say.
type EnumInfo struct {
	// Form is where the variants' textual forms come from.
	Form EnumForm

	// Variants are the declared variants in declaration order, each
	// with the literal it renders as.
	//
	// Declaration order rather than sorted: where values run from an
	// implicit counter, declaration order *is* numeric order, and a
	// rendered switch that reordered them would no longer align.
	Variants []EnumText

	// Fallback is the type an undeclared value converts through
	// before anything prints it.
	//
	// A reference rather than a name, because an enumeration declared
	// over another package's type converts through a qualified
	// reference and the rendered file has to register the import.
	// Text cannot ask for one: a generator composing the conversion
	// from an identifier emits a file naming a package it never
	// imported.
	Fallback Ref

	// FallbackFormat is the format token that prints a value of
	// [EnumInfo.Fallback] faithfully, empty for a language with no
	// format tokens.
	//
	// Paired with the conversion rather than derived beside it,
	// because the two drift: a token chosen for integers, applied to
	// a value converted to a float, prints a diagnostic where the
	// value should be — in the consuming repository, where nobody
	// wrote it.
	FallbackFormat string

	// Zero names the variant whose declared value is the type's zero,
	// empty when no variant holds it.
	//
	// The zero is what a value of the type holds before anything sets
	// it, so which variant it is — or that it is none of them — is
	// the one fact a validity check cannot assume. The two cases read
	// as opposite assertions.
	Zero string

	// Duplicate is the first textual form two variants share, empty
	// when each is distinct.
	//
	// Reported rather than generated around: a parser maps text to
	// exactly one variant, so a collision makes one of them
	// unreachable — and the generated round trip then fails without
	// naming the cause.
	Duplicate string

	// Foreign lists, sorted, the packages declaring variants of this
	// type outside the one that declares the type.
	//
	// Legal in a language that admits them, and silently wrong here:
	// a set the projection cannot see makes every generated answer
	// about that set confidently false.
	Foreign []string

	// UnknownText is a textual form outside the declared set as a
	// literal, empty when none could be derived.
	//
	// What a check submits to assert that parsing refuses text naming
	// no variant. Empty where the declared set turns out to contain
	// the marker a language would otherwise use — the one case where
	// the probe would assert the opposite of what it means.
	UnknownText string

	// OutOfRange is a value past the declared set as a literal, empty
	// when none could be derived.
	//
	// Empty for a set saturating its type, and for one whose values
	// the projection cannot read. Both drop the checks that need a
	// boundary rather than writing them against a value the set may
	// turn out to declare.
	OutOfRange string
}
