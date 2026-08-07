// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// Go type expressions as text, in both directions.
//
// # Why this is not renderType
//
// The backend's `renderType` is bound to a render state because it
// registers the file's imports and elides same-package qualifiers.
// Nothing here can do either, and nothing here should: these
// produce text for a human — a diagnostic naming the type that
// failed, a doc comment describing what a generated method takes, a
// `%s` in a message. Text cannot ask for an import, so a caller
// that puts the result into generated source emits a file naming a
// package it never imports.
//
// The rule is worth stating because the mistake is easy and the
// failure is late: the generated file parses, and the consumer's
// compiler reports an undefined identifier against code they did
// not write.

// ErrBadTypeExpr is returned by [ParseTypeRef] for text that is not
// a Go type expression this package can read.
var ErrBadTypeExpr = errors.New("golang: malformed type expression")

// maxTypeDepth bounds both directions of the conversion.
//
// A type expression nests — `map[string][]*[]T` is four levels —
// and the model admits a cycle no source could produce. Sixteen is
// past anything a human writes and shallow enough that a malformed
// graph fails a generation pass rather than the process.
const maxTypeDepth = 16

// TypeString renders a source type reference as the Go expression
// an author would recognise.
//
// Qualified by the package's last segment rather than its full
// path: `store.User`, not `example.com/store.User`. This is for
// reading, and the short form is what appears in the author's own
// file.
//
// Empty for nil and for a shape this package cannot spell, so a
// caller interpolating the result needs no guard — and an empty
// type in a message is visibly wrong, where a partial one would
// read as correct.
func TypeString(t *node.TypeRef) string {
	return typeText(t, maxTypeDepth, false)
}

// TypeStringQualified renders the same expression with full import
// paths — `example.com/store.User`.
//
// For a message that has to be unambiguous across packages, where
// two `store.User` types would otherwise read as one.
func TypeStringQualified(t *node.TypeRef) string {
	return typeText(t, maxTypeDepth, true)
}

// typeText is the recursive renderer, with the qualification
// choice and the depth budget threaded through.
func typeText(t *node.TypeRef, depth int, full bool) string {
	if t == nil || depth <= 0 {
		return ""
	}
	switch {
	case IsChannel(t):
		return chanString(t, depth, full)
	case t.IsPointer():
		return "*" + typeText(t.Elem, depth-1, full)
	case t.IsSlice():
		return "[]" + typeText(t.Elem, depth-1, full)
	case t.IsArray():
		return "[" + strconv.Itoa(t.ArrayLen) + "]" + typeText(t.Elem, depth-1, full)
	case t.IsMap():
		return "map[" + typeText(t.MapKey, depth-1, full) + "]" +
			typeText(t.MapValue, depth-1, full)
	case t.IsFunc():
		return funcString(t, depth, full)
	case t.IsAnonStruct():
		// The field list is deliberately not spelled out: an anonymous
		// struct in a message is identified by being one, and
		// reproducing its fields makes a diagnostic longer than the
		// declaration it is about.
		return "struct{…}"
	case t.IsAnonInterface():
		if len(t.Methods) == 0 && len(t.Embeds) == 0 {
			return typeAny
		}
		return "interface{…}"
	case t.IsTypeParam():
		return t.Name
	default:
		return namedString(t, depth, full)
	}
}

// namedString renders a named reference with its qualifier and any
// type arguments.
func namedString(t *node.TypeRef, depth int, full bool) string {
	name := t.Name
	if t.Package != "" {
		if full {
			name = t.Package + "." + t.Name
		} else {
			name = PackageName(t.Package) + "." + t.Name
		}
	}
	if len(t.TypeArgs) == 0 {
		return name
	}
	args := make([]string, len(t.TypeArgs))
	for i, a := range t.TypeArgs {
		args[i] = typeText(a, depth-1, full)
	}
	return name + "[" + strings.Join(args, ", ") + "]"
}

// funcString renders a function type's parameter and return lists.
//
// Parameters carry no names — the model records only types — and a
// single unnamed result renders bare, which is what Go's own
// grammar prefers.
func funcString(t *node.TypeRef, depth int, full bool) string {
	params := make([]string, len(t.FuncParams))
	for i, p := range t.FuncParams {
		params[i] = typeText(p, depth-1, full)
	}
	out := "func(" + strings.Join(params, ", ") + ")"
	switch len(t.FuncReturns) {
	case 0:
		return out
	case 1:
		return out + " " + typeText(t.FuncReturns[0], depth-1, full)
	default:
		returns := make([]string, len(t.FuncReturns))
		for i, r := range t.FuncReturns {
			returns[i] = typeText(r, depth-1, full)
		}
		return out + " (" + strings.Join(returns, ", ") + ")"
	}
}

// chanString renders a channel with its direction.
//
// The model has no channel variant — a channel arrives as a named
// reference under a synthetic package with the direction stamped
// beside it — so the spelling is reassembled from the stamp rather
// than read off the shape.
func chanString(t *node.TypeRef, depth int, full bool) string {
	elem := ""
	if e := ChanElem(t); e != nil {
		elem = typeText(e, depth-1, full)
	}
	switch ChanDir(t) {
	case ChanSend:
		return "chan<- " + elem
	case ChanRecv:
		return "<-chan " + elem
	default:
		return "chan " + elem
	}
}

// ParseTypeRef reads a Go type expression into the reference a
// generated file can use.
//
//	User                  -> resolved against srcPkg
//	string                -> the builtin, bare
//	*User                 -> a pointer to it
//	[]User                -> a slice
//	map[string]*User      -> a map, both halves parsed
//	example.com/x.User    -> a full import path
//
// The parser exists because a directive value naming a type is text
// — `//gen:store value=map[string]*User` — and every consumer that
// wanted one either restricted itself to bare identifiers or built
// the reference by concatenation, which produces a name with no
// import behind it.
//
// Deliberately partial: function types, channels, generics and
// anonymous composites are refused rather than half-parsed. Each
// needs syntax a directive value has no room for, and a caller
// wanting one declares a named type and refers to that.
func ParseTypeRef(expr, srcPkg string) (emit.Ref, error) {
	return parseTypeRef(strings.TrimSpace(expr), srcPkg, maxTypeDepth)
}

// parseTypeRef is [ParseTypeRef] with the nesting budget threaded
// through.
func parseTypeRef(expr, srcPkg string, depth int) (emit.Ref, error) {
	if depth <= 0 {
		return nil, fmt.Errorf("%w: %q nests too deeply", ErrBadTypeExpr, expr)
	}
	switch {
	case expr == "":
		return nil, fmt.Errorf("%w: empty", ErrBadTypeExpr)

	case strings.HasPrefix(expr, "*"):
		elem, err := parseTypeRef(strings.TrimSpace(expr[1:]), srcPkg, depth-1)
		if err != nil {
			return nil, err
		}
		return emit.Ptr(elem), nil

	case strings.HasPrefix(expr, "[]"):
		elem, err := parseTypeRef(strings.TrimSpace(expr[2:]), srcPkg, depth-1)
		if err != nil {
			return nil, err
		}
		return emit.SliceOf(elem), nil

	case strings.HasPrefix(expr, "map["):
		return parseMap(expr, srcPkg, depth)

	case strings.HasPrefix(expr, "["):
		// An array's length has to be a constant this package can read;
		// anything else is a reference to a declaration it cannot
		// resolve, and guessing a length emits a type of the wrong size.
		return nil, fmt.Errorf("%w: %q — array types are not parsed", ErrBadTypeExpr, expr)

	case !isPlainTypeName(expr):
		// Everything that reaches here has had its prefixes stripped,
		// so what remains must be a qualified name and nothing else.
		// Checked positively rather than by listing the shapes to
		// reject: `chan int` carries no punctuation at all, and a
		// character blacklist admitted it.
		return nil, fmt.Errorf(
			"%w: %q — declare a named type for a function, channel, "+
				"generic instantiation or anonymous composite and name "+
				"that instead", ErrBadTypeExpr, expr,
		)

	default:
		return RefForQualified(expr, srcPkg)
	}
}

// isPlainTypeName reports whether expr is a bare or qualified type
// name with no further syntax.
//
// An import path admits slashes, dots and hyphens, and an
// identifier admits letters, digits and underscores — so the
// permitted set is their union. A space rules out every keyword-led
// form (`chan T`, `func()`, `struct{…}`) in one condition, and a
// bracket rules out an instantiation.
func isPlainTypeName(expr string) bool {
	if expr == "" {
		return false
	}
	for _, r := range expr {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '/' || r == '_' || r == '-' || r == '~':
		default:
			return false
		}
	}
	// A leading digit cannot open an identifier, and a path never
	// starts with one either.
	return expr[0] < '0' || expr[0] > '9'
}

// parseMap splits `map[K]V` at the bracket that closes the key.
//
// Counted rather than found with an index, because a key may itself
// be a map: `map[map[string]int]bool` closes its outer bracket at
// the third `]`, and taking the first produces a key of `map[string`
// that resolves to nothing.
func parseMap(expr, srcPkg string, depth int) (emit.Ref, error) {
	rest := expr[len("map["):]
	level, end := 1, -1
	for i, r := range rest {
		switch r {
		case '[':
			level++
		case ']':
			level--
			if level == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("%w: %q has no closing bracket", ErrBadTypeExpr, expr)
	}
	key, err := parseTypeRef(strings.TrimSpace(rest[:end]), srcPkg, depth-1)
	if err != nil {
		return nil, err
	}
	value, err := parseTypeRef(strings.TrimSpace(rest[end+1:]), srcPkg, depth-1)
	if err != nil {
		return nil, err
	}
	return emit.MapOf(key, value), nil
}
