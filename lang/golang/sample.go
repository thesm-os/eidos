// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// Values that need a type beside them.
//
// [SampleValues] and [ZeroLiteral] answer for builtins, where the
// literal is the whole answer. Everything else needs the type spelled
// too — `Weekday(42)`, `Point{X: 42}` — and a spelling is not a
// string: the file it lands in may not be the file the type was
// declared in, so it has to be a reference the backend can resolve and
// register an import for.
//
// The string-returning forms could not do that. They composed the
// spelling with [QName], which is the *import path* joined to the
// name, so a defined type came out as `example.com/cfg.Weekday(42)` —
// not Go, and no import registered. Nothing in the tree called them,
// which is why it survived; the one consumer that tried wrote its own
// and left a note saying why.

// Sample is a value a generated check writes, together with the type
// it is written against.
//
// Ref is nil when the text stands alone, which is the builtin case and
// the only one the string-returning forms can serve.
type Sample struct {
	// Ref qualifies the type the text is written against. Rendered
	// through the backend's `renderType`, which resolves the spelling
	// for the file and registers the import it needs.
	Ref emit.Ref

	// Text is the literal. Empty when no sample could be derived,
	// which is the caller's signal to omit the check rather than write
	// one that cannot fail.
	Text string

	// Composite selects the syntax: `Ref{Text}` when true, `Ref(Text)`
	// when false. A conversion and a composite literal are not
	// interchangeable, and which one applies is a property of the type
	// rather than of the value.
	Composite bool
}

// OK reports whether a value was derived.
//
// The check a caller makes before emitting: a sample that derived
// nothing must produce no assertion, because an assertion against an
// undefined value passes whatever the subject does.
func (s Sample) OK() bool { return s.Text != "" }

// literalSample returns a sample needing no type beside it.
func literalSample(text string) Sample { return Sample{Text: text} }

// SampleRefFor returns two distinct sample values for a source type,
// resolving named types through r.
//
// The answer [SampleFor] cannot give. Two rather than one for the
// reason given in this package's values documentation: a check
// comparing against a single value passes whenever the subject already
// held it.
//
// Handles builtins, `any`, defined types, arrays and structs. A type
// the resolver cannot reach, or one admitting no distinguishable
// values, yields two zero Samples — ask [Sample.OK] rather than
// comparing against the zero value.
//
// # Allocation
//
// One Sample pair per call, plus one [Ref] per level of named type
// unwrapped. The walk is bounded by the same depth limit as
// [ZeroLiteralFor], so a self-referential type terminates.
func SampleRefFor(t *node.TypeRef, fieldName string, r Resolver) (sample, alternate Sample) {
	return sampleRefFor(t, fieldName, r, maxResolveDepth)
}

// sampleRefFor is [SampleRefFor] with the recursion budget threaded
// through.
func sampleRefFor(
	t *node.TypeRef, fieldName string, r Resolver, depth int,
) (sample, alternate Sample) {
	if t == nil || depth <= 0 {
		return Sample{}, Sample{}
	}
	if IsAny(t) {
		// `any` admits every value, so the string pair serves and needs
		// no type beside it.
		s, a := SampleValues(typeString, fieldName)
		return literalSample(s), literalSample(a)
	}
	if t.IsBuiltin() {
		s, a := SampleValues(t.Name, fieldName)
		if s == "" {
			return Sample{}, Sample{}
		}
		return literalSample(s), literalSample(a)
	}
	if t.IsArray() {
		return arraySample(t, fieldName, r, depth)
	}
	if t.TypeKind != node.TypeRefNamed || r == nil {
		return Sample{}, Sample{}
	}
	target, found := r.Resolve(t)
	if !found {
		return Sample{}, Sample{}
	}
	switch decl := target.(type) {
	case *node.Alias:
		return definedSample(t, decl, fieldName, r, depth)
	case *node.Struct:
		return structSample(t, decl, fieldName, r, depth)
	default:
		return Sample{}, Sample{}
	}
}

// definedSample renders a defined type's sample as a conversion of its
// underlying one — `Weekday(42)`, which keeps its own type where a
// bare 42 compiles today and stops compiling the moment the field
// moves.
func definedSample(
	t *node.TypeRef, decl *node.Alias, fieldName string, r Resolver, depth int,
) (sample, alternate Sample) {
	if decl.Target == nil {
		return Sample{}, Sample{}
	}
	inner, innerAlt := sampleRefFor(decl.Target, fieldName, r, depth-1)
	if !inner.OK() {
		return Sample{}, Sample{}
	}
	ref := FromNode(t)
	return Sample{Ref: ref, Text: inner.Text}, Sample{Ref: ref, Text: innerAlt.Text}
}

// structSample renders a struct's sample as a composite literal
// setting its first field that has one.
//
// One field rather than all of them: the sample exists to be
// distinguishable, and setting a single field achieves that while
// staying readable in the generated source. The first *settable* one,
// because a struct may lead with a field of a type the resolver cannot
// reach and refusing on that would lose samples a later field supplies.
func structSample(
	t *node.TypeRef, decl *node.Struct, fieldName string, r Resolver, depth int,
) (sample, alternate Sample) {
	ref := FromNode(t)
	for _, f := range decl.Fields {
		if f == nil || !IsExported(f.Name) {
			continue
		}
		inner, innerAlt := sampleRefFor(f.Type, fieldName, r, depth-1)
		if !inner.OK() {
			continue
		}
		return Sample{Ref: ref, Text: "{" + f.Name + ": " + inner.Text + "}", Composite: true},
			Sample{Ref: ref, Text: "{" + f.Name + ": " + innerAlt.Text + "}", Composite: true}
	}
	return Sample{}, Sample{}
}

// ZeroRefFor returns a type's zero as a [Sample], resolving named
// types through r.
//
// The ref-carrying counterpart to [ZeroLiteralFor], which can only
// answer for types whose spelling needs no import. A generated check
// comparing a cross-package field against its zero needs this one.
//
// Reports false when no zero could be derived, which is the same
// signal [ZeroLiteralFor] gives and means the same thing: emit
// nothing rather than a comparison against an undefined value.
func ZeroRefFor(t *node.TypeRef, r Resolver) (Sample, bool) {
	// The literal forms answer first, because a type spellable without
	// an import wants no ref beside it and a Sample carrying one would
	// render `pkg.T` where the file says `T`.
	if lit, ok := ZeroLiteralFor(t, r); ok {
		return literalSample(lit), true
	}
	if t == nil || r == nil || t.TypeKind != node.TypeRefNamed {
		return Sample{}, false
	}
	target, found := r.Resolve(t)
	if !found {
		return Sample{}, false
	}
	switch decl := target.(type) {
	case *node.Alias:
		if decl.Target == nil {
			return Sample{}, false
		}
		inner, ok := ZeroRefFor(decl.Target, r)
		if !ok || inner.Ref != nil {
			return Sample{}, false
		}
		return Sample{Ref: FromNode(t), Text: inner.Text}, true
	case *node.Struct:
		return Sample{Ref: FromNode(t), Text: "{}", Composite: true}, true
	default:
		return Sample{}, false
	}
}

// arraySample renders an array's sample as a composite literal holding
// one element, which is enough to distinguish it from the zero array.
func arraySample(
	t *node.TypeRef, fieldName string, r Resolver, depth int,
) (sample, alternate Sample) {
	elem, _ := ArrayElem(t)
	inner, innerAlt := sampleRefFor(elem, fieldName, r, depth-1)
	if !inner.OK() {
		return Sample{}, Sample{}
	}
	ref := FromNode(t)
	return Sample{Ref: ref, Text: "{" + inner.Text + "}", Composite: true},
		Sample{Ref: ref, Text: "{" + innerAlt.Text + "}", Composite: true}
}
