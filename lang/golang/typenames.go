// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

// Go's predeclared type names, spelled once.
//
// Six tables in this package enumerate overlapping subsets of them —
// the keyword set, the predeclared set, the numeric set, the integer
// set, the integer bounds, the bit widths — and every predicate that
// asks "is this the builtin X" spells one too. Written as literals
// at each site they are a rename away from silent disagreement: a
// table that omits `int16` answers wrongly for it rather than not at
// all, which is the failure mode that put `nil` into an int8 field.
//
// The names are constants rather than a single enumerated set
// because the tables answer different questions about one domain.
// Their overlap is real and their membership is not: `byte` is an
// integer and is not a distinct width, `uintptr` is unsigned and is
// not portable, `error` is predeclared and is not a type this
// package can zero without knowing it is an interface.
const (
	typeBool       = "bool"
	typeString     = "string"
	typeError      = "error"
	typeAny        = "any"
	typeComparable = "comparable"

	// typeByte and typeUint8 are Go's two spellings of the same
	// builtin. A frontend records whichever the author wrote, so a
	// predicate checking one misses code written the other way.
	typeByte  = "byte"
	typeUint8 = "uint8"

	typeRune    = "rune"
	typeInt     = "int"
	typeInt8    = "int8"
	typeInt16   = "int16"
	typeInt32   = "int32"
	typeInt64   = "int64"
	typeUint    = "uint"
	typeUint16  = "uint16"
	typeUint32  = "uint32"
	typeUint64  = "uint64"
	typeUintptr = "uintptr"

	typeFloat32    = "float32"
	typeFloat64    = "float64"
	typeComplex64  = "complex64"
	typeComplex128 = "complex128"
)

// Literal spellings of Go's zero values, spelled once for the same
// reason: [ZeroLiteral] produces them and [ZeroValueExpr] switches
// on what it produced, so a divergence between the two is a
// generator emitting an expression that does not match the text it
// documented.
const (
	litNil   = "nil"
	litFalse = "false"
	litEmpty = `""`
	litZero  = "0"
)
