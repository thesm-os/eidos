// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"strconv"
	"strings"

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
// An alias of [emit.Sample]: every field is an emit value, so the
// declaration belongs beside them and a generator carrying samples
// through a language-neutral core names one type rather than
// converting between two spellings of it.
type Sample = emit.Sample

// SampleRefusal says why a sample could not be derived, aliased from
// [emit.SampleRefusal] for the reason [Sample] is.
type SampleRefusal = emit.SampleRefusal

// The refusal reasons, re-exported so a caller reading one need not
// import the emit package to name it.
const (
	RefusedNone       = emit.RefusedNone
	RefusedNoResolver = emit.RefusedNoResolver
	RefusedDepth      = emit.RefusedDepth
	RefusedUnresolved = emit.RefusedUnresolved
	RefusedNoLiteral  = emit.RefusedNoLiteral
)

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
	if t.IsFunc() {
		return funcSample(t)
	}
	if t.TypeKind != node.TypeRefNamed {
		// A kind with no arm above — a type parameter or an anonymous
		// struct or interface — has no value to write, whatever the
		// resolver holds.
		return refused(RefusedNoLiteral)
	}
	if IsChannel(t) {
		// A channel arrives as a named ref with the frontend's own
		// stamp on it, never as a kind of its own, so it is caught
		// here rather than beside the func arm — left to fall
		// through, the resolver would refuse `go.chan` as it refuses
		// any type the run never loaded.
		return chanSample(t)
	}
	if s, a, ok := stdlibSample(t); ok {
		return s, a
	}
	if r == nil {
		return refused(RefusedNoResolver)
	}
	target, found := r.Resolve(t)
	if !found {
		return refused(RefusedUnresolved)
	}
	s, a := declaredSample(t, target, fieldName, r, depth)
	return authoredOver(target, s, a)
}

// declaredSample derives a value pair from what the declaration is.
//
// An interface reaches the default arm and is refused, which is the
// honest answer: it has no literal form, so nothing derivable can be
// written for it. That is the case [authoredOver] exists to answer.
func declaredSample(
	t *node.TypeRef, target node.Node, fieldName string, r Resolver, depth int,
) (sample, alternate Sample) {
	switch decl := target.(type) {
	case *node.Alias:
		return definedSample(t, decl, fieldName, r, depth)
	case *node.Struct:
		return structSample(t, decl, fieldName, r, depth)
	default:
		return refused(RefusedNoLiteral)
	}
}

// authoredOver replaces each derived value with the one an author
// named for the declaration, where they named one.
//
// Read here rather than by each consumer of [SampleRefFor]. What a
// value of a type looks like is a question the language answers, and a
// stamp every asker has to remember to prefer is one that two of them
// will forget — leaving a check written against a derivation the
// author declared was wrong.
//
// Each half is replaced independently, because a type may need one and
// not the other: a derived first value is often fine where the second
// has to differ from it in a way the derivation cannot know.
func authoredOver(target node.Node, sample, alternate Sample) (Sample, Sample) {
	bag := target.Meta()
	if authored, ok := emit.AuthoredSampleOf(bag); ok {
		sample = authored
	}
	if authored, ok := emit.AuthoredAlternateOf(bag); ok {
		alternate = authored
	}
	return sample, alternate
}

// stdlibSamples holds the sample builder for named standard-library
// types, keyed by import path and name.
//
// The resolver answers declarations the run loaded, and the standard
// library is never among them, so a named stdlib type refused here
// even when a value is plainly writable. Sampling by underlying
// kind cannot close that: the underlying kind lives in the
// declaration the resolver cannot reach. A curated entry is the only
// thing that can answer, so each one is a value a person asserted
// reads sensibly in a generated fixture — extended when a corpus
// asks, not swept from a list of known kinds.
//
// pkgTime is the import path the curated entries live under.
//
//nolint:gochecknoglobals // package-curated sample table
const pkgTime = "time"

//nolint:gochecknoglobals // package-curated sample table
var stdlibSamples = map[[2]string]func(t *node.TypeRef) (sample, alternate Sample){
	{pkgTime, "Duration"}: durationSample,
	{pkgTime, "Time"}:     timeSample,
}

// stdlibSample answers for a named type in [stdlibSamples].
//
// Consulted before the resolver gate rather than as a fallback, so a
// nil-resolver caller gets the value too; nothing can shadow a
// standard-library import path, so the order is unobservable beyond
// that.
func stdlibSample(t *node.TypeRef) (sample, alternate Sample, ok bool) {
	build, found := stdlibSamples[[2]string{t.Package, t.Name}]
	if !found {
		return Sample{}, Sample{}, false
	}
	sample, alternate = build(t)
	return sample, alternate, true
}

// durationSample renders conversion-form, exactly as [definedSample]
// renders a loaded defined type: `time.Duration(42)` beside
// `Weekday(42)`. The Ref is what registers the import — text alone
// would land an unqualified name in a file that never imported the
// package.
func durationSample(t *node.TypeRef) (sample, alternate Sample) {
	ref := FromNode(t)
	return Sample{Ref: ref, Text: "42"}, Sample{Ref: ref, Text: "7"}
}

// timeSample answers `time.Unix(42, 0)` beside `time.Unix(7, 0)` —
// distinct, comparable, and deterministic.
//
// A constructor call, which is why the entry waited for
// [Sample.Expr]: a timestamp has no conversion form, its zero is the
// year 1, and the call embeds a symbol reference mid-expression that
// a Ref-and-Text pair cannot carry.
func timeSample(*node.TypeRef) (sample, alternate Sample) {
	return Sample{Expr: timeUnix(42)}, Sample{Expr: timeUnix(7)}
}

// timeUnix builds the `time.Unix(sec, 0)` call. [emit.ExprExternal]
// is what registers the `time` import when the backend renders it.
func timeUnix(sec int64) *emit.Expr {
	return &emit.Expr{
		ExprKind: emit.ExprCall,
		Callee:   &emit.Expr{ExprKind: emit.ExprExternal, Pkg: pkgTime, Name: "Unix"},
		Args:     []*emit.Expr{emit.NewLiteralInt(sec), emit.NewLiteralInt(0)},
	}
}

// sampleStampedBy attributes the meta stamps this package writes onto
// refs it constructs itself.
const sampleStampedBy = "lang/golang.sample"

// funcSample answers a func type with the no-op literal — the one
// value a caller can pass that asserts nothing, which is what a
// generated fixture wants from a parameter it has no opinion about.
//
// Results are answered by declaring a var per result and returning
// them: `var r0 error` compiles for every result type with nothing
// but the type's reference, so no zero literal is derived and no
// refusal can propagate from one. Parameters stay anonymous — a
// literal referencing none of them needs no names.
//
// The frontend records a func type's parameter types only, not
// whether the last is variadic, so a `func(...T)` parameter samples
// as `func([]T)` and the generated file's compile is the loud
// failure — the same place an unloaded handle's member fails.
//
// The alternate is built independently rather than aliased: two
// evaluations are two distinct values, funcs compare only to nil, and
// a consumer mutating one must not reach the other.
func funcSample(t *node.TypeRef) (sample, alternate Sample) {
	build := func() *emit.Expr {
		lit := &emit.Expr{ExprKind: emit.ExprFuncLit}
		for _, param := range t.FuncParams {
			lit.FuncParams = append(lit.FuncParams, &emit.Param{Type: FromNode(param)})
		}
		if len(t.FuncReturns) == 0 {
			return lit
		}
		ret := &emit.Stmt{StmtKind: emit.StmtReturn}
		for i, res := range t.FuncReturns {
			lit.FuncReturns = append(lit.FuncReturns, FromNode(res))
			name := "r" + strconv.Itoa(i)
			lit.FuncBody = append(lit.FuncBody, &emit.Stmt{
				StmtKind: emit.StmtVar, DeclName: name, DeclType: FromNode(res),
			})
			ret.Returns = append(ret.Returns, &emit.Expr{ExprKind: emit.ExprIdent, Name: name})
		}
		lit.FuncBody = append(lit.FuncBody, ret)
		return lit
	}
	return Sample{Expr: build()}, Sample{Expr: build()}
}

// chanSample answers a channel type with `make(chan T)`.
//
// Bidirectional regardless of the parameter's spelled direction,
// because `make` is not legal on a directional channel and a
// `chan T` assigns to both `<-chan T` and `chan<- T`. The made ref
// is constructed fresh rather than reusing t, whose direction stamp
// would render the directional form.
//
// The alternate is an independent build for the same reason as
// [funcSample]'s: channels compare by identity, and two makes are
// two channels.
func chanSample(t *node.TypeRef) (sample, alternate Sample) {
	elem := ChanElem(t)
	if elem == nil {
		return refused(RefusedNoLiteral)
	}
	build := func() *emit.Expr {
		made := &node.TypeRef{
			TypeKind: node.TypeRefNamed,
			Package:  "go",
			Name:     "chan",
			TypeArgs: []*node.TypeRef{elem},
		}
		MetaIsChannel.Set(made.EnsureMeta(), true, sampleStampedBy)
		MetaChanDir.Set(made.EnsureMeta(), string(ChanBoth), sampleStampedBy)
		return &emit.Expr{ExprKind: emit.ExprMake, AsType: FromNode(made)}
	}
	return Sample{Expr: build()}, Sample{Expr: build()}
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
	// Bound to what the reference wrote before the walk descends. The
	// alias's body is written in terms of its own parameters, so
	// `Filter[string]` over `type Filter[T any] func(T) bool` sampled
	// unbound renders a func literal taking a `T` that names nothing
	// in the file it lands in.
	target := BindTypeArgs(decl.Target, decl.TypeParams, t.TypeArgs)
	inner, innerAlt := sampleRefFor(target, fieldName, r, depth-1)
	if !inner.OK() {
		// The underlying type's reason, not this one's: an alias over an
		// unloaded type is unresolved, and saying "no literal" would
		// report a fixable run as a settled fact.
		return refused(inner.Refusal)
	}
	if inner.Expr != nil {
		// The conversion the text path spells as `Weekday(42)`, built
		// as a call because the argument is an expression. Before this
		// arm existed the pair below wrapped an empty Text in a Ref —
		// a sample that was not OK and carried RefusedNone, the
		// "nothing was attempted" state, as its only explanation.
		return Sample{Expr: convertExpr(t, inner)}, Sample{Expr: convertExpr(t, innerAlt)}
	}
	ref := FromNode(t)
	// The inner's Composite rides along: a defined type over a struct
	// or collection is written `Rec{X: 42}`, and dropping the flag
	// rendered the conversion form `Rec({X: 42})` — an untyped
	// composite in a position Go denies elision to. The defined
	// type's own ref replaces the target's, which is the conversion.
	return Sample{Ref: ref, Text: inner.Text, Composite: inner.Composite},
		Sample{Ref: ref, Text: innerAlt.Text, Composite: innerAlt.Composite}
}

// convertExpr wraps an expression-form sample in a conversion to the
// defined type — `Stamp(time.Unix(42, 0))`, spelled as a call.
func convertExpr(t *node.TypeRef, inner Sample) *emit.Expr {
	return &emit.Expr{ExprKind: emit.ExprCall, Callee: typeExpr(t), Args: []*emit.Expr{inner.Expr}}
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
		// Bound for the same reason [definedSample] binds: a generic
		// struct's field is written in terms of the declaration's
		// parameters, and the reference is what says which types
		// those are here.
		ft := BindTypeArgs(f.Type, decl.TypeParams, t.TypeArgs)
		inner, innerAlt := sampleRefFor(ft, fieldName, r, depth-1)
		if !inner.OK() {
			if incomplete == RefusedNone && inner.Refusal.Incomplete() {
				incomplete = inner.Refusal
			}
			continue
		}
		if inner.Expr != nil || (inner.Ref != nil && inner.Composite) {
			// The field is writable but not as text. An expression
			// inner has no text at all; a composite inner has text
			// that is only legal where Go permits type elision, and
			// a struct field value is not such a place — composed as
			// text, a struct-typed field renders `{In: {X: 42}}`
			// and fails at format.Source. Skipping either would
			// refuse a struct whose only exported field is
			// writable.
			return Sample{Expr: keyedComposite(ref, f.Name, inner)},
				Sample{Expr: keyedComposite(ref, f.Name, innerAlt)}
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

// SamplePart renders one successful sample as an expression part for
// a composite literal the caller builds itself.
//
// The element-position counterpart to the whole-expression rule the
// Go backend's `renderSample` applies, and a different rule — which
// is why one cannot stand in for the other. In element position a
// Ref-and-Text sample drops its type, because the literal's field
// already declares it: `42` is right where `time.Duration(42)` is
// noise. A composite inner is rebuilt around its own Ref, because the
// position that routes one here is the position Go denies type
// elision to. An expression-form sample is already the part.
//
// A bare text sample becomes verbatim text, matching what the
// text-composing path does with it today: inside a composite literal
// an untyped constant needs no conversion and no import, which is why
// text composition worked at all.
//
// Exported for the consumer composing a literal this package does
// not derive — one that fills several members, or aims a member at a
// value of its own. Which members to fill is that consumer's choice;
// how one [Sample] sits inside the literal is a property of Sample
// and of Go, and a hand copy of it breaks silently when a composing
// arm changes shape. The caller owes each Sample an OK() gate first,
// as [SampleRefFor]'s contract requires everywhere else.
func SamplePart(s Sample) *emit.Expr {
	if s.Expr != nil {
		return s.Expr
	}
	if s.Ref != nil && s.Composite {
		return &emit.Expr{
			ExprKind: emit.ExprComposite,
			AsType:   s.Ref,
			Args:     []*emit.Expr{{ExprKind: emit.ExprRaw, RawText: trimBraces(s.Text)}},
		}
	}
	return &emit.Expr{ExprKind: emit.ExprRaw, RawText: s.Text}
}

// trimBraces strips the outer braces a composite sample's Text always
// carries — every composing arm builds "{...}" — so the body can ride
// as the argument of an [emit.ExprComposite], whose renderer supplies
// the braces around it.
func trimBraces(text string) string {
	return strings.TrimSuffix(strings.TrimPrefix(text, "{"), "}")
}

// typeExpr spells t as an expression callee — `pkg.Name` with the
// import registered, or the bare identifier for a local type, carrying
// the instantiation where the reference is to a generic declaration.
//
// The instantiation is an index expression because that is what Go's
// own grammar makes it: `Filter[string]` indexes the declaration by
// its argument, and the parser reads the two forms the same way. A
// callee spelled without it names a generic type uninstantiated, which
// the compiler refuses.
func typeExpr(t *node.TypeRef) *emit.Expr {
	base := &emit.Expr{ExprKind: emit.ExprIdent, Name: t.Name}
	if t.Package != "" {
		base = &emit.Expr{ExprKind: emit.ExprExternal, Pkg: t.Package, Name: t.Name}
	}
	switch len(t.TypeArgs) {
	case 0:
		return base
	case 1:
		return emit.NewIndex(base, typeExpr(t.TypeArgs[0]))
	default:
		// `T[A, B]` is one index carrying a list, which is why
		// [emit.ExprIndexList] is a kind of its own: nesting two plain
		// indexes spells `T[A][B]`, a different expression that
		// happens to parse.
		args := make([]*emit.Expr, len(t.TypeArgs))
		for i, arg := range t.TypeArgs {
			args[i] = typeExpr(arg)
		}
		return emit.NewIndexList(base, args...)
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
	if inner.Expr != nil {
		return Sample{Expr: elemComposite(ref, inner)}, Sample{Expr: elemComposite(ref, innerAlt)}
	}
	return Sample{Ref: ref, Text: "{" + inner.Text + "}", Composite: true},
		Sample{Ref: ref, Text: "{" + innerAlt.Text + "}", Composite: true}
}

// elemComposite builds the one-element composite literal for a slice
// or array whose element sample is expression-form — `[]time.Time{
// time.Unix(42, 0)}`. The whole sample rides [Sample.Expr], because a
// text with an expression-shaped hole in it is how #48 happened.
func elemComposite(ref emit.Ref, elem Sample) *emit.Expr {
	return &emit.Expr{ExprKind: emit.ExprComposite, AsType: ref, Args: []*emit.Expr{SamplePart(elem)}}
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
	if keySample.Expr != nil {
		// A keyed composite carries its keys as rendered text, so an
		// expression-form key has nowhere to live. No corpus has
		// asked for a map keyed by one; refusing names the gap where
		// composing would quietly reintroduce the text-with-a-hole.
		return refused(RefusedNoLiteral)
	}
	valSample, _ := sampleRefFor(MapValue(t), fieldName, r, depth-1)
	if !valSample.OK() {
		return refused(valSample.Refusal)
	}
	ref := FromNode(t)
	if valSample.Expr != nil {
		return Sample{Expr: keyedComposite(ref, keySample.Text, valSample)},
			Sample{Expr: keyedComposite(ref, keyAlt.Text, valSample)}
	}
	return Sample{Ref: ref, Text: "{" + keySample.Text + ": " + valSample.Text + "}", Composite: true},
		Sample{Ref: ref, Text: "{" + keyAlt.Text + ": " + valSample.Text + "}", Composite: true}
}

// keyedComposite builds the one-entry keyed composite for a map or
// struct whose value sample is expression-form. The key is rendered
// text either way — a map key came from the text path, a struct key
// is a field name.
func keyedComposite(ref emit.Ref, key string, val Sample) *emit.Expr {
	return &emit.Expr{
		ExprKind: emit.ExprCompositeKeyed,
		AsType:   ref,
		Keys:     []string{key},
		Args:     []*emit.Expr{SamplePart(val)},
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
		return refused(inner.Refusal)
	}
	ref := FromNode(t)
	if inner.Expr != nil {
		return Sample{Expr: elemComposite(ref, inner)}, Sample{Expr: elemComposite(ref, innerAlt)}
	}
	return Sample{Ref: ref, Text: "{" + inner.Text + "}", Composite: true},
		Sample{Ref: ref, Text: "{" + innerAlt.Text + "}", Composite: true}
}
