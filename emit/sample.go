// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

// Sample is a value a generated check writes, together with the type
// it is written against.
//
// Here rather than in a language package because every field is one:
// a sample is a value expression bound to a type reference, which is
// what this package models. A generator projecting a declaration
// carries samples through a language-neutral core, and a language
// answers what the values are.
//
// Ref is nil when the text stands alone, which is the builtin case and
// the only one the string-returning forms can serve.
type Sample struct {
	// Ref qualifies the type the text is written against. Rendered
	// through the backend's `renderType`, which resolves the spelling
	// for the file and registers the import it needs.
	Ref Ref

	// Text is the literal. Empty when no sample could be derived,
	// which is the caller's signal to omit the check rather than write
	// one that cannot fail.
	Text string

	// Expr carries a sample no Ref-and-Text pair can spell — a func
	// literal whose signature names several types, a make with the
	// type embedded mid-expression, a constructor call. When non-nil
	// it is the whole sample: Ref and Text are empty, and a consumer
	// renders it through the backend's expression path, which
	// registers every embedded reference's import exactly as it does
	// for a slot-contributed expression.
	Expr *Expr

	// Composite selects the syntax: `Ref{Text}` when true, `Ref(Text)`
	// when false. A conversion and a composite literal are not
	// interchangeable, and which one applies is a property of the type
	// rather than of the value.
	Composite bool

	// Refusal says why Text is empty. [RefusedNone] when a value was
	// derived, so the zero Sample reads as "nothing was attempted"
	// rather than as a refusal with a reason.
	//
	// Meaningful only when [Sample.OK] is false. A caller emitting is
	// not interested; a caller explaining an assertion it declined to
	// write is, and could not otherwise tell an incomplete run from a
	// type that genuinely has no literal.
	Refusal SampleRefusal
}

// OK reports whether a value was derived.
//
// The check a caller makes before emitting: a sample that derived
// nothing must produce no assertion, because an assertion against an
// undefined value passes whatever the subject does.
func (s Sample) OK() bool { return s.Text != "" || s.Expr != nil }

// SampleRefusal says why a sample could not be derived.
//
// Only [RefusedNoLiteral] is a fact about the type. The rest describe
// an input the caller could fix — which matters because the response
// to all of them is to omit the check, so a run missing a package
// silently produces a test that asserts less than it appears to.
type SampleRefusal uint8

const (
	// RefusedNone is the zero value: no refusal, a value was derived.
	RefusedNone SampleRefusal = iota

	// RefusedNoResolver is a nil resolver where a named type needed
	// one, or a nil type reference. The caller's own input.
	RefusedNoResolver

	// RefusedDepth is a walk that hit the recursion budget, which a
	// self-referential type reaches before it terminates.
	RefusedDepth

	// RefusedUnresolved is a named type the resolver could not reach —
	// ordinarily a package the run's patterns did not load.
	RefusedUnresolved

	// RefusedNoLiteral is a type that admits no distinguishable
	// literal: a builtin outside the value table, a struct with no
	// exported settable field, or a declaration with no sample form.
	// The only refusal that is settled rather than fixable.
	RefusedNoLiteral
)

// String names the refusal for diagnostics and error text. A backend
// asked to render a sample carrying nothing embeds it, and a consumer
// explaining a declined check wants the word rather than the ordinal.
func (r SampleRefusal) String() string {
	switch r {
	case RefusedNone:
		return "none"
	case RefusedNoResolver:
		return "no-resolver"
	case RefusedDepth:
		return "depth"
	case RefusedUnresolved:
		return "unresolved"
	case RefusedNoLiteral:
		return "no-literal"
	default:
		return "refusal(?)"
	}
}

// Incomplete reports whether the refusal describes the input rather
// than the type.
//
// The distinction that earns the enum: a caller warning "no check was
// written for this field" wants to say so only when the answer might
// have been different under a wider run. [RefusedNoLiteral] never
// would be.
func (r SampleRefusal) Incomplete() bool {
	return r == RefusedNoResolver || r == RefusedDepth || r == RefusedUnresolved
}
