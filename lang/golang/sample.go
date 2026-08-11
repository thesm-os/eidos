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

	// Refusal says why Text is empty. [RefusedNone] when a value was
	// derived, so the zero Sample reads as "nothing was attempted"
	// rather than as a refusal with a reason.
	//
	// Meaningful only when [Sample.OK] is false. A caller emitting is
	// not interested; a caller explaining an assertion it declined to
	// write is, and could not previously tell an incomplete run from a
	// type that genuinely has no literal.
	Refusal SampleRefusal
}

// SampleRefusal says why a sample could not be derived.
//
// Twelve refusal sites in this package answered with the same empty
// [Sample], and only [RefusedNoLiteral] is a fact about the type. The
// rest describe an input the caller could fix — which matters because
// the response to all of them is to omit the check, so a run missing a
// package silently produces a test that asserts less than it appears
// to.
type SampleRefusal uint8

const (
	// RefusedNone is the zero value: no refusal, a value was derived.
	RefusedNone SampleRefusal = iota

	// RefusedNoResolver is a nil [Resolver] where a named type needed
	// one, or a nil type reference. The caller's own input.
	RefusedNoResolver

	// RefusedDepth is a walk that hit the recursion budget, which a
	// self-referential type reaches before it terminates.
	RefusedDepth

	// RefusedUnresolved is a named type the resolver could not reach —
	// ordinarily a package the run's patterns did not load.
	RefusedUnresolved

	// RefusedNoLiteral is a type that admits no distinguishable
	// literal: a builtin outside the value table, a struct with no
	// exported settable field, or a declaration with no sample form.
	// The only refusal that is settled rather than fixable.
	RefusedNoLiteral
)

// Incomplete reports whether the refusal describes the input rather
// than the type.
//
// The distinction that earns the enum: a caller warning "no check was
// written for this field" wants to say so only when the answer might
// have been different under a wider run. [RefusedNoLiteral] never
// would be.
func (r SampleRefusal) Incomplete() bool {
	return r == RefusedNoResolver || r == RefusedDepth || r == RefusedUnresolved
}

// OK reports whether a value was derived.
//
// The check a caller makes before emitting: a sample that derived
// nothing must produce no assertion, because an assertion against an
// undefined value passes whatever the subject does.
func (s Sample) OK() bool { return s.Text != "" }

// literalSample returns a sample needing no type beside it.
func literalSample(text string) Sample { return Sample{Text: text} }

// refused returns the empty sample pair carrying why.
func refused(why SampleRefusal) (sample, alternate Sample) {
	return Sample{Refusal: why}, Sample{Refusal: why}
}

// SampleRefFor returns two distinct sample values for a source type,
// resolving named types through r.
//
// The answer [SampleFor] cannot give. Two rather than one for the
// reason given in this package's values documentation: a check
// comparing against a single value passes whenever the subject already
// held it.
//
// Handles builtins, `any`, defined types, arrays, slices, maps and
// structs. A type the resolver cannot reach, or one admitting no
// distinguishable values, yields two zero Samples — ask [Sample.OK]
// rather than comparing against the zero value.
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
	if t == nil {
		return refused(RefusedNoResolver)
	}
	if depth <= 0 {
		return refused(RefusedDepth)
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
			return refused(RefusedNoLiteral)
		}
		return literalSample(s), literalSample(a)
	}
	if t.IsArray() {
		return arraySample(t, fieldName, r, depth)
	}
	if t.IsSlice() {
		return sliceSample(t, fieldName, r, depth)
	}
	if t.IsMap() {
		return mapSample(t, fieldName, r, depth)
	}
	if t.TypeKind != node.TypeRefNamed {
		// A kind with no arm above — a func or channel type — has no
		// literal to write, whatever the resolver holds.
		return refused(RefusedNoLiteral)
	}
	if r == nil {
		return refused(RefusedNoResolver)
	}
	target, found := r.Resolve(t)
	if !found {
		return refused(RefusedUnresolved)
	}
	switch decl := target.(type) {
	case *node.Alias:
		return definedSample(t, decl, fieldName, r, depth)
	case *node.Struct:
		return structSample(t, decl, fieldName, r, depth)
	default:
		return refused(RefusedNoLiteral)
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
		return refused(RefusedUnresolved)
	}
	inner, innerAlt := sampleRefFor(decl.Target, fieldName, r, depth-1)
	if !inner.OK() {
		// The underlying type's reason, not this one's: an alias over an
		// unloaded type is unresolved, and saying "no literal" would
		// report a fixable run as a settled fact.
		return refused(inner.Refusal)
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
	// The first fixable reason a field gave, kept so a struct whose only
	// candidates were unloaded reports that rather than claiming it has
	// no literal — the answer would differ under a wider run.
	incomplete := RefusedNone
	for _, f := range decl.Fields {
		if f == nil || !IsExported(f.Name) {
			continue
		}
		inner, innerAlt := sampleRefFor(f.Type, fieldName, r, depth-1)
		if !inner.OK() {
			if incomplete == RefusedNone && inner.Refusal.Incomplete() {
				incomplete = inner.Refusal
			}
			continue
		}
		return Sample{Ref: ref, Text: "{" + f.Name + ": " + inner.Text + "}", Composite: true},
			Sample{Ref: ref, Text: "{" + f.Name + ": " + innerAlt.Text + "}", Composite: true}
	}
	if incomplete != RefusedNone {
		return refused(incomplete)
	}
	return refused(RefusedNoLiteral)
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
// nothing rather than a comparison against an undefined value. The
// returned Sample carries [Sample.Refusal] either way, so a caller
// that declined can say whether a wider run would have answered.
func ZeroRefFor(t *node.TypeRef, r Resolver) (Sample, bool) {
	// The literal forms answer first, because a type spellable without
	// an import wants no ref beside it and a Sample carrying one would
	// render `pkg.T` where the file says `T`.
	if lit, ok := ZeroLiteralFor(t, r); ok {
		return literalSample(lit), true
	}
	if t == nil || r == nil {
		return Sample{Refusal: RefusedNoResolver}, false
	}
	if t.TypeKind != node.TypeRefNamed {
		return Sample{Refusal: RefusedNoLiteral}, false
	}
	target, found := r.Resolve(t)
	if !found {
		return Sample{Refusal: RefusedUnresolved}, false
	}
	switch decl := target.(type) {
	case *node.Alias:
		if decl.Target == nil {
			return Sample{Refusal: RefusedUnresolved}, false
		}
		inner, ok := ZeroRefFor(decl.Target, r)
		if !ok {
			return Sample{Refusal: inner.Refusal}, false
		}
		if inner.Ref != nil {
			// The inner zero already carries a ref, so wrapping it would
			// spell one type and import another. Settled, not fixable.
			return Sample{Refusal: RefusedNoLiteral}, false
		}
		return Sample{Ref: FromNode(t), Text: inner.Text}, true
	case *node.Struct:
		return Sample{Ref: FromNode(t), Text: "{}", Composite: true}, true
	default:
		return Sample{Refusal: RefusedNoLiteral}, false
	}
}

// sliceSample derives a one-element literal per half, so the pair
// differs in the element rather than in the length.
//
// A length-only difference — `{x}` against `{x, x}` — is invisible to
// a subject that reads the contents and only shows up in one that
// counts them, which is the rarer shape. Differing in the element
// distinguishes both.
//
// An element deriving nothing yields nothing for the slice. `[]T{}`
// and `[]T{x}` are different claims and only the second is a sample:
// a check built from an empty literal passes against an
// implementation that reads no element at all.
func sliceSample(
	t *node.TypeRef, fieldName string, r Resolver, depth int,
) (sample, alternate Sample) {
	inner, innerAlt := sampleRefFor(SliceElem(t), fieldName, r, depth-1)
	if !inner.OK() {
		return refused(inner.Refusal)
	}
	ref := FromNode(t)
	return Sample{Ref: ref, Text: "{" + inner.Text + "}", Composite: true},
		Sample{Ref: ref, Text: "{" + innerAlt.Text + "}", Composite: true}
}

// mapSample derives a one-entry literal per half, differing in the
// key.
//
// The key rather than the value, because `map[K]struct{}` is how Go
// spells a set: a pair differing only in the value produces two
// identical literals for it, and a check comparing them cannot fail.
// Keying the difference works for both shapes.
//
// Key and value must both derive, for the reason the element case
// gives: a literal missing either half is a different claim, not a
// smaller one. That the pair's two keys differ follows from the pair
// [sampleRefFor] returns — every arm answers with two distinct texts
// or with nothing — so it is not rechecked here.
func mapSample(
	t *node.TypeRef, fieldName string, r Resolver, depth int,
) (sample, alternate Sample) {
	keySample, keyAlt := sampleRefFor(MapKey(t), fieldName, r, depth-1)
	if !keySample.OK() {
		return refused(keySample.Refusal)
	}
	valSample, _ := sampleRefFor(MapValue(t), fieldName, r, depth-1)
	if !valSample.OK() {
		return refused(valSample.Refusal)
	}
	ref := FromNode(t)
	return Sample{Ref: ref, Text: "{" + keySample.Text + ": " + valSample.Text + "}", Composite: true},
		Sample{Ref: ref, Text: "{" + keyAlt.Text + ": " + valSample.Text + "}", Composite: true}
}

// arraySample renders an array's sample as a composite literal holding
// one element, which is enough to distinguish it from the zero array.
func arraySample(
	t *node.TypeRef, fieldName string, r Resolver, depth int,
) (sample, alternate Sample) {
	elem, _ := ArrayElem(t)
	inner, innerAlt := sampleRefFor(elem, fieldName, r, depth-1)
	if !inner.OK() {
		return refused(inner.Refusal)
	}
	ref := FromNode(t)
	return Sample{Ref: ref, Text: "{" + inner.Text + "}", Composite: true},
		Sample{Ref: ref, Text: "{" + innerAlt.Text + "}", Composite: true}
}
