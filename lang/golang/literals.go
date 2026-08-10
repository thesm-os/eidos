// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/node"
)

// ErrMalformedLiteral is returned by [IsWellFormedLiteral] for text
// that cannot be stamped into generated Go as a value expression.
var ErrMalformedLiteral = errors.New("golang: malformed literal")

// IsWellFormedLiteral reports whether src can be stamped into
// generated Go as a value expression, returning
// [ErrMalformedLiteral] wrapped with what is wrong when it cannot.
//
// Deliberately shallow, on the same terms as Go's own
// `module.CheckImportPath`: it rejects what would produce a file the
// toolchain cannot *parse* — an empty value, an unterminated string,
// raw string or rune literal — and passes everything else to the
// consumer's compiler, which can resolve the named constants,
// conversions and package-qualified identifiers this cannot.
//
// Shallow is the whole design, not a limitation admitted. A deeper
// check would need the type the value is stamped against and the
// scope it resolves in, neither of which this package has; refusing
// what it cannot verify would reject `time.Second`, `MaxRetries` and
// every conversion an author legitimately writes into a directive.
// The failure it does prevent is the one with no attribution: an
// unbalanced quote does not fail at the directive, it fails as a
// syntax error somewhere else in a file the author never wrote.
//
// A value that parses here can still fail to compile there. That is
// the correct division — the compiler's message names the consumer's
// own type, and a guess made here would name neither.
func IsWellFormedLiteral(src string) error {
	if src == "" {
		return fmt.Errorf("%w: empty", ErrMalformedLiteral)
	}
	if strings.TrimSpace(src) == "" {
		return fmt.Errorf("%w: %q is only whitespace", ErrMalformedLiteral, src)
	}
	return checkQuoting(src)
}

// checkQuoting reports an unterminated quoted form.
//
// Scans rather than reaching for [strconv.Unquote], which answers a
// different question: it rejects a *concatenation* (`"a" + "b"`) and
// a literal with anything around it, both of which are valid value
// expressions an author writes. What matters here is only that every
// quote a value opens, it closes — an unbalanced one swallows the
// rest of the generated file.
//
// Escapes are honoured inside interpreted strings and rune literals
// so `"a\""` is not read as closing early; a raw string takes no
// escapes, which is what makes its scan the simplest of the three.
func checkQuoting(src string) error {
	for i := 0; i < len(src); i++ {
		var closed bool
		switch src[i] {
		case '`':
			i, closed = scanRaw(src, i)
		case '"', '\'':
			i, closed = scanEscaped(src, i)
		default:
			continue
		}
		if !closed {
			return fmt.Errorf("%w: %q has an unterminated quoted value", ErrMalformedLiteral, src)
		}
	}
	return nil
}

// scanRaw advances past a raw string literal opened at start,
// returning the index of its closing backquote and whether one was
// found. A raw string ends at the next backquote and honours no
// escapes at all.
func scanRaw(src string, start int) (end int, closed bool) {
	if i := strings.IndexByte(src[start+1:], '`'); i >= 0 {
		return start + 1 + i, true
	}
	return len(src), false
}

// scanEscaped advances past an interpreted string or rune literal
// opened at start, returning the index of its closing quote and
// whether one was found. A backslash escapes the next byte, so a
// quote following one does not close the literal.
func scanEscaped(src string, start int) (end int, closed bool) {
	quote := src[start]
	for i := start + 1; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case quote:
			return i, true
		}
	}
	return len(src), false
}

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
