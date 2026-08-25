// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/mod/module"

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

// LiteralFor renders text as a literal of t, reporting false when it
// cannot be one.
//
// The step a value read from a struct tag needs and a value read from
// a directive does not. Go's tag grammar consumes one layer of quoting
// to delimit the entry, so `default:"localhost"` reads back as the
// six bare characters an author would have written unquoted — correct
// for a number, a bool or nil, and wrong for a string, whose stamped
// source text still has to read as a string literal. Stamped verbatim
// it names an identifier, and the consumer's build fails on a symbol
// nobody declared.
//
// Inference rather than refusal, because the type settles it. A
// textual member's value is text: quoting it is not a guess about what
// the author meant but a statement of what their type admits. Where
// the text is already a well-formed literal it is passed through, so
// an author who wrote the escaped form gets what they wrote.
//
// False where the text cannot be a literal of t at all — a word in a
// numeric member, a number in a bool. That is the caller's signal to
// report at the declaration rather than let the consumer's compiler
// find it in generated source.
//
// A qualified name whose qualifier f imports is a reference and not
// text. An author writing `default:"seed.Region"` in a file that
// imports seed means the constant, and quoting it stamps the eleven
// characters of its own spelling instead — code that compiles, so
// nothing reports the substitution. The import block is what decides,
// rather than the shape of the text: `example.com` reads as a
// qualified name and is a hostname in a file importing no `example`,
// and only the file separates them. An author whose string value
// really is spelled `pkg.Name` writes the escaped form, which is
// passed through above.
//
// The numeric arms ask less, and take a bare identifier on sight. A
// word is not a value those types admit, so a reference is the only
// reading left; in a textual member it is one of two, which is why
// that one wants evidence.
//
// A type this cannot reason about passes through untouched, on the
// same terms as [IsWellFormedLiteral]: a named constant, a conversion
// and a package-qualified identifier are all things an author writes,
// and none of them can be checked without the scope they resolve in.
func LiteralFor(f *node.File, t *node.TypeRef, text string, r Resolver) (string, bool) {
	if text == "" {
		return "", false
	}
	switch {
	case isTextual(t, r):
		if isQuoted(text) {
			return text, true
		}
		if namesBoundSymbol(f, text) || namesImportPathSymbol(text) {
			return text, true
		}
		return Quote(text), true
	case IsBool(t):
		return text, text == litTrue || text == litFalse
	case IsInteger(t):
		_, ok := ParseIntValue(text)
		return text, ok || namesSymbolText(text)
	case IsNumeric(t):
		return text, isNumericText(text) || namesSymbolText(text)
	}
	return text, true
}

// namesImportPathSymbol reports whether text spells a symbol by the
// full import path of the package declaring it.
//
// The notation that exists because an import written only to feed a
// declared default is an unused import, which does not compile. A file
// naming a constant it uses for nothing else has no qualifier to
// resolve against, so the value carries the path itself.
//
// Split on the last dot, since a path may hold dots and a symbol may
// not. Three things have to hold, where the numeric arms ask for none
// of them, because here every candidate is also a perfectly good
// string:
//
//   - The path holds a slash. `example.com` would otherwise be package
//     `example`, and it is a hostname far more often than it is a
//     single-segment import naming an exported `com`.
//   - The path is one Go would accept. `https://example.com/Foo` is
//     refused on its double slash, which is what keeps a URL a URL.
//   - The symbol is exported, because nothing else can be named from
//     another package at all. That is what leaves `tmpl/index.Html`
//     ambiguous and `tmpl/index.html` plainly a filename.
//
// Together they are shape rather than evidence, which is why the
// exported check is here and not on [namesBoundSymbol]: an import
// block that binds the qualifier has already settled what the text is,
// and a lowercase symbol under a real qualifier is a typo better
// spelled out by the consumer's compiler than quoted into silence.
func namesImportPathSymbol(text string) bool {
	i := strings.LastIndex(text, ".")
	if i <= 0 || i == len(text)-1 {
		return false
	}
	path, symbol := text[:i], text[i+1:]
	if !strings.Contains(path, "/") || !isExportedIdent(symbol) {
		return false
	}
	return module.CheckImportPath(path) == nil
}

// isExportedIdent reports whether s is a Go identifier another package
// can name.
func isExportedIdent(s string) bool {
	for i, r := range s {
		switch {
		case i == 0 && unicode.IsUpper(r):
		case i > 0 && (r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)):
		default:
			return false
		}
	}
	return s != ""
}

// namesBoundSymbol reports whether text names a symbol through a
// qualifier f's import block binds.
//
// A qualifier is required: a bare identifier is what every plain
// string looks like, so accepting one would quote nothing. A name the
// declaring package owns is a reference too, and is recognised before
// this is reached — by the caller, which holds the declarations this
// does not.
//
// Resolution rather than a grammar check, because the two answers
// differ for text that is well-formed either way. `time.Second` in a
// file that imports time is the constant; the same eleven characters
// in a file that does not are eleven characters.
func namesBoundSymbol(f *node.File, text string) bool {
	if qualifier, _ := QualifierOf(text); qualifier == "" {
		return false
	}
	_, err := ResolveQualified(f, text, "")
	return err == nil
}

// isTextual reports whether t's values are written as quoted text,
// following a defined type down to what it is defined as.
func isTextual(t *node.TypeRef, r Resolver) bool {
	if IsString(t) {
		return true
	}
	under := UnderlyingOf(t, r)
	return under != t && IsString(under)
}

// isQuoted reports whether text is already written as a string
// literal, in either of Go's two forms.
//
// An author who escaped the quotes through the tag grammar gets what
// they wrote rather than a second layer around it.
func isQuoted(text string) bool {
	if len(text) < 2 {
		return false
	}
	first, last := text[0], text[len(text)-1]
	return (first == '"' && last == '"') || (first == '`' && last == '`')
}

// isNumericText reports whether text reads as a number.
func isNumericText(text string) bool {
	_, err := strconv.ParseFloat(text, 64)
	return err == nil
}

// namesSymbolText reports whether text reads as an identifier rather
// than a literal, which is what lets a numeric member take a named
// constant — `MaxRetries`, `time.Second` — without this refusing it.
func namesSymbolText(text string) bool {
	for i := range len(text) {
		c := text[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_', c == '.':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return text != "" && (text[0] < '0' || text[0] > '9')
}
