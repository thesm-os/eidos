// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"errors"
	"fmt"
	"strings"

	"go.thesmos.sh/eidos/core/contract"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/typescript"
)

// ErrUnsupportedRef is returned when a ref carries a shape this
// backend cannot spell as a TypeScript type.
var ErrUnsupportedRef = errors.New("backend: cannot render type reference")

// maxRefDepth bounds the type walk. A type expression nests, and the
// model admits a cycle no source could produce; sixteen is past
// anything written by hand and shallow enough that a malformed graph
// fails one declaration rather than the process.
const maxRefDepth = 16

// renderType spells a [emit.Ref] as a TypeScript type expression,
// registering any import the reference needs on the way.
func (s *renderState) renderType(ref emit.Ref) (string, error) {
	return s.typeAt(ref, maxRefDepth)
}

// typeAt is the recursive worker, with the depth budget threaded
// through.
func (s *renderState) typeAt(ref emit.Ref, depth int) (string, error) {
	if ref == nil {
		// A declaration with no declared type is legal TypeScript —
		// the compiler infers it — so an absent ref renders as an
		// absent annotation rather than as an error.
		return "", nil
	}
	if depth <= 0 {
		return "", fmt.Errorf("%w: nested past %d levels", ErrUnsupportedRef, maxRefDepth)
	}

	switch r := ref.(type) {
	case *emit.BuiltinRef:
		return builtinName(r.Name), nil
	case *emit.TypeRef:
		return s.internalRef(r, depth)
	case *emit.ExternalRef:
		return s.externalRef(r, depth)
	case *emit.CompositeRef:
		return s.compositeRef(r, depth)
	default:
		return "", fmt.Errorf("%w: %T", ErrUnsupportedRef, ref)
	}
}

// builtinName maps a language-neutral builtin onto its TypeScript
// spelling.
//
// A generator emitting for several languages names a builtin in
// whichever vocabulary it thinks in, and the ones it reaches for are
// mostly Go's. Translating here rather than demanding the generator
// pre-translate is what lets a language-neutral generator target
// TypeScript at all; a name with no mapping passes through, so a
// generator that already said `string` gets `string`.
func builtinName(name string) string {
	if got, ok := builtinNames[name]; ok {
		return got
	}
	return name
}

// tsNumber is TypeScript's single numeric type. Every fixed-width
// integer and float a source language distinguishes collapses onto
// it, which is why it appears once per width below.
const tsNumber = "number"

// builtinNames is the mapping [builtinName] applies.
//
// `int64` and `uint64` map to `number`, not `bigint`. bigint is the
// only type holding their full range, but JSON.parse yields number,
// so a generated bigint is wrong for any value crossing JSON without
// a reviver — which is the boundary generated types nearly always sit
// on. The loss is silent above 2^53 and the alternative is wrong far
// more often.
var builtinNames = map[string]string{ //nolint:gochecknoglobals // immutable lookup table
	"bool":    "boolean",
	"byte":    tsNumber,
	"rune":    "string",
	"int":     tsNumber,
	"int8":    tsNumber,
	"int16":   tsNumber,
	"int32":   tsNumber,
	"int64":   tsNumber,
	"uint":    tsNumber,
	"uint8":   tsNumber,
	"uint16":  tsNumber,
	"uint32":  tsNumber,
	"uint64":  tsNumber,
	"float32": tsNumber,
	"float64": tsNumber,
	"error":   "Error",
	"any":     "unknown",
}

// internalRef spells a reference to another entity in this run.
//
// Same-module references render bare. A cross-module one registers a
// named import, because TypeScript has no qualifier syntax for
// reaching into another module without binding a name first.
func (s *renderState) internalRef(r *emit.TypeRef, depth int) (string, error) {
	name := targetName(r.Target)
	if name == "" {
		return "", fmt.Errorf("%w: internal ref names no target", ErrUnsupportedRef)
	}
	local := s.imports.Named(targetModule(r.Target), name, true)
	if local == "" {
		local = name
	}
	return s.withArgs(local, r.TypeArgs, depth)
}

// externalRef spells a reference to a type from another module.
func (s *renderState) externalRef(r *emit.ExternalRef, depth int) (string, error) {
	if r.Name == "" {
		return "", fmt.Errorf("%w: external ref names no type", ErrUnsupportedRef)
	}
	local := s.imports.Named(r.Package, r.Name, true)
	if local == "" {
		local = r.Name
	}
	return s.withArgs(local, r.TypeArgs, depth)
}

// withArgs appends a generic argument list to a rendered name.
func (s *renderState) withArgs(name string, args []emit.Ref, depth int) (string, error) {
	if len(args) == 0 {
		return name, nil
	}
	parts := make([]string, 0, len(args))
	for _, a := range args {
		got, err := s.typeAt(a, depth-1)
		if err != nil {
			return "", err
		}
		parts = append(parts, got)
	}
	return name + "<" + strings.Join(parts, ", ") + ">", nil
}

// compositeRef spells a compound shape.
func (s *renderState) compositeRef(r *emit.CompositeRef, depth int) (string, error) {
	switch r.Shape {
	case emit.ShapePointer:
		return s.pointerRef(r, depth)
	case emit.ShapeSlice, emit.ShapeArray:
		return s.sequenceRef(r, depth)
	case emit.ShapeMap:
		return s.mapRef(r, depth)
	case emit.ShapeFunc:
		return s.funcRef(r, depth)
	case emit.ShapeUnion:
		return s.unionRef(r, depth)
	default:
		return "", fmt.Errorf("%w: composite shape %v", ErrUnsupportedRef, r.Shape)
	}
}

// pointerRef spells a pointer as a nullable type.
//
// TypeScript has no pointer. What a pointer carries that a value does
// not is the ability to be absent, and `T | null` is how TypeScript
// spells that. Rendering the element alone would drop the distinction
// a generator drew when it chose a pointer.
func (s *renderState) pointerRef(r *emit.CompositeRef, depth int) (string, error) {
	elem, err := s.typeAt(r.Elem, depth-1)
	if err != nil {
		return "", err
	}
	if elem == "" {
		return "", fmt.Errorf("%w: pointer names no element", ErrUnsupportedRef)
	}
	return elem + " | null", nil
}

// sequenceRef spells a slice or array.
//
// Both become `T[]`, and a fixed length is dropped: TypeScript's only
// fixed-length sequence is a tuple, and a tuple of N identical
// elements is not what an array of N means to the languages that
// distinguish them. The element type is what survives translation.
//
// A union element is parenthesised, because `A | B[]` binds as
// `A | (B[])` and would silently mean an array of B beside a plain A.
func (s *renderState) sequenceRef(r *emit.CompositeRef, depth int) (string, error) {
	elem, err := s.typeAt(r.Elem, depth-1)
	if err != nil {
		return "", err
	}
	if elem == "" {
		return "", fmt.Errorf("%w: sequence names no element", ErrUnsupportedRef)
	}
	if needsParens(elem) {
		return "(" + elem + ")[]", nil
	}
	return elem + "[]", nil
}

// mapRef spells an associative container as a Record.
//
// `Record<K, V>` rather than `Map<K, V>`: the two are different types
// in TypeScript with different APIs and different JSON behaviour, and
// a language-neutral map is a plain keyed object at the boundary
// generated types sit on.
func (s *renderState) mapRef(r *emit.CompositeRef, depth int) (string, error) {
	key, err := s.typeAt(r.MapKey, depth-1)
	if err != nil {
		return "", err
	}
	val, err := s.typeAt(r.MapValue, depth-1)
	if err != nil {
		return "", err
	}
	if key == "" || val == "" {
		return "", fmt.Errorf("%w: map names no key or value", ErrUnsupportedRef)
	}
	return "Record<" + key + ", " + val + ">", nil
}

// funcRef spells a function type.
//
// Parameters are named positionally. TypeScript requires a name in a
// function type — `(string) => void` declares a parameter *named*
// string with an inferred type, which is a different signature from
// the one meant — so a ref that carries only types has names invented
// for it.
func (s *renderState) funcRef(r *emit.CompositeRef, depth int) (string, error) {
	params := make([]string, 0, len(r.FuncParams))
	for i, p := range r.FuncParams {
		got, err := s.typeAt(p, depth-1)
		if err != nil {
			return "", err
		}
		params = append(params, fmt.Sprintf("arg%d: %s", i, got))
	}

	ret, err := s.funcReturn(r.FuncReturns, depth)
	if err != nil {
		return "", err
	}
	return "(" + strings.Join(params, ", ") + ") => " + ret, nil
}

// funcReturn spells a function type's return.
//
// Several returns become a tuple. TypeScript has one return value, so
// a signature carrying more than one — which Go's does routinely — is
// spelled as the tuple that holds them.
func (s *renderState) funcReturn(returns []emit.Ref, depth int) (string, error) {
	switch len(returns) {
	case 0:
		return typescript.TypeVoid, nil
	case 1:
		got, err := s.typeAt(returns[0], depth-1)
		if err != nil {
			return "", err
		}
		if got == "" {
			return typescript.TypeVoid, nil
		}
		return got, nil
	default:
		parts := make([]string, 0, len(returns))
		for _, rr := range returns {
			got, err := s.typeAt(rr, depth-1)
			if err != nil {
				return "", err
			}
			parts = append(parts, got)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	}
}

// unionRef spells a union.
//
// The approximate marker Go's type sets carry has no TypeScript
// counterpart — `~int` means "any type whose underlying type is int",
// which TypeScript's structural typing has no way to say — so the
// term renders as the type it approximates.
func (s *renderState) unionRef(r *emit.CompositeRef, depth int) (string, error) {
	if len(r.UnionTerms) == 0 {
		return typescript.TypeNever, nil
	}
	parts := make([]string, 0, len(r.UnionTerms))
	for _, term := range r.UnionTerms {
		got, err := s.typeAt(term.Type, depth-1)
		if err != nil {
			return "", err
		}
		if got != "" {
			parts = append(parts, got)
		}
	}
	if len(parts) == 0 {
		return typescript.TypeNever, nil
	}
	return strings.Join(parts, " | "), nil
}

// needsParens reports whether a rendered type must be parenthesised
// before a postfix operator binds to it.
func needsParens(rendered string) bool {
	return strings.Contains(rendered, " | ") || strings.Contains(rendered, " => ")
}

// targetName returns the declared name of an internal ref's target.
func targetName(target emit.Node) string {
	if owner, ok := target.(contract.Owner); ok {
		return owner.OwnerName()
	}
	return ""
}

// targetModule returns the module specifier an internal ref's target
// is declared in, or empty when the run does not know one.
//
// Type-switched rather than read through an interface, because the
// emit kinds carry Target as a field and the model declares no
// accessor for it. An unknown kind reports no module, which renders
// the reference bare — right for a target in the same file, and
// visible as a missing import for one that is not.
func targetModule(target emit.Node) string {
	switch t := target.(type) {
	case *emit.Struct:
		return t.Target.ImportPath
	case *emit.Interface:
		return t.Target.ImportPath
	case *emit.Enum:
		return t.Target.ImportPath
	case *emit.Alias:
		// An alias names the file it lands in `File`, not `Target` —
		// its Target field is the type it aliases.
		return t.File.ImportPath
	case *emit.Function:
		return t.Target.ImportPath
	case *emit.Variable:
		return t.Target.ImportPath
	case *emit.Constant:
		return t.Target.ImportPath
	default:
		return ""
	}
}
