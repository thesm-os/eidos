// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/node"
)

// numericBuiltins is every Go builtin whose zero value is 0.
//
// Listed rather than pattern-matched on an `int`/`uint`/`float`
// prefix, because the prefixes admit names that are not builtins —
// a consumer's own `integer` type would match one — and because the
// set is closed by the language spec.
//
//nolint:gochecknoglobals // closed language-defined set
var numericBuiltins = map[string]struct{}{
	typeInt: {}, typeInt8: {}, typeInt16: {}, typeInt32: {}, typeInt64: {},
	typeUint: {}, typeUint8: {}, typeUint16: {}, typeUint32: {}, typeUint64: {},
	typeUintptr: {}, typeByte: {}, typeRune: {},
	typeFloat32: {}, typeFloat64: {}, typeComplex64: {}, typeComplex128: {},
}

// ZeroLiteral returns the Go source text for a type's zero value,
// and whether one is derivable.
//
// The second result is the point. A generator writing a composite
// literal needs a value for every field it names, and guessing when
// it does not know produces code that does not compile — `nil` for
// an `int8` field is the shape this exists to prevent, and it is
// what a partial private copy of this table produced.
//
// Derivable for the builtins and for the reference shapes whose
// zero is `nil`. Not derivable for a named non-interface type: the
// zero of a struct is `T{}` and of a defined numeric type is `0`,
// and the model records only a name — so the caller, which may be
// able to resolve it, decides.
func ZeroLiteral(t *node.TypeRef) (string, bool) {
	if t == nil {
		return "", false
	}
	switch {
	case t.IsPointer(), t.IsSlice(), t.IsMap(), t.IsFunc(), t.IsAnonInterface():
		return litNil, true
	case IsChannel(t):
		return litNil, true
	case t.IsArray():
		// An array's zero is a composite literal of itself, which
		// needs the element spelling the render site owns.
		return "", false
	case t.IsAnonStruct():
		return "", false
	}
	if !t.IsBuiltin() {
		// An interface's zero is nil whatever it is named; anything
		// else named needs resolution the model cannot do.
		if IsInterface(t) || IsError(t) {
			return litNil, true
		}
		return "", false
	}
	switch t.Name {
	case typeBool:
		return litFalse, true
	case typeString:
		return litEmpty, true
	case typeError, typeAny:
		return litNil, true
	}
	if _, ok := numericBuiltins[t.Name]; ok {
		return litZero, true
	}
	return "", false
}

// StructTag assembles a Go struct-tag literal from its entries.
//
// Entries render in the supplied order rather than sorted, because
// a struct tag is read by humans as often as by reflection and the
// conventional order — `json` first, then the rest — carries
// meaning no sort reproduces.
//
// The value is quoted with Go's own escaping, so a value containing
// a quote produces a valid tag rather than one that truncates at
// the first `"`. The result excludes the surrounding backticks: the
// backend owns the quoting of the tag as a whole, and a caller
// embedding backticks here would produce a literal it cannot nest.
func StructTag(entries ...TagEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	for i, e := range entries {
		if e.Key == "" {
			continue
		}
		if i > 0 && b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(e.Key)
		b.WriteByte(':')
		b.WriteString(strconv.Quote(e.Value))
	}
	return b.String()
}

// TagEntry is one key-value pair of a struct tag.
type TagEntry struct {
	// Key is the tag namespace — "json", "db", "yaml".
	Key string

	// Value is the unquoted content; [StructTag] quotes it.
	Value string
}
