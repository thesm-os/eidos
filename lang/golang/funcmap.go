// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"errors"
	"fmt"
	"maps"
	"text/template"

	"go.thesmos.sh/eidos/node"
)

// The template-facing surface, and why it is two bundles rather
// than one.
//
// [FuncMap] is the canonical set every Go backend merges once. It
// cannot grow without bound: the backend rejects two plugins
// registering the same extension name outright, so a name added
// here is a name no plugin may ever contribute, and a plugin that
// contributed one before it existed stops building.
//
// [SigFuncMap] is opt-in and prefixed for that reason. A plugin
// rendering signatures asks for the bundle under its own prefix,
// two plugins can both have it, and the helpers themselves are
// shared in Go — which is the coupling that survives a rename.

// SigFuncMap returns the signature-rendering helpers under the
// given prefix, for a plugin to contribute through its own
// TemplateFuncs implementation.
//
//	{{ eidosArgs .Sig }}          -> ctx, id
//	{{ eidosCallFields .Sig }}    -> Ctx: ctx, ID: id
//	{{ eidosFails .Sig }}         -> _, err
//
// Prefixed rather than shared under one name because the backend
// rejects a duplicate extension registration at Build time: an
// unprefixed bundle would fail every run in which two plugins
// wanted it, rather than one output. An empty prefix is accepted
// for a consumer that has confirmed it is the only claimant.
//
// Every entry takes a [Sig]. A template passing a hand-built value
// is reconstructing the field, local and error-slot conventions the
// projection owns, which is the duplication this removes.
func SigFuncMap(prefix string) template.FuncMap {
	return template.FuncMap{
		prefix + "args":        Args,
		prefix + "paramNames":  ParamNames,
		prefix + "idents":      Idents,
		prefix + "identArgs":   IdentArgs,
		prefix + "blanks":      Blanks,
		prefix + "callFields":  CallFields,
		prefix + "locals":      Locals,
		prefix + "localFields": LocalFields,
		prefix + "identFields": IdentFields,
		prefix + "namedFields": NamedFields,
		prefix + "reads":       Reads,
		prefix + "fails":       Fails,
	}
}

// QueryFuncMap returns the type and signature predicates under the
// given prefix.
//
// Separate from [SigFuncMap] because the two answer different
// questions and a template usually wants one of them: a branch on a
// type's shape needs no rendering helpers, and a rendered argument
// list needs no predicates. Contributing only what a template uses
// keeps the funcmap a plugin registers small enough to read.
func QueryFuncMap(prefix string) template.FuncMap {
	return template.FuncMap{
		prefix + "isError":     IsError,
		prefix + "isContext":   IsContext,
		prefix + "isBool":      IsBool,
		prefix + "isString":    IsString,
		prefix + "isNumeric":   IsNumeric,
		prefix + "isInteger":   IsInteger,
		prefix + "isAny":       IsAny,
		prefix + "nilable":     Nilable,
		prefix + "keyable":     Keyable,
		prefix + "pointerElem": PointerElem,
		prefix + "sliceElem":   SliceElem,
		prefix + "mapKey":      MapKey,
		prefix + "mapValue":    MapValue,
		prefix + "deref":       Deref,
		prefix + "qname":       QName,
		prefix + "display":     Display,
		prefix + "localName":   LocalName,
		prefix + "zeroLiteral": TemplateZeroLiteral,
		prefix + "formatVerb":  FormatVerb,
		prefix + "quote":       Quote,
		prefix + "sequenceOf":  SequenceOf,
	}
}

// TemplateSentinelSubject is [SentinelSubject] in the shape a
// template can install — the subject alone.
//
// text/template rejects the `(string, bool)` signature at
// registration. The flag is dropped rather than travelling as an
// error because there is no failure here: a name carrying no prefix
// is returned whole, which is exactly what a template interpolating
// a subject wants. A caller that needs to *distinguish* the two
// branches asks [IsSentinelName], which a template can also call.
func TemplateSentinelSubject(ident string) string {
	subject, _ := SentinelSubject(ident)
	return subject
}

// TemplateZeroLiteral is [ZeroLiteral] in the shape a template can
// install.
//
// text/template accepts a function returning one value, or two
// where the second is an error; a `(string, bool)` signature panics
// at registration. The refusal therefore travels as an error, which
// aborts the render — the loud failure, and the right one: a
// template that received the empty string instead would emit a
// composite literal with a missing value, and the consumer's
// compiler would report it against generated code.
func TemplateZeroLiteral(t *node.TypeRef) (string, error) {
	lit, ok := ZeroLiteral(t)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrNoZeroValue, QName(t))
	}
	return lit, nil
}

// ErrNoZeroValue is returned by [TemplateZeroLiteral] for a type
// whose zero this package cannot derive.
//
// A named non-interface type is the common case: the model records
// a package and an identifier, and the zero of a struct is `T{}`
// while the zero of a defined numeric type is `0`. A caller holding
// the graph resolves it through [ZeroLiteralFor]; a template cannot.
var ErrNoZeroValue = errors.New("golang: no derivable zero value")

// ConventionFuncMap returns the Go naming conventions under the
// given prefix.
//
// Rendering a name is the one thing a template does more often than
// rendering a type, and getting it wrong is silent: a test function
// whose name does not open with an upper-case rune after `Test`
// never runs, and the suite reports one fewer case than the file
// declares.
func ConventionFuncMap(prefix string) template.FuncMap {
	return template.FuncMap{
		prefix + "testFuncName":      TestFuncName,
		prefix + "benchmarkFuncName": BenchmarkFuncName,
		prefix + "exampleFuncName":   ExampleFuncName,
		prefix + "constructorName":   ConstructorName,
		prefix + "getterName":        GetterName,
		prefix + "setterName":        SetterName,
		prefix + "withName":          WithName,
		prefix + "sentinelName":      SentinelName,
		prefix + "sentinelSubject":   TemplateSentinelSubject,
		prefix + "parseFuncName":     ParseFuncName,
		prefix + "doc":               Doc,
		prefix + "deprecatedDoc":     DeprecatedDoc,
	}
}

// AllFuncMap returns every optional bundle under one prefix.
//
// For a plugin whose templates span the lot. Merged here rather
// than by the caller so a bundle added later reaches every consumer
// that asked for everything, instead of only those that remember to
// add the new call.
func AllFuncMap(prefix string) template.FuncMap {
	out := template.FuncMap{}
	for _, bundle := range []template.FuncMap{
		SigFuncMap(prefix), QueryFuncMap(prefix), ConventionFuncMap(prefix),
		EnumFuncMap(prefix), ShapeFuncMap(prefix), EmbedFuncMap(prefix),
		GenericsFuncMap(prefix),
	} {
		maps.Copy(out, bundle)
	}
	return out
}

// EnumFuncMap returns the enum vocabulary under the given prefix.
//
//	{{ eidosVariantText .Enum .Variant }} -> us-east
//	{{ eidosZeroVariant .Enum }}          -> the variant at zero, or nil
//	{{ eidosOutOfRange .Enum }}           -> 4
//
// The bundle whose absence cost the most. An enum generator reaches
// for six of these, and without a bundle each one becomes a Go
// function on the plugin whose whole body is the call plus a reshape
// — which is where a generator re-decides something this package
// already decided. One that paired the underlying type with a format
// verb by hand, and drifted from [FormatVerb], printed
// `%!d(float64=0.5)` in a consumer's repository.
func EnumFuncMap(prefix string) template.FuncMap {
	return template.FuncMap{
		prefix + "enumForm":       EnumFormOf,
		prefix + "enumUnderlying": EnumUnderlying,
		prefix + "variantText":    VariantText,
		prefix + "enumTexts":      EnumTexts,
		prefix + "enumTextLit":    EnumTextLiteral,
		prefix + "duplicateText":  TemplateDuplicateText,
		prefix + "zeroVariant":    TemplateZeroVariant,
		prefix + "outOfRange":     TemplateOutOfRange,
		prefix + "outOfRangeText": TemplateOutOfRangeText,
		prefix + "enumMethods":    EnumMethods,
		prefix + "enumDeclares":   EnumDeclares,
		prefix + "isIotaDerived":  IsIotaDerived,
	}
}

// ShapeFuncMap returns the standard-library shape matchers under the
// given prefix.
//
//	{{ if eidosImplementsStringer .Type }} … {{ end }}
//	{{ if eidosIsErrorMethod .Method }} … {{ end }}
//
// Thirty-five predicates over a method or a method set, none of them
// previously reachable. A template branching on whether a type
// already declares `String` is the case: without this the plugin
// answers in Go and parks a bool on an emit value, which is a field
// carrying a question rather than an answer.
func ShapeFuncMap(prefix string) template.FuncMap {
	return template.FuncMap{
		prefix + "isErrorMethod":       IsErrorMethod,
		prefix + "isUnwrapMethod":      IsUnwrapMethod,
		prefix + "isIsMethod":          IsIsMethod,
		prefix + "isAsMethod":          IsAsMethod,
		prefix + "isStringMethod":      IsStringMethod,
		prefix + "isWriteMethod":       IsWriteMethod,
		prefix + "isReadMethod":        IsReadMethod,
		prefix + "isCloseMethod":       IsCloseMethod,
		prefix + "isScanMethod":        IsScanMethod,
		prefix + "isValuerMethod":      IsValuerMethod,
		prefix + "isEqualMethod":       IsEqualMethod,
		prefix + "isCompareMethod":     IsCompareMethod,
		prefix + "isCloneMethod":       IsCloneMethod,
		prefix + "isResetMethod":       IsResetMethod,
		prefix + "isValidateMethod":    IsValidateMethod,
		prefix + "isLenMethod":         IsLenMethod,
		prefix + "isLessMethod":        IsLessMethod,
		prefix + "isSwapMethod":        IsSwapMethod,
		prefix + "implementsError":     ImplementsError,
		prefix + "implementsStringer":  ImplementsStringer,
		prefix + "implementsWriter":    ImplementsWriter,
		prefix + "implementsReader":    ImplementsReader,
		prefix + "implementsSorter":    ImplementsSorter,
		prefix + "isMarshalBinary":     IsMarshalBinary,
		prefix + "isUnmarshalBinary":   IsUnmarshalBinary,
		prefix + "isMarshalText":       IsMarshalText,
		prefix + "isUnmarshalText":     IsUnmarshalText,
		prefix + "isMarshalJSON":       IsMarshalJSON,
		prefix + "isUnmarshalJSON":     IsUnmarshalJSON,
		prefix + "isGobEncode":         IsGobEncode,
		prefix + "isGobDecode":         IsGobDecode,
		prefix + "codecs":              Codecs,
		prefix + "isByteSliceAny":      IsByteSliceAny,
		prefix + "recommendedReceiver": RecommendedReceiver,
		prefix + "sameSignature":       SameSignature,
	}
}

// EmbedFuncMap returns the embedding and satisfaction helpers under
// the given prefix.
//
//	{{ range eidosPromotedFields .Struct nil }} … {{ end }}
//
// Every entry takes a [Resolver], which a template cannot construct —
// it is passed the one the plugin was handed, or nil for the
// first-level answer. That is why these are here rather than folded
// into [QueryFuncMap]: the resolver argument is the thing a template
// has to be given, and grouping them says so.
func EmbedFuncMap(prefix string) template.FuncMap {
	return template.FuncMap{
		prefix + "embedIdent":        TemplateEmbedIdent,
		prefix + "embedTarget":       EmbedTarget,
		prefix + "fieldSet":          TemplateFieldSet,
		prefix + "promotedFields":    TemplatePromotedFields,
		prefix + "exportedFieldSet":  TemplateExportedFieldSet,
		prefix + "promotedMethods":   TemplatePromotedMethods,
		prefix + "embedsType":        EmbedsType,
		prefix + "underlyingOf":      UnderlyingOf,
		prefix + "comparableDeep":    TemplateComparableDeep,
		prefix + "satisfies":         TemplateSatisfies,
		prefix + "receiverIsPointer": ReceiverIsPointerDecl,
	}
}

// GenericsFuncMap returns the type-parameter and witness helpers
// under the given prefix.
//
//	{{ eidosTypeParamNames .TypeParams }} -> K, V
//	{{ eidosWitnessUse .TypeParams }}     -> [string, int]
//
// A generic double's entry point is written at concrete types, and
// choosing them is what [Witnesses] does. A template appending the
// instantiation is the natural spelling; without the bundle the
// plugin renders the bracket list in Go and hands over a string,
// which is the one shape that cannot be checked against the type
// parameters it was derived from.
func GenericsFuncMap(prefix string) template.FuncMap {
	return template.FuncMap{
		prefix + "typeParamsOf":   TypeParamsOf,
		prefix + "typeParamDecls": TypeParamDecls,
		prefix + "typeParamNames": TypeParamNames,
		prefix + "typeParamRefs":  TypeParamRefs,
		prefix + "selfRef":        SelfRef,
		prefix + "witnesses":      Witnesses,
		prefix + "witnessNames":   WitnessNames,
		prefix + "witnessUse":     WitnessUse,
		prefix + "isGeneric":      IsGeneric,
	}
}

// TemplateZeroVariant is [ZeroVariant] in the shape a template can
// install.
//
// A `(T, bool)` signature panics at registration, so the absence
// travels as a nil the template tests with `{{ if }}` rather than as
// an error: an enum whose zero is not a declared variant is the
// ordinary case — a typed-iota set starting at one — not a failure.
func TemplateZeroVariant(e *node.Enum) *node.EnumVariant {
	v, ok := ZeroVariant(e)
	if !ok {
		return nil
	}
	return v
}

// TemplateOutOfRange is [OutOfRangeLiteral] in the shape a template
// can install.
//
// Empty rather than an error, for the same reason: a set saturating
// its type has no value outside it, and a generator's correct
// response is to omit the probe rather than to abort the render.
func TemplateOutOfRange(e *node.Enum) string {
	lit, ok := OutOfRangeLiteral(e)
	if !ok {
		return ""
	}
	return lit
}

// TemplateOutOfRangeText is [OutOfRangeText] in the shape a template
// can install. Empty when the declared set already carries the
// marker, which is the one case a probe would assert the opposite of
// what it means.
func TemplateOutOfRangeText(e *node.Enum) string {
	text, ok := OutOfRangeText(e)
	if !ok {
		return ""
	}
	return text
}

// TemplateFieldSet is [FieldSet] in the shape a template can install
// — and the pattern its three siblings below follow.
//
// Each returns `(T, error)` where the library returns
// `(T, []UnresolvedEmbed)`, and the error is non-nil exactly when the
// walk could not complete. That is the loud failure and the right
// one: the slice says the answer is smaller than the truth, and a
// template rendering it anyway emits a builder short a setter or a
// double short a method — which the consumer's compiler reports
// against generated code. A caller that wants to decide for itself
// holds the graph and calls the library form.
func TemplateFieldSet(s *node.Struct, r Resolver) ([]PromotedField, error) {
	out, problems := FieldSet(s, r)
	return out, embedWalkError("FieldSet", problems)
}

// TemplatePromotedFields is [PromotedFields] for a template.
func TemplatePromotedFields(s *node.Struct, r Resolver) ([]PromotedField, error) {
	out, problems := PromotedFields(s, r)
	return out, embedWalkError("PromotedFields", problems)
}

// TemplateExportedFieldSet is [ExportedFieldSet] for a template.
func TemplateExportedFieldSet(s *node.Struct, r Resolver) ([]PromotedField, error) {
	out, problems := ExportedFieldSet(s, r)
	return out, embedWalkError("ExportedFieldSet", problems)
}

// TemplatePromotedMethods is [PromotedMethods] for a template.
func TemplatePromotedMethods(s *node.Struct, r Resolver) ([]PromotedMethod, error) {
	out, problems := PromotedMethods(s, r)
	return out, embedWalkError("PromotedMethods", problems)
}

// TemplateComparableDeep is [ComparableDeep] for a template.
//
// The error carries the same weight as the walks above: an
// unreachable type is not evidence of comparability, and a template
// keying a map on the answer would emit one the consumer cannot
// compile.
func TemplateComparableDeep(t *node.TypeRef, r Resolver) (bool, error) {
	equalable, problems := ComparableDeep(t, r)
	if len(problems) == 0 {
		return equalable, nil
	}
	return false, fmt.Errorf("%w: ComparableDeep could not reach %s",
		ErrIncompleteWalk, problems[0].Written)
}

// embedWalkError renders an incomplete walk as the error a template
// aborts on, naming the first embed and how many followed it.
func embedWalkError(walk string, problems []UnresolvedEmbed) error {
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s could not reach %s in %s (%s), and %d other(s)",
		ErrIncompleteWalk, walk, problems[0].Written, problems[0].Host,
		problems[0].Reason, len(problems)-1)
}

// ErrIncompleteWalk is returned by a template-shaped walk that could
// not resolve every embed or type it needed.
//
// Distinct from [ErrNoZeroValue] because the remedy differs: a zero
// this package cannot derive needs a caller holding the graph, while
// an incomplete walk usually means the run was narrower than the
// declaration — the package carrying the embed was not loaded.
var ErrIncompleteWalk = errors.New("golang: walk did not complete")

// TemplateSatisfies is [Satisfies] for a template — the verdict
// alone.
//
// The missing-method detail is dropped rather than raised as an
// error, because false is an ordinary answer here: a template asking
// whether a type satisfies an interface is branching, not failing. A
// caller wanting to name what is missing is writing a diagnostic, and
// a template is the wrong place to write one.
func TemplateSatisfies(have, want []*node.Method) bool {
	ok, _ := Satisfies(have, want)
	return ok
}

// TemplateEmbedIdent is [EmbedIdent] for a template — the contributed
// field name, empty when the embed names none.
//
// The pointer half is dropped because a template asking for the name
// is spelling a selector, and whether the embed was by pointer does
// not change it. [EmbedIdent] answers both for a caller allocating.
func TemplateEmbedIdent(e *node.Embed) string {
	name, _ := EmbedIdent(e)
	return name
}

// TemplateDuplicateText is [DuplicateText] for a template — the
// colliding text, empty when the set has none.
//
// A collision makes one variant unreachable through Parse, so a
// template usually branches on it to withhold the round-trip check
// rather than to abort.
func TemplateDuplicateText(e *node.Enum) string {
	text, _ := DuplicateText(e)
	return text
}
