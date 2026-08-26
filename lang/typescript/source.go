// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"go.thesmos.sh/eidos/core/naming"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// ErrNoFileScope reports a value that names a qualifier with no file
// to resolve it against.
var ErrNoFileScope = errors.New("typescript: no file scope to resolve a qualifier against")

// Source answers how a declaration written in TypeScript is read.
//
// The read-side half of what a plugin declares for TypeScript,
// satisfying the SDK's source-rules contract. Every method forwards
// to the function beside it in this package, which is the point: the
// notations a directive value may be written in, the module-qualifier
// lookup, the decorator vocabulary and the type classification are
// TypeScript's own rules and already live here. A plugin holding
// private copies is the same rules written twice, disagreeing the
// first time either is corrected.
//
// The zero value is usable and carries no state, so a declaration is
// `Source{}` and costs nothing to hand out.
//
// # Why this package does not import the SDK
//
// The methods take [node] types and return [emit] ones, and the
// façade's source and emit names are aliases of exactly those — so
// this satisfies the SDK interface structurally without this package
// depending on it. That keeps the boundary the package documentation
// describes: `lang/typescript` sits over [node], [emit] and `core`,
// below every consumer.
type Source struct{}

// ResolveValue splits a value written in a directive into the module
// it names and the symbol within it.
//
// Two notations, matching what an author writes. A qualified name —
// `models.User` — resolves its qualifier against the file's imports,
// so `import * as models from './models'` makes it `./models`. A bare
// name names no module.
//
// A qualifier the file does not import is an error rather than a
// guess: inventing a specifier would emit an import for a module that
// may not exist, and the failure would surface in the consumer's
// build rather than in this run.
func (Source) ResolveValue(f *node.File, value string) (pkg, symbol string, err error) {
	return ResolveValue(f, value)
}

// ResolveValue is [Source.ResolveValue] as a plain function.
func ResolveValue(f *node.File, value string) (pkg, symbol string, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", nil
	}

	idx := strings.LastIndex(value, ".")
	if idx < 0 {
		return "", value, nil
	}
	qualifier, name := value[:idx], value[idx+1:]

	if f == nil {
		return "", "", fmt.Errorf("%w: %q", ErrNoFileScope, value)
	}
	for _, imp := range f.Imports {
		if imp.LocalName() == qualifier {
			return imp.Path, name, nil
		}
	}
	return "", "", fmt.Errorf(
		"%w: %q names %q, which %s does not import",
		ErrNoFileScope, value, qualifier, fileLabel(f),
	)
}

// fileLabel names a file for a diagnostic, distinguishing "no file"
// from one whose imports fall short.
func fileLabel(f *node.File) string {
	switch {
	case f == nil:
		return "no file"
	case f.Name != "":
		return f.Name
	default:
		return "an unnamed file"
	}
}

// Tag returns the named decorator's arguments on a field, and whether
// the field carries it.
//
// A decorator is what a struct tag is in Go: the mechanism a
// framework uses to attach machine-readable metadata to a
// declaration. `Tag(field, "Column")` answers what `@Column({…})`
// carries, which is the question the SDK's tag contract asks.
//
// The first application wins where a name repeats. A caller wanting
// every one reads [DecoratorsNamed] directly; this contract returns
// one value and cannot say more.
func (Source) Tag(f *node.Field, key string) (string, bool) {
	return Tag(f, key)
}

// Tag is [Source.Tag] as a plain function.
func Tag(f *node.Field, key string) (string, bool) {
	if f == nil {
		return "", false
	}
	d, ok := DecoratorNamed(f, key)
	if !ok {
		return "", false
	}
	return d.Args, true
}

// FileOf returns the file within pkg that declared n.
func (Source) FileOf(pkg *node.Package, n node.Node) *node.File {
	return FileOf(pkg, n)
}

// FileOf is [Source.FileOf] as a plain function.
//
// Matched on the position's basename, because a position carries a
// full path while a package keys its files by name — a lookup
// composed at a call site is one truncation away from always missing,
// silently, which reads as source that imports nothing.
func FileOf(pkg *node.Package, n node.Node) *node.File {
	if pkg == nil || n == nil {
		return nil
	}
	name := n.Pos().File
	if name == "" {
		return nil
	}
	return pkg.FileByName(path.Base(name))
}

// Settable returns the members of s a constructor can assign.
//
// Readonly and static members are excluded. A readonly property is
// assignable only in the constructor of its own class, so a
// constructor generated in another module cannot set it; a static one
// belongs to the class rather than to an instance.
//
// Unlike Go there is no export rule to apply: TypeScript's `private`
// and `protected` are compile-time, and a generated constructor in
// another module cannot name them either — so those are excluded on
// the same ground.
func (Source) Settable(s *node.Struct) []emit.Member {
	return Settable(s)
}

// Settable is [Source.Settable] as a plain function.
func Settable(s *node.Struct) []emit.Member {
	if s == nil {
		return nil
	}
	out := make([]emit.Member, 0, len(s.Fields))
	for _, f := range s.Fields {
		if !settableField(f) {
			continue
		}
		out = append(out, emit.Member{
			Name:   f.Name,
			Type:   FromNode(f.Type),
			Source: f,
		})
	}
	return out
}

// settableField reports whether a constructor elsewhere can assign f.
func settableField(f *node.Field) bool {
	if ro, _ := MetaReadonly.Get(f.Meta()); ro {
		return false
	}
	if st, _ := MetaStatic.Get(f.Meta()); st {
		return false
	}
	if v, ok := MetaVisibility.Get(f.Meta()); ok && v != VisibilityPublic {
		return false
	}
	return true
}

// TypeOf classifies a TypeScript type into the shape vocabulary.
//
// The order of the arms is the classification: a nullable union is
// read as optional before it is read as a union, because what makes
// `T | null` interesting to a generator is that the value may be
// absent, not that it has two members.
//
// TypeScript has no set type — there is `Set<T>`, but it is an
// ordinary generic class rather than a shape the language
// distinguishes — so [emit.ShapeSet] never arises here.
func (Source) TypeOf(t *node.TypeRef, r node.Resolver) emit.TypeInfo {
	return TypeOf(t, r)
}

// TypeOf is [Source.TypeOf] as a plain function.
func TypeOf(t *node.TypeRef, _ node.Resolver) emit.TypeInfo {
	switch {
	case t == nil:
		return emit.TypeInfo{Shape: emit.ShapeScalar}
	case isUint8Array(t):
		return emit.TypeInfo{Shape: emit.ShapeBytes}
	case nullableUnion(t) != nil:
		return emit.TypeInfo{Shape: emit.ShapeOptional, Elem: FromNode(nullableUnion(t))}
	case t.IsSlice(), t.IsArray():
		return emit.TypeInfo{Shape: emit.ShapeSequence, Elem: FromNode(t.Elem)}
	case t.IsMap():
		return emit.TypeInfo{
			Shape: emit.ShapeMapping,
			Key:   FromNode(t.MapKey),
			Elem:  FromNode(t.MapValue),
		}
	default:
		return emit.TypeInfo{Shape: emit.ShapeScalar}
	}
}

// isUint8Array reports the type TypeScript uses for a byte sequence.
func isUint8Array(t *node.TypeRef) bool {
	return t != nil && t.TypeKind == node.TypeRefNamed && t.Name == "Uint8Array"
}

// nullableUnion returns the member a `T | null` union carries besides
// its absent values, or nil when t is not one.
//
// Only a two-member union qualifies. `A | B | null` is a union that
// happens to admit null, and calling it optional would name one of
// its members as the type — which one being an arbitrary choice.
func nullableUnion(t *node.TypeRef) *node.TypeRef {
	if !IsUnion(t) {
		return nil
	}
	members := Members(t)
	if len(members) != 2 {
		return nil
	}
	switch {
	case isAbsent(members[0]):
		return members[1]
	case isAbsent(members[1]):
		return members[0]
	default:
		return nil
	}
}

// isAbsent reports whether a member is `null` or `undefined`.
func isAbsent(t *node.TypeRef) bool {
	if t == nil {
		return false
	}
	lit, ok := MetaLiteralType.Get(t.Meta())
	if !ok {
		lit = t.Name
	}
	return lit == TypeNull || lit == TypeUndefined
}

// SamplesOf returns two distinct values of a TypeScript type.
//
// Two rather than one because a generated test asserting a round trip
// needs a value and something it is not; one sample would let an
// implementation that ignores its argument pass.
func (Source) SamplesOf(t *node.TypeRef, hint string, r node.Resolver) (sample, alternate emit.Sample) {
	return SamplesOf(t, hint, r)
}

// SamplesOf is [Source.SamplesOf] as a plain function.
func SamplesOf(t *node.TypeRef, hint string, _ node.Resolver) (sample, alternate emit.Sample) {
	ref := FromNode(t)
	first, second, ok := sampleTexts(t, hint)
	if !ok {
		return emit.Sample{Ref: ref}, emit.Sample{Ref: ref}
	}
	return emit.Sample{Ref: ref, Text: first}, emit.Sample{Ref: ref, Text: second}
}

// sampleTexts returns two distinct literals for a type, or ok=false
// where none can be written.
//
// A type with no literal form — a named interface, a function — gets
// no sample rather than a plausible one: a generated fixture holding
// an invented value is worse than one the author has to fill in,
// because it compiles.
func sampleTexts(t *node.TypeRef, hint string) (first, second string, ok bool) {
	if t == nil {
		return "", "", false
	}

	switch {
	case t.IsSlice(), t.IsArray():
		elemFirst, elemSecond, elemOK := sampleTexts(t.Elem, hint)
		if !elemOK {
			return "[]", "[]", false
		}
		return "[" + elemFirst + "]", "[" + elemFirst + ", " + elemSecond + "]", true
	case t.IsMap():
		return "{}", "{}", false
	case nullableUnion(t) != nil:
		inner, _, innerOK := sampleTexts(nullableUnion(t), hint)
		if !innerOK {
			return "", "", false
		}
		return inner, TypeNull, true
	}

	return scalarSamples(t, hint)
}

// scalarSamples returns two literals for a non-composite type.
func scalarSamples(t *node.TypeRef, hint string) (first, second string, ok bool) {
	switch t.Name {
	case ScalarString:
		if hint != "" {
			return Quote(hint), Quote(hint + "-2"), true
		}
		return Quote("a"), Quote("b"), true
	case ScalarNumber:
		return "1", "2", true
	case ScalarBigInt:
		return "1n", "2n", true
	case ScalarBoolean:
		return LiteralTrue, LiteralFalse, true
	default:
		return "", "", false
	}
}

// TypeParams lifts a generic parameter list into the emit form.
func (Source) TypeParams(params []*node.TypeParam) []*emit.TypeParam {
	return TypeParams(params)
}

// TypeParams is [Source.TypeParams] as a plain function.
func TypeParams(params []*node.TypeParam) []*emit.TypeParam {
	out := make([]*emit.TypeParam, 0, len(params))
	for _, p := range params {
		tp := &emit.TypeParam{Name: p.Name}
		if p.Constraint != nil {
			tp.Constraint = &emit.Constraint{Raw: p.Constraint.Raw}
			for _, e := range p.Constraint.Embedded {
				tp.Constraint.Embedded = append(tp.Constraint.Embedded, FromNode(e))
			}
		}
		out = append(out, tp)
	}
	return out
}

// TypeArgs renders a parameter list in use position — the `<T, U>` a
// reference to the generic type spells.
func (Source) TypeArgs(params []*node.TypeParam) string { return TypeArgs(params) }

// TypeArgs is [Source.TypeArgs] as a plain function.
func TypeArgs(params []*node.TypeParam) string {
	if len(params) == 0 {
		return ""
	}
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, Ident(p.Name))
	}
	return "<" + strings.Join(names, ", ") + ">"
}

// Witnesses returns one concrete type per generic parameter, or nil
// when any carries a bound that cannot be satisfied by choosing.
//
// A generated test instantiating a generic type has to pick something
// for each parameter. An unconstrained one takes `unknown`, which
// every value satisfies. A bounded one cannot be chosen for without
// resolving the bound, which needs a type checker this adapter does
// not have — so the whole set is refused rather than half-answered.
func (Source) Witnesses(params []*node.TypeParam) []emit.Ref {
	return Witnesses(params)
}

// Witnesses is [Source.Witnesses] as a plain function.
func Witnesses(params []*node.TypeParam) []emit.Ref {
	out := make([]emit.Ref, 0, len(params))
	for _, p := range params {
		if p.Constraint != nil && len(p.Constraint.Embedded) > 0 {
			return nil
		}
		out = append(out, emit.Builtin(TypeUnknown))
	}
	return out
}

// SubstituteParams replaces every use of a generic parameter in t
// with the parameter itself.
//
// A no-op for TypeScript, and deliberately so. Go needs this because
// a method on a generic receiver names the receiver's parameters in
// its own signature, and a generator emitting that method elsewhere
// has to rebind them. TypeScript's parameters are lexically scoped to
// the declaration, so a use site outside it cannot name one — there
// is nothing to substitute.
func (Source) SubstituteParams(t *node.TypeRef, _ []*node.TypeParam) *node.TypeRef {
	return t
}

// SubstituteRef is [Source.SubstituteParams] on the emit side, and a
// no-op for the same reason.
func (Source) SubstituteRef(r emit.Ref, _ []*node.TypeParam) emit.Ref { return r }

// LiteralFor renders text as a literal of type t, reporting whether
// it could.
//
// The question a directive value asks: an author wrote `default=3` on
// a numeric field, and the generator needs `3` rather than `'3'`. A
// type with no literal form reports false rather than quoting, which
// would produce a string where the declaration promised something
// else.
func (Source) LiteralFor(f *node.File, t *node.TypeRef, text string, r node.Resolver) (string, bool) {
	return LiteralFor(f, t, text, r)
}

// LiteralFor is [Source.LiteralFor] as a plain function.
func LiteralFor(_ *node.File, t *node.TypeRef, text string, _ node.Resolver) (string, bool) {
	if t == nil || text == "" {
		return "", false
	}
	switch t.Name {
	case ScalarString:
		return Quote(text), true
	case ScalarNumber, ScalarBigInt:
		return text, true
	case ScalarBoolean:
		if text == LiteralTrue || text == LiteralFalse {
			return text, true
		}
		return "", false
	default:
		return "", false
	}
}

// ZeroLiteral renders the value a declaration of type t holds before
// anything assigns to it.
//
// TypeScript has no zero value: an unassigned binding is `undefined`
// whatever its declared type, and `strictNullChecks` makes assigning
// that to a non-nullable type an error. So this answers only for the
// types with a conventional empty value, and refuses the rest rather
// than inventing one the compiler would reject.
func (Source) ZeroLiteral(t *node.TypeRef, r node.Resolver) (string, bool) {
	return ZeroLiteral(t, r)
}

// ZeroLiteral is [Source.ZeroLiteral] as a plain function.
func ZeroLiteral(t *node.TypeRef, _ node.Resolver) (string, bool) {
	if t == nil {
		return "", false
	}
	switch {
	case t.IsSlice(), t.IsArray():
		return "[]", true
	case t.IsMap():
		return "{}", true
	case nullableUnion(t) != nil:
		return TypeNull, true
	}
	switch t.Name {
	case ScalarString:
		return "''", true
	case ScalarNumber:
		return "0", true
	case ScalarBigInt:
		return "0n", true
	case ScalarBoolean:
		return LiteralFalse, true
	default:
		return "", false
	}
}

// TypeName joins parts into a TypeScript type identifier.
//
// PascalCase, which every TypeScript style guide agrees on for a
// type, and made bindable so a part that collides with a keyword does
// not produce an unusable name.
func (Source) TypeName(parts ...string) string { return TypeName(parts...) }

// TypeName is [Source.TypeName] as a plain function.
func TypeName(parts ...string) string {
	joined := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			joined = append(joined, p)
		}
	}
	return Ident(naming.Pascal(strings.Join(joined, " ")))
}

// ConstructorName returns the name of a function that builds a value
// of the named type.
//
// `createUser`, not `newUser` or `NewUser`. TypeScript has no
// constructor-function convention the way Go's `New` prefix is one —
// a class is constructed with `new` — so a generated builder is an
// ordinary function, and `create` is the verb the ecosystem's
// factories use.
func (Source) ConstructorName(base string) string { return ConstructorName(base) }

// ConstructorName is [Source.ConstructorName] as a plain function.
func ConstructorName(base string) string {
	return Ident(naming.Camel("create " + base))
}

// EnumOf projects a declared enumeration into the neutral form.
//
// TypeScript's enum is first-class, so unlike Go's there is no
// constant group to recognise — the constants a caller holds
// contribute nothing and are ignored. A variant declared outside the
// type is not expressible: declaration merging can add members to an
// enum, but only within one module, so [emit.EnumInfo.Foreign] is
// always empty rather than unknown.
func (Source) EnumOf(e *node.Enum, constants []*node.Constant) emit.EnumInfo {
	return EnumOf(e, constants)
}

// SigOf lifts one method into the form a generator renders.
func (Source) SigOf(m *node.Method) *emit.SigInfo { return SigOf(m) }

// IsConstraint reports whether an interface declares a type set
// rather than a method-set contract — never, in TypeScript.
func (Source) IsConstraint(i *node.Interface) bool { return IsConstraint(i) }

// SentinelName spells the identifier a declared error is named under.
func (Source) SentinelName(base string) string { return SentinelName(base) }

// IsSentinelName reports whether an identifier follows that
// convention.
func (Source) IsSentinelName(ident string) bool { return IsSentinelName(ident) }

// ErrorOf projects a declaration into the error contract it carries.
func (Source) ErrorOf(s *node.Struct, r node.Resolver) (emit.ErrorInfo, bool) {
	return ErrorOf(s, r)
}
