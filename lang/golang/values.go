// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"reflect"
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/node"
)

// Values a generator writes into emitted source, and the tags it
// reads them out of.
//
// [ZeroLiteral] and [StructTag] live in literals.go. What is here
// is the rest: the sample pair a generated check needs, the
// resolver-backed zero for a named type, and tag parsing.
//
// # Zero and sample are different questions
//
// A zero is what a field holds when nobody set it. A sample is a
// value distinguishable from whatever it held before — and a check
// comparing against a single value passes whenever the subject
// already held it, which is not always knowable. Conflating the two
// produces a check that passes against code doing nothing, which is
// worse than no check because it reads as coverage.

// Resolver answers what a named type is, over the declarations a
// run loaded.
//
// [ZeroLiteral] cannot answer for a named type: the model records a
// package and an identifier, so `Weekday` gives no clue that it is
// an integer. A caller holding the graph can resolve it, and this
// is the narrow interface that lets one — satisfied by a
// store-backed index without this package depending on the store.
//
// `store.Reader.Resolve` is that index, so a plugin passes the reader
// it was handed and writes no adapter: `SampleFor(t, name,
// ctx.Reader)`. Every function here taking a Resolver accepts it.
type Resolver interface {
	// Resolve returns the declaration a type reference names, and
	// whether the run loaded one. A type from a package the run
	// never read reports false, which is a smaller answer rather
	// than a wrong one.
	Resolve(t *node.TypeRef) (node.Node, bool)
}

// maxResolveDepth bounds the walk through named types.
//
// A defined type may name another, and a struct may hold a field of
// its own type — `Manager *User` inside `User` — so the walk needs
// a stop. Ten is far past any real alias chain and small enough
// that a cycle terminates in a generation pass rather than a stack
// overflow.
const maxResolveDepth = 10

// SampleValues returns two distinct values of the named Go builtin
// as source text, or two empty strings when the type admits none.
//
// Two rather than one, because a check comparing against a single
// value passes whenever the subject already held it, and what it
// held is not always knowable — a constructor's seed may come from
// a function the generator cannot read. Whatever it was equals at
// most one of a pair.
//
// The string form carries the field's own name, so a value
// appearing in a failure message says where it came from.
//
// Source text rather than a reference because every value is a
// builtin literal: it names no package, so nothing here can produce
// something the rendered file would have to import.
func SampleValues(typeName, fieldName string) (sample, alternate string) {
	lower := strings.ToLower(fieldName)
	switch typeName {
	case typeString:
		return `"test-` + lower + `"`, `"other-` + lower + `"`
	case typeBool:
		// The only type whose pair exhausts its values, which makes a
		// bool check the strictest of them: code that assigned nothing
		// fails against one arm no matter what was there before.
		return "true", "false"
	case typeFloat32, typeFloat64:
		return "3.14", "2.72"
	case typeComplex64, typeComplex128:
		return "1 + 2i", "3 + 4i"
	}
	if _, ok := integerBuiltins[typeName]; ok {
		return "42", "7"
	}
	return "", ""
}

// SampleFor returns a sample pair for a source type, resolving
// named types through r.
//
// A builtin answers directly. A defined type answers through its
// underlying type and keeps its own spelling — `Weekday(42)` rather
// than a bare 42, which compiles today and stops compiling the
// moment the field's type moves. A type the resolver cannot reach
// yields two empty strings, which is the caller's signal to omit
// the check rather than write one that cannot fail.
//
// Pass a nil resolver to answer for builtins only.
//
// # Only what a string can spell
//
// A type from another package cannot be written into generated source
// as a string: the spelling depends on the file the value lands in,
// and the import has to be registered. This used to compose it with
// [QName] and emit `example.com/cfg.Weekday(42)`, which is not Go and
// registers nothing. Such a type now yields two empty strings — the
// same "omit the check" signal an unresolvable type gives, and honest
// where the old answer was not.
//
// [SampleRefFor] is the form that answers for those: it returns the
// reference beside the text, which is what lets the backend spell it
// for the file and register the import.
func SampleFor(t *node.TypeRef, fieldName string, r Resolver) (sample, alternate string) {
	return sampleFor(t, fieldName, r, maxResolveDepth)
}

// spellableAsString reports whether t can be written into generated
// source without an import being registered for it.
//
// A reference carrying no package is either a builtin or local to
// whatever file the caller is emitting, so its name is its spelling.
// Anything else needs [SampleRefFor] or [ZeroRefFor].
func spellableAsString(t *node.TypeRef) bool {
	return t != nil && t.Package == ""
}

// sampleFor is [SampleFor] over [SampleRefFor], keeping only the
// answers a string can carry.
//
// One walk rather than two. Every type this can answer for,
// [SampleRefFor] answers with a nil Ref — and every type it answers
// with a Ref is one a string cannot spell, because the Ref is exactly
// the import that would have to be registered. An [Sample.Expr]
// sample needs no clause of its own: its Text is empty by
// construction, so it flows out as the same empty-string refusal.
func sampleFor(t *node.TypeRef, fieldName string, r Resolver, depth int) (sample, alternate string) {
	s, a := sampleRefFor(t, fieldName, r, depth)
	if s.Ref != nil || a.Ref != nil {
		return "", ""
	}
	return s.Text, a.Text
}

// ZeroLiteralFor returns a type's zero as source text, resolving
// named types through r.
//
// The answer [ZeroLiteral] refuses: given the declaration behind
// `Weekday`, the zero is `0` and behind a struct it is `T{}`. A
// caller without a resolver keeps the narrower answer, which is
// correct rather than absent.
func ZeroLiteralFor(t *node.TypeRef, r Resolver) (string, bool) {
	return zeroLiteralFor(t, r, maxResolveDepth)
}

// zeroLiteralFor is [ZeroLiteralFor] with the recursion budget
// threaded through.
func zeroLiteralFor(t *node.TypeRef, r Resolver, depth int) (string, bool) {
	if lit, ok := ZeroLiteral(t); ok {
		return lit, true
	}
	if t == nil || r == nil || depth <= 0 {
		return "", false
	}
	if t.IsArray() {
		// An array's zero is a composite literal of itself, which needs
		// the element spelling — available here where it was not.
		if elem, _ := ArrayElem(t); spellableAsString(elem) {
			return "[" + strconv.Itoa(t.ArrayLen) + "]" + QName(elem) + "{}", true
		}
		return "", false
	}
	if t.TypeKind != node.TypeRefNamed {
		return "", false
	}
	target, found := r.Resolve(t)
	if !found {
		return "", false
	}
	switch decl := target.(type) {
	case *node.Alias:
		inner, ok := zeroLiteralFor(decl.Target, r, depth-1)
		if !ok {
			return "", false
		}
		// A defined type keeps its own type through a conversion; a
		// reference type's zero is nil and needs none, since nil is
		// already assignable to it.
		if inner == litNil {
			return litNil, true
		}
		if !spellableAsString(t) {
			return "", false
		}
		return QName(t) + "(" + inner + ")", true
	case *node.Struct:
		if !spellableAsString(t) {
			return "", false
		}
		return QName(t) + "{}", true
	case *node.Interface:
		return litNil, true
	default:
		return "", false
	}
}

// ParseTag reads a struct tag into its key-value entries.
//
// The raw tag arrives as the source wrote it, backticks included,
// so the quoting is trimmed first — a caller that forgets sees the
// first key start with a backtick and every lookup miss. Returns an
// empty map rather than nil for an unparseable tag, so a caller
// ranges over the result without a guard.
func ParseTag(raw string) map[string]string {
	tag := reflect.StructTag(strings.Trim(raw, "`"))
	out := map[string]string{}
	// reflect.StructTag exposes no iteration, so the keys are read off
	// the raw text and looked up through it — the lookup is what
	// applies Go's own unquoting rules to each value.
	for _, key := range tagKeys(string(tag)) {
		if v, ok := tag.Lookup(key); ok {
			out[key] = v
		}
	}
	return out
}

// TagValue returns one struct-tag entry's value, and whether the
// tag declared it.
//
// The second result separates an absent key from one declared
// empty: `json:""` is a field the author deliberately left
// unnamed, which is not the same as a field carrying no json tag.
func TagValue(raw, key string) (string, bool) {
	return reflect.StructTag(strings.Trim(raw, "`")).Lookup(key)
}

// tagKeys returns the keys a struct tag declares, in source order.
//
// A tag is a space-separated run of `key:"value"` pairs; the keys
// are everything before each colon. Malformed input yields whatever
// prefix parsed, which the caller's Lookup then filters — a partial
// answer beats rejecting a tag the Go toolchain itself tolerates.
func tagKeys(tag string) []string {
	var out []string
	for tag != "" {
		tag = strings.TrimLeft(tag, " ")
		key, rest, found := strings.Cut(tag, ":")
		if !found || key == "" {
			return out
		}
		value, remainder, closed := cutQuoted(rest)
		if !closed {
			return out
		}
		_ = value
		out = append(out, key)
		tag = remainder
	}
	return out
}

// cutQuoted consumes a leading quoted string and returns it with
// the remainder, reporting whether the quoting closed.
func cutQuoted(s string) (value, rest string, closed bool) {
	if !strings.HasPrefix(s, `"`) {
		return "", "", false
	}
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return s[1:i], s[i+1:], true
		}
	}
	return "", "", false
}

// Quote renders s as a Go interpreted string literal.
//
// Go's own escaping, so a value holding a quote produces a literal
// that parses rather than one truncating at the first `"`.
func Quote(s string) string { return strconv.Quote(s) }

// RawQuote renders s as a Go raw string literal, and whether one is
// possible.
//
// A raw literal is delimited by backticks and admits no escapes, so
// a value containing one cannot be written this way at all — the
// second result is false and the caller falls back to [Quote].
// Worth having for generated regular expressions and templates,
// where interpreted quoting doubles every backslash.
func RawQuote(s string) (string, bool) {
	if strings.ContainsAny(s, "`\r") {
		return "", false
	}
	return "`" + s + "`", true
}
