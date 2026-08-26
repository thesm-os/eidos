// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript

import (
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// unknownText is the textual form a generated check submits to assert
// that parsing refuses a value naming no variant.
//
// Deliberately unwriteable as a TypeScript identifier, so a
// declaration cannot collide with it by accident. The collision is
// still checked, because a string enum's values are arbitrary text
// and one of them may be this.
const unknownText = "__eidos_unknown__"

// EnumOf is [Source.EnumOf] as a plain function.
func EnumOf(e *node.Enum, _ []*node.Constant) emit.EnumInfo {
	if e == nil {
		return emit.EnumInfo{}
	}

	stringly := isStringEnum(e)
	info := emit.EnumInfo{
		Form:     enumForm(stringly),
		Variants: enumTexts(e, stringly),
		Fallback: enumFallback(stringly),
		Zero:     enumZero(e, stringly),
	}
	info.Duplicate = firstDuplicate(info.Variants)
	info.UnknownText = enumUnknown(info.Variants)
	info.OutOfRange = enumOutOfRange(e, stringly)
	return info
}

// isStringEnum reports whether the enum's members are assigned
// strings.
//
// Read from the variants rather than from Underlying, because a
// declaration reaching this projection from a bridge may carry no
// underlying type at all — and the values are the fact either way. A
// member with no value is numeric by definition: the implicit value
// comes from a counter.
func isStringEnum(e *node.Enum) bool {
	stringly := false
	for _, v := range e.Variants {
		if v == nil {
			continue
		}
		if v.Value == "" || !quoted(v.Value) {
			return false
		}
		stringly = true
	}
	return stringly
}

// enumForm classifies where the variants' textual forms come from.
//
// A string enum's declared value *is* its textual form — that is what
// distinguishes `Role.Admin = 'admin'` from `Role.Admin` — while a
// numeric one carries no text of its own, so the identifier is all
// there is.
func enumForm(stringly bool) emit.EnumForm {
	if stringly {
		return emit.EnumFormValue
	}
	return emit.EnumFormIdentifier
}

// enumTexts pairs each variant with the literal its textual form
// renders as.
//
// Re-quoted rather than passed through: the source may have written
// `"admin"` or a template literal, and a generated file has one quote
// style. Quoting is applied here because it is the language's rule.
func enumTexts(e *node.Enum, stringly bool) []emit.EnumText {
	out := make([]emit.EnumText, 0, len(e.Variants))
	for _, v := range e.Variants {
		if v == nil || v.Name == "" {
			continue
		}
		text := v.Name
		if stringly {
			text = unquote(v.Value)
		}
		out = append(out, emit.EnumText{Name: v.Name, Text: Quote(text)})
	}
	return out
}

// enumFallback is the type an undeclared value carries.
//
// TypeScript's enum members are strings or numbers already, so a
// value outside the declared set needs no conversion to be printed —
// it is the underlying type. [emit.EnumInfo.FallbackFormat] stays
// empty for the same reason: the language has no format tokens.
func enumFallback(stringly bool) emit.Ref {
	if stringly {
		return emit.Builtin(ScalarString)
	}
	return emit.Builtin(ScalarNumber)
}

// enumZero names the variant holding the type's zero.
//
// Only a numeric enum has one. Its members run from an implicit
// counter that starts at zero and resumes from the last declared
// value, which is the rule the frontend deliberately does not apply —
// it records what the source wrote — so it is applied here.
//
// A member with a non-numeric declared value stops the walk rather
// than being skipped: a computed member makes every value after it
// unknowable, and reporting a zero derived past one would name the
// wrong variant.
func enumZero(e *node.Enum, stringly bool) string {
	if stringly {
		return ""
	}
	next := 0
	for _, v := range e.Variants {
		if v == nil || v.Name == "" {
			continue
		}
		if v.Value != "" {
			declared, err := strconv.Atoi(strings.TrimSpace(v.Value))
			if err != nil {
				return ""
			}
			next = declared
		}
		if next == 0 {
			return v.Name
		}
		next++
	}
	return ""
}

// firstDuplicate returns the first textual form two variants share.
//
// A parser maps text to exactly one variant, so a collision makes one
// of them unreachable — reported rather than generated around,
// because the generated round trip otherwise fails without naming the
// cause.
func firstDuplicate(texts []emit.EnumText) string {
	seen := make(map[string]struct{}, len(texts))
	for _, t := range texts {
		if _, clash := seen[t.Text]; clash {
			return t.Text
		}
		seen[t.Text] = struct{}{}
	}
	return ""
}

// enumUnknown returns a textual form outside the declared set.
//
// Empty when the declared set turns out to contain the marker, which
// is the one case where the probe would assert the opposite of what
// it means.
func enumUnknown(texts []emit.EnumText) string {
	want := Quote(unknownText)
	for _, t := range texts {
		if t.Text == want {
			return ""
		}
	}
	return want
}

// enumOutOfRange returns a value past the declared set.
//
// One past the highest declared member of a numeric enum. Empty for a
// string enum, whose values have no ordering to be past, and for a
// numeric one carrying a member this projection cannot read — both
// drop the checks that need a boundary rather than writing them
// against a value the set may turn out to declare.
func enumOutOfRange(e *node.Enum, stringly bool) string {
	if stringly || len(e.Variants) == 0 {
		return ""
	}
	next, high := 0, 0
	for _, v := range e.Variants {
		if v == nil || v.Name == "" {
			continue
		}
		if v.Value != "" {
			declared, err := strconv.Atoi(strings.TrimSpace(v.Value))
			if err != nil {
				return ""
			}
			next = declared
		}
		if next > high {
			high = next
		}
		next++
	}
	return strconv.Itoa(high + 1)
}

// quoted reports whether a verbatim value is a quoted string.
func quoted(v string) bool {
	if len(v) < 2 {
		return false
	}
	q := v[0]
	return (q == '\'' || q == '"' || q == '`') && v[len(v)-1] == q
}

// unquote strips the surrounding quotes from a verbatim string value,
// leaving anything else untouched.
func unquote(v string) string {
	if !quoted(v) {
		return v
	}
	return v[1 : len(v)-1]
}
