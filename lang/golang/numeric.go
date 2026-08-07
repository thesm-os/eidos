// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"math"
	"strconv"

	"go.thesmos.sh/eidos/node"
)

// Facts about Go's integer types that a generator emitting values
// has to know and cannot derive from the model.
//
// A generator writing a value outside a declared enum's set, or
// checking that a constant fits the type it is declared against,
// needs the type's range. Nothing in the node model carries it —
// the frontend records the type's name and the constant's printed
// form — so each generator that needed it wrote a table, and a
// table that omits a width answers wrongly for it rather than not
// at all.

// bounds is the closed range of each builtin integer type.
//
// Platform-dependent widths — `int`, `uint`, `uintptr` — take the
// 64-bit range. Generated code is compiled somewhere this tool is
// not, so the host's width is not the target's; the wider range is
// the one that does not reject a value the target accepts.
//
//nolint:gochecknoglobals // closed language-defined table
var bounds = map[string]struct{ min, max int64 }{
	typeInt:     {math.MinInt64, math.MaxInt64},
	typeInt8:    {math.MinInt8, math.MaxInt8},
	typeInt16:   {math.MinInt16, math.MaxInt16},
	typeInt32:   {math.MinInt32, math.MaxInt32},
	typeInt64:   {math.MinInt64, math.MaxInt64},
	typeRune:    {math.MinInt32, math.MaxInt32},
	typeUint:    {0, math.MaxInt64},
	typeUint8:   {0, math.MaxUint8},
	typeUint16:  {0, math.MaxUint16},
	typeUint32:  {0, math.MaxUint32},
	typeUint64:  {0, math.MaxInt64},
	typeUintptr: {0, math.MaxInt64},
	typeByte:    {0, math.MaxUint8},
}

// bitSizes is the declared width of each fixed-width builtin.
//
// The platform-dependent three are absent deliberately: their width
// is the target's to decide, and answering 64 would be a claim this
// package cannot support.
//
//nolint:gochecknoglobals // closed language-defined table
var bitSizes = map[string]int{
	typeInt8: 8, typeInt16: 16, typeInt32: 32, typeInt64: 64, typeRune: 32,
	typeUint8: 8, typeUint16: 16, typeUint32: 32, typeUint64: 64, typeByte: 8,
	typeFloat32: 32, typeFloat64: 64,
	typeComplex64: 64, typeComplex128: 128,
}

// NumericBounds returns the closed range of a builtin integer type.
//
// The third result distinguishes "not an integer type" from a range
// that happens to include zero. `uint64` reports [math.MaxInt64]
// rather than its true maximum, because the range is expressed in
// int64 and the alternative is a second signed/unsigned API for a
// bound no generated enum reaches.
func NumericBounds(typeName string) (minValue, maxValue int64, ok bool) {
	b, found := bounds[typeName]
	if !found {
		return 0, 0, false
	}
	return b.min, b.max, true
}

// FitsIn reports whether v is representable in the named builtin
// integer type.
//
// False for a type this package does not recognise, which is the
// conservative answer: a caller emitting a value it cannot prove
// fits should emit nothing rather than a constant the compiler
// rejects in the consumer's build.
func FitsIn(v int64, typeName string) bool {
	lo, hi, ok := NumericBounds(typeName)
	return ok && v >= lo && v <= hi
}

// IsUnsigned reports whether the named builtin integer type has no
// negative values.
//
// The question behind "may I write -1 here": a generator deriving a
// sentinel outside a declared set reaches for the value below the
// minimum, and for an unsigned type there is none.
func IsUnsigned(typeName string) bool {
	lo, _, ok := NumericBounds(typeName)
	return ok && lo == 0
}

// BitSize returns a fixed-width builtin's declared width in bits.
//
// The platform-dependent widths — `int`, `uint`, `uintptr` — report
// false rather than guessing: the width belongs to the target
// build, which is not this process.
func BitSize(typeName string) (int, bool) {
	n, ok := bitSizes[typeName]
	return n, ok
}

// NextOutOfRange returns a value of the named integer type that is
// not in used, and whether one exists.
//
// A generated check asserting "a value outside the declared set is
// rejected" needs a value the set does not hold. One past the
// largest is the boundary a hand-written fallback is most likely to
// get wrong, so that is tried first; when the largest is already
// the type's maximum the search walks down from it instead, and
// only a set that saturates the whole type has no answer.
//
// Used values outside the type's range are ignored rather than
// rejected — the declaration is the caller's to validate, and a
// value that does not fit cannot collide with one that does.
func NextOutOfRange(typeName string, used []int64) (int64, bool) {
	lo, hi, ok := NumericBounds(typeName)
	if !ok {
		return 0, false
	}
	taken := make(map[int64]struct{}, len(used))
	// Tracked with a flag rather than by seeding `highest` below the
	// minimum: for `int` the minimum is [math.MinInt64], and one less
	// than that wraps to the maximum — which would report the whole
	// range saturated for a set of three small values.
	var highest int64
	seen := false
	for _, v := range used {
		if v < lo || v > hi {
			continue
		}
		taken[v] = struct{}{}
		if !seen || v > highest {
			highest, seen = v, true
		}
	}
	if !seen {
		return lo, true
	}
	if highest < hi {
		return highest + 1, true
	}
	// Saturated at the top: walk down for the first gap. Bounded by
	// the type's range, so a fully-populated int8 terminates and a
	// fully-populated int64 is not reachable by any real declaration.
	for v := hi; v >= lo; v-- {
		if _, clash := taken[v]; !clash {
			return v, true
		}
		if v == lo {
			break
		}
	}
	return 0, false
}

// FormatVerb returns the printf verb that renders t faithfully.
//
// A generated failure message interpolates the value it compared,
// and `%v` on a string loses the quoting that shows a trailing
// space or an empty value — which is exactly the difference a
// failing assertion is trying to explain. Strings therefore take
// `%q`, floats `%g` so a small magnitude does not print as zero,
// and everything unrecognised falls back to `%v`.
func FormatVerb(t *node.TypeRef) string {
	switch {
	case IsString(t):
		return "%q"
	case IsBool(t):
		return "%t"
	case IsFloat(t):
		return "%g"
	case IsInteger(t):
		return "%d"
	default:
		return "%v"
	}
}

// ParseIntValue reads a constant's recorded value as an integer.
//
// [node.Constant.Value] and [node.EnumVariant.Value] hold the
// verbatim source form the type checker printed, so an integer
// arrives bare and a string arrives quoted. The second result is
// false for anything that is not a plain integer — a float, a
// character literal, an expression the checker did not fold — which
// is the caller's signal that a numeric derivation is unavailable
// rather than zero.
func ParseIntValue(raw string) (int64, bool) {
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// ParseStringValue reads a constant's recorded value as a string.
//
// The value arrives quoted, so a caller rendering it into generated
// source without unquoting produces `"\"us-east\""` — which
// compiles and is wrong. False for a value that is not a quoted
// string.
func ParseStringValue(raw string) (string, bool) {
	v, err := strconv.Unquote(raw)
	if err != nil {
		return "", false
	}
	return v, true
}
