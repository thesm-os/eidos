// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// Go's enum convention, which is a convention and not a language
// feature.
//
// A Go enum is a defined type plus a block of typed constants.
// Nothing stops a conversion admitting a value outside the set,
// nothing notices when a variant is added without the switch arm it
// needs, and nothing relates the type's textual form to the values
// it was declared with. Every generator emitting an enum surface
// has to decide the same six things, and two in this workspace
// decided them independently — agreeing on most and differing on
// the one that matters.
//
// # The one that matters
//
// For a string-valued enum the textual form is already written
// down. `US Region = "us-east"` renders as `us-east`. Deriving `US`
// from the identifier instead discards the only thing the
// declaration said: a value arriving from JSON, a database column
// or a query parameter no longer parses, and one written out
// carries a spelling no consumer sent — while still round-tripping
// against itself, so a check that only tested the generated pair
// passes.

// EnumForm classifies where a variant's textual form comes from.
type EnumForm string

// The two forms an enum's text can take.
const (
	// FormIdentifier derives the text from the constant's name, with
	// the type's prefix stripped. The only form a numeric enum can
	// take: its declared value is `1`, and rendering `String()` as
	// `"1"` says less than the identifier does.
	FormIdentifier EnumForm = "identifier"

	// FormValue takes the text from the declared constant value. For
	// a string-valued enum the value *is* the textual form.
	FormValue EnumForm = "value"
)

// EnumUnderlying returns the enum's underlying type name, or empty
// when the model records none.
//
// A frontend that produces typeless enums leaves it nil, and an
// enum with no stated underlying type is numeric — the only thing a
// Go const group without an explicit type can be.
func EnumUnderlying(e *node.Enum) string {
	if e == nil || e.Underlying == nil {
		return ""
	}
	return e.Underlying.Name
}

// EnumFallback returns the conversion a generated `String` applies
// to a value outside the declared set, and the printf verb that
// renders the result.
//
// Returned together because the verb follows the conversion and
// nothing else relates the two. `%d` on a set declared over
// `float64` prints `%!d(float64=0.5)`, and `go vet` reports it
// against the consuming repository, where nobody wrote it. Two
// calls — one for the type, one for the verb — is precisely the
// shape that drifts, and it has drifted independently in two
// generators in this workspace.
//
// An [emit.Ref] rather than a name, because a set whose underlying
// type is declared in another package converts through a qualified
// reference and the rendered file has to register the import. Text
// cannot ask for one: a generator composing the conversion from
// [EnumUnderlying] writes `Status(v)` for a `cfg.Status`, which
// names a type the file never imported.
//
// A set recording no underlying type converts through `int` — a Go
// const group without one is an untyped integer, the same
// assumption [OutOfRangeValue] bounds such a set with. A nil enum
// answers the same way rather than yielding a nil ref: the
// conversion is total, so there is no absent answer to report, and
// a nil ref is the one thing a caller cannot render.
func EnumFallback(e *node.Enum) (emit.Ref, string) {
	t := enumFallbackType(e)
	return FromNode(t), FormatVerb(t)
}

// enumFallbackType returns the type an out-of-set value converts
// through — the declared underlying type, or `int` when the model
// records none.
//
// Built fresh per call rather than shared from a package-level
// value: the result is handed to [FromNode], which stamps it as the
// produced ref's origin, and a shared node would then be reachable
// for mutation from every generated ref that ever took this branch.
func enumFallbackType(e *node.Enum) *node.TypeRef {
	if e != nil && e.Underlying != nil {
		return e.Underlying
	}
	return &node.TypeRef{TypeKind: node.TypeRefNamed, Name: typeInt}
}

// ForeignVariants returns, sorted, the import paths of packages
// declaring constants of e's type outside e's own package.
//
// A fact about eidos's own frontend that a consumer otherwise has to
// know: constants are coalesced into an enum only within one package,
// so a `const Extra cfg.Status = 3` declared in another package stays
// a loose [node.Constant] and never reaches [node.Enum.Variants]. It
// is legal Go, and every generated answer about the set is then
// confidently false — `IsValid` rejects a declared value, an arity
// check pins a count that is not the truth, and `String` falls to the
// numeric fallback for a variant that has a name.
//
// Paths rather than the constants themselves, because there is
// nothing useful to do with them: a generator cannot fold a foreign
// constant into a set it does not own, so what it needs is a
// diagnostic naming where to look. Sorted so that diagnostic is
// stable across runs; map iteration order would make the same source
// produce two different messages.
//
// A pure function over a slice rather than a store query, so the
// caller keeps its [store.Reader] and this package stays below it.
func ForeignVariants(e *node.Enum, constants []*node.Constant) []string {
	if e == nil {
		return nil
	}
	want := e.QName()
	seen := make(map[string]struct{})
	for _, c := range constants {
		if c == nil || c.Package == e.Package {
			continue
		}
		if QName(c.Type) == want {
			seen[c.Package] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(seen))
}

// EnumFormOf decides how the enum's variants render as text.
func EnumFormOf(e *node.Enum) EnumForm {
	if EnumUnderlying(e) == typeString {
		return FormValue
	}
	return FormIdentifier
}

// VariantText returns the textual form one variant renders as,
// resolving the three layers highest-precedence first.
//
//  1. A `value` directive on the variant wins outright — the author
//     saying explicitly what the textual form is, for the case
//     where a derived spelling clashes with a protocol's and the
//     derivation cannot be taught about it.
//  2. For a string-valued enum, the declared constant value,
//     unquoted. The value arrives in verbatim source form, so a
//     caller rendering it without unquoting produces
//     `return "\"us-east\""` — which compiles and is wrong.
//  3. Otherwise the identifier with the type's name stripped:
//     `StatusActive` on `type Status int` renders as `Active`. The
//     type name is already context wherever the value appears, and
//     repeating it is noise in every log line and wire payload.
//
// Unquoted text, not a Go literal. A caller writing it into
// generated source quotes it — through [Quote], which escapes.
func VariantText(e *node.Enum, v *node.EnumVariant) string {
	if e == nil || v == nil {
		return ""
	}
	if override, ok := VariantOverride(v); ok {
		return override
	}
	if EnumFormOf(e) == FormValue {
		if unquoted, ok := ParseStringValue(v.Value); ok {
			return unquoted
		}
		// A string enum whose value is not a quoted literal — a
		// constant expression the checker did not fold — falls back to
		// the raw form rather than to the identifier, because the raw
		// form is what the declaration says.
		return v.Value
	}
	return strings.TrimPrefix(v.Name, e.Name)
}

// VariantOverride returns the textual form a `value` directive pins
// on a variant, and whether one was written.
//
// Separated from [VariantText] so a caller can tell an authored
// spelling from a derived one — a diagnostic about a collision
// should say which of the two it can ask the author to change.
func VariantOverride(v *node.EnumVariant) (string, bool) {
	if v == nil {
		return "", false
	}
	for _, d := range v.Directives() {
		if d != nil && d.Name == directive.Name(EnumValueDirective) && len(d.Args) > 0 {
			return d.Args[0], true
		}
	}
	return "", false
}

// EnumValueDirective is the canonical per-variant text override.
//
// Declared as a plain string rather than imported from the pipeline
// so this package stays below it in the dependency order — the same
// reason the `go.*` meta keys are declared here.
const EnumValueDirective = "value"

// EnumTexts returns every variant's textual form in declaration
// order.
//
// Declaration order rather than sorted, because an iota-derived
// enum's order is its numeric order, and a generated switch or
// slice that reordered them would no longer align with the values.
func EnumTexts(e *node.Enum) []string {
	if e == nil {
		return nil
	}
	out := make([]string, 0, len(e.Variants))
	for _, v := range e.Variants {
		out = append(out, VariantText(e, v))
	}
	return out
}

// DuplicateText returns the first textual form two variants share,
// and whether there is one.
//
// Reported rather than generated around: a parse function maps text
// to exactly one variant, so a collision makes one of them
// unreachable through it — and the generated round-trip check fails
// with no indication of the cause.
func DuplicateText(e *node.Enum) (string, bool) {
	seen := make(map[string]struct{}, len(EnumTexts(e)))
	for _, text := range EnumTexts(e) {
		if _, clash := seen[text]; clash {
			return text, true
		}
		seen[text] = struct{}{}
	}
	return "", false
}

// ZeroVariant returns the variant whose declared value is the
// type's zero, and whether the enum declares one.
//
// Worth asking because the zero is what a value of the type holds
// before anything sets it: an enum with no zero variant has an
// unnameable default, which is a fact a generated validity check
// has to state rather than assume.
func ZeroVariant(e *node.Enum) (*node.EnumVariant, bool) {
	if e == nil {
		return nil, false
	}
	for _, v := range e.Variants {
		if v != nil && isZeroValue(v.Value) {
			return v, true
		}
	}
	return nil, false
}

// isZeroValue reports whether a variant's verbatim value is its
// type's zero, in either the numeric or the string spelling.
func isZeroValue(value string) bool {
	return value == litZero || value == litEmpty
}

// EnumValues returns every variant's declared value as an integer,
// and whether all of them read.
//
// All-or-nothing: a caller deriving a bound from the set needs the
// whole set, and a partial read would compute a maximum over the
// variants that happened to parse. False for a string enum, and for
// a numeric one whose values are expressions the type checker did
// not fold.
func EnumValues(e *node.Enum) ([]int64, bool) {
	if e == nil || len(e.Variants) == 0 {
		return nil, false
	}
	out := make([]int64, 0, len(e.Variants))
	for _, v := range e.Variants {
		if v == nil {
			return nil, false
		}
		n, ok := ParseIntValue(v.Value)
		if !ok {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// OutOfRangeText returns a textual form outside the declared set,
// and whether one could be derived.
//
// What a generated check needs to assert that parsing rejects an
// unknown value. For a string enum it is a marker no sensible
// declaration collides with; the collision is checked rather than
// assumed, because a corpus containing the marker would otherwise
// produce a check that passes for the wrong reason.
func OutOfRangeText(e *node.Enum) (string, bool) {
	const marker = "__eidos_unknown__"
	if slices.Contains(EnumTexts(e), marker) {
		return "", false
	}
	if e == nil || len(e.Variants) == 0 {
		return "", false
	}
	return marker, true
}

// OutOfRangeValue returns a numeric value outside the declared set,
// and whether one could be derived.
//
// One past the largest where the type admits it, which is the
// boundary a hand-written fallback most often gets wrong. False for
// a string enum, for one whose values do not read as integers, and
// for one saturating its underlying type — where there is no value
// outside the set to name.
func OutOfRangeValue(e *node.Enum) (int64, bool) {
	if EnumFormOf(e) == FormValue {
		return 0, false
	}
	values, ok := EnumValues(e)
	if !ok {
		return 0, false
	}
	underlying := EnumUnderlying(e)
	if underlying == "" {
		// A const group with no explicit type is an untyped integer,
		// which `int` bounds correctly for this purpose.
		underlying = typeInt
	}
	return NextOutOfRange(underlying, values)
}

// EnumMethods reports which of the conventional enum methods the
// type does not already declare.
//
// The set a generator may emit without shadowing the author's own.
// An author who wrote their own `String` meant to keep it, and a
// generator that refused to run until they deleted it would be
// demanding they give up the more specific statement — so an
// existing declaration is skipped silently rather than reported.
//
// Package-level functions are not in the returned set, because an
// enum node cannot see a same-named function beside it. A caller
// deciding whether to emit `Parse<T>` reads whether `String` is in
// the set: a type keeping its own String almost always keeps its
// own Parse, and generating one that shadows theirs is the worse
// guess.
func EnumMethods(e *node.Enum) []string {
	if e == nil {
		return nil
	}
	candidates := []string{
		MethodString, MethodMarshalText, MethodUnmarshalText,
		MethodMarshalJSON, MethodUnmarshalJSON, MethodValidate,
	}
	out := make([]string, 0, len(candidates))
	for _, name := range candidates {
		if e.MethodByName(name) == nil {
			out = append(out, name)
		}
	}
	return out
}

// EnumDeclares reports whether the enum's type already declares the
// named method.
func EnumDeclares(e *node.Enum, name string) bool {
	return e != nil && e.MethodByName(name) != nil
}

// IsIotaDerived reports whether the enum's values run consecutively
// from zero — the shape an `iota` block produces.
//
// Worth asking because it licenses a generated `IsValid` to be a
// range check rather than a switch, and because a gap in the
// sequence is usually a deleted variant whose value a wire format
// still carries.
//
// False for a string enum and for a set whose values do not read as
// integers, which is the conservative answer: a caller emitting a
// range check for a set that is not contiguous rejects values the
// declaration admits.
func IsIotaDerived(e *node.Enum) bool {
	values, ok := EnumValues(e)
	if !ok || len(values) == 0 {
		return false
	}
	for i, v := range values {
		if v != int64(i) {
			return false
		}
	}
	return true
}

// EnumTextLiteral returns a variant's textual form as a quoted Go
// literal, ready to render.
//
// The quoting belongs here rather than in a template because
// [VariantText] can return a value carrying a quote — an authored
// override is arbitrary text — and a template concatenating quotes
// produces a literal that truncates at the first one.
func EnumTextLiteral(e *node.Enum, v *node.EnumVariant) string {
	return strconv.Quote(VariantText(e, v))
}

// EnumFloatValues returns every variant's declared value as a float,
// and whether all of them read.
//
// [EnumValues] for a set declared over `float32` or `float64`. Split
// rather than generalised because [node.EnumVariant.Value] is source
// text and the model does not record which kind it was written in: a
// generic form would still try one parse and fall back to the other,
// which is this branch spelled less plainly.
//
// All-or-nothing for the same reason as the integer form — a bound
// derived from part of a set is a bound over the variants that
// happened to parse.
func EnumFloatValues(e *node.Enum) ([]float64, bool) {
	if e == nil || len(e.Variants) == 0 {
		return nil, false
	}
	out := make([]float64, 0, len(e.Variants))
	for _, v := range e.Variants {
		if v == nil {
			return nil, false
		}
		n, ok := ParseFloatValue(v.Value)
		if !ok {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// OutOfRangeFloat returns a value outside a float-valued set as Go
// source, and whether one could be derived.
//
// Easier than the integer case, and the reason it is a separate
// function rather than an arm of one: an integer set can exhaust its
// type, so [NextOutOfRange] walks down looking for a gap. A float set
// cannot — the largest declared value plus one is always outside it
// and always representable — so there is no walk and no saturation
// case to report.
//
// False for a set that is not float-valued, which includes an
// integer set: a caller wanting either asks [OutOfRangeLiteral].
func OutOfRangeFloat(e *node.Enum) (string, bool) {
	if EnumFormOf(e) == FormValue || !isFloatEnum(e) {
		return "", false
	}
	values, ok := EnumFloatValues(e)
	if !ok {
		return "", false
	}
	return FormatFloatValue(slices.Max(values) + 1), true
}

// OutOfRangeLiteral returns a value past the declared set as Go
// source, whatever numeric kind the set is declared in.
//
// The form a generator wants: a probe is rendered into source, so the
// caller needs text rather than an `int64` it must then spell — and
// asking which numeric kind the set is declared in, before asking for
// a value outside it, is a question about the library rather than
// about the enum.
//
// Integer first, because an integral literal parses as a float too
// and the narrower reading is the one a set declared over `int` means.
func OutOfRangeLiteral(e *node.Enum) (string, bool) {
	if v, ok := OutOfRangeValue(e); ok {
		return strconv.FormatInt(v, 10), true
	}
	return OutOfRangeFloat(e)
}

// isFloatEnum reports whether e is declared over a float type.
//
// Read from the underlying type rather than inferred from the values,
// because `1` and `2` parse as floats and a set declared over `int`
// must not answer here — [OutOfRangeLiteral] would then return a
// float spelling for an integer set.
func isFloatEnum(e *node.Enum) bool {
	return e != nil && IsFloat(e.Underlying)
}

// EnumInfoOf projects a Go enum into the neutral vocabulary a
// language-neutral generator renders.
//
// Every function above, asked once and answered together. A generator
// calling them one at a time re-derives the relationships between
// them: the form decides where the text comes from, the texts decide
// whether two collide, and the declared values decide what lies
// outside them. Two generators in this workspace asked separately and
// disagreed about the first.
//
// The constants are the run's loose constant declarations, which is
// what lets [emit.EnumInfo.Foreign] report variants declared outside
// the type's own package. A caller with none passes nil.
//
// Every text arrives as a quoted literal rather than as bare text,
// which is where the quoting belongs: an authored override is
// arbitrary text, and a template concatenating quotes around one
// carrying a quote produces a literal that truncates at the first.
func EnumInfoOf(e *node.Enum, constants []*node.Constant) emit.EnumInfo {
	conv, verb := EnumFallback(e)
	dup, _ := DuplicateText(e)
	info := emit.EnumInfo{
		Form:           enumFormOut(e),
		Variants:       enumTextsOut(e),
		Fallback:       conv,
		FallbackFormat: verb,
		Duplicate:      dup,
		Foreign:        ForeignVariants(e, constants),
	}
	if zero, ok := ZeroVariant(e); ok {
		info.Zero = zero.Name
	}
	if text, ok := OutOfRangeText(e); ok {
		info.UnknownText = Quote(text)
	}
	info.OutOfRange = outOfRangeOut(e)
	return info
}

// enumFormOut maps this package's form vocabulary onto the neutral
// one.
//
// A conversion rather than an alias. The two are the same distinction
// and they are not the same type: [EnumForm] classifies a Go
// declaration and [emit.EnumForm] is what a template branches on, and
// tying them together would make a rename here a change to every
// language's contract.
func enumFormOut(e *node.Enum) emit.EnumForm {
	if EnumFormOf(e) == FormValue {
		return emit.EnumFormValue
	}
	return emit.EnumFormIdentifier
}

// enumTextsOut pairs each variant's identifier with its quoted text,
// in declaration order.
func enumTextsOut(e *node.Enum) []emit.EnumText {
	if e == nil {
		return nil
	}
	out := make([]emit.EnumText, 0, len(e.Variants))
	for _, v := range e.Variants {
		if v == nil {
			continue
		}
		out = append(out, emit.EnumText{Name: v.Name, Text: EnumTextLiteral(e, v)})
	}
	return out
}

// outOfRangeOut returns a value past the declared set as Go source,
// whichever kind the set is declared in.
//
// A textual set probes with a marker and a numeric one with a value,
// and the two are different literals — so the branch is here rather
// than in a caller that would have to know which question to ask
// before it could ask it. Empty drops the checks that need a
// boundary, which is the conservative answer: a probe rendered from a
// value the set turns out to declare asserts that a declared variant
// is undeclared.
func outOfRangeOut(e *node.Enum) string {
	if EnumFormOf(e) == FormValue {
		text, ok := OutOfRangeText(e)
		if !ok {
			return ""
		}
		return Quote(text)
	}
	literal, ok := OutOfRangeLiteral(e)
	if !ok {
		return ""
	}
	return literal
}
