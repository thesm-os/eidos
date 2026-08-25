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

// The template-facing surface, and who registers it.
//
// Every bundle here is merged once, by the Go backend, into the
// overrideable half of its funcmap. A plugin registers none of them
// and its templates call them anyway; a plugin that wants a helper
// of its own registers that one, and one that wants to replace a
// name here supplies it through TemplateOverrides.
//
// [FuncMap] is the canonical subset. It cannot grow without bound:
// the backend rejects two plugins registering the same extension
// name, so a name added here is a name no plugin may contribute, and
// a plugin that contributed one before it existed stops building.
//
// An earlier arrangement handed every plugin its own copy of the
// optional bundles, under a name derived from the plugin. It meant a
// template called a helper by a name no declaration contained, and
// the arrangement existed only because two plugins wanting one
// helper would otherwise register it twice. Registering once, here,
// answers that without the rename.

// SigFuncMap returns the signature-rendering helpers.
//
//	{{ args .Sig }}          -> ctx, id
//	{{ callFields .Sig }}    -> Ctx: ctx, ID: id
//	{{ fails .Sig }}         -> _, err
//
// Every entry takes a [Sig]. A template passing a hand-built value
// is reconstructing the field, local and error-slot conventions the
// projection owns, which is the duplication this removes.
func SigFuncMap() template.FuncMap {
	return template.FuncMap{
		"args":        Args,
		"paramNames":  ParamNames,
		"idents":      Idents,
		"identArgs":   IdentArgs,
		"blanks":      Blanks,
		"callFields":  CallFields,
		"locals":      Locals,
		"localFields": LocalFields,
		"identFields": IdentFields,
		"namedFields": NamedFields,
		"reads":       Reads,
		"fails":       Fails,
	}
}

// QueryFuncMap returns the type and signature predicates.
//
// Separate from [SigFuncMap] because the two answer different
// questions: a branch on a type's shape needs no rendering helpers,
// and a rendered argument list needs no predicates. The split is for
// a reader of this file — the backend merges both — and it is what
// lets a bundle be described in one sentence.
func QueryFuncMap() template.FuncMap {
	return template.FuncMap{
		"isError":     IsError,
		"isContext":   IsContext,
		"isBool":      IsBool,
		"isString":    IsString,
		"isNumeric":   IsNumeric,
		"isInteger":   IsInteger,
		"isAny":       IsAny,
		"nilable":     Nilable,
		"keyable":     Keyable,
		"pointerElem": PointerElem,
		"sliceElem":   SliceElem,
		"mapKey":      MapKey,
		"mapValue":    MapValue,
		"deref":       Deref,
		"qname":       QName,
		"display":     Display,
		"localName":   LocalName,
		"zeroLiteral": TemplateZeroLiteral,
		"formatVerb":  FormatVerb,
		"quote":       Quote,
		"sequenceOf":  SequenceOf,
	}
}

// TemplateSentinelSubject is [SentinelSubject] in the shape a
// template can install — the subject alone.
//
// text/template rejects the `(string, bool)` signature at
// registration. The flag is dropped rather than travelling as an
// error because there is no failure here: a name that does not
// follow the sentinel convention has no subject, and the empty
// string is the honest answer. Compare [TemplateZeroLiteral], where
// the absent answer is a defect the render must not paper over.
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

// ConventionFuncMap returns the Go naming conventions.
//
// Rendering a name is the one thing a template does more often than
// rendering a type, and getting it wrong is silent: a test function
// whose name does not open with an upper-case rune after `Test`
// never runs, and the suite reports one fewer case than the file
// declares.
func ConventionFuncMap() template.FuncMap {
	return template.FuncMap{
		"testFuncName":      TestFuncName,
		"benchmarkFuncName": BenchmarkFuncName,
		"exampleFuncName":   ExampleFuncName,
		"constructorName":   ConstructorName,
		"getterName":        GetterName,
		"setterName":        SetterName,
		"withName":          WithName,
		"sentinelName":      SentinelName,
		"sentinelSubject":   TemplateSentinelSubject,
		"parseFuncName":     ParseFuncName,
		"doc":               Doc,
		"deprecatedDoc":     DeprecatedDoc,
	}
}

// AllFuncMap returns every optional bundle.
//
// For a plugin whose templates span the lot. Merged here rather
// than by the caller so a bundle added later reaches every consumer
// that asked for everything, instead of only those that remember to
// add the new call.
func AllFuncMap() template.FuncMap {
	out := template.FuncMap{}
	for _, bundle := range []template.FuncMap{
		SigFuncMap(), QueryFuncMap(), ConventionFuncMap(),
		EnumFuncMap(), ShapeFuncMap(), EmbedFuncMap(),
		GenericsFuncMap(),
	} {
		maps.Copy(out, bundle)
	}
	return out
}

// EnumFuncMap returns the enum vocabulary.
//
//	{{ variantText .Enum .Variant }} -> us-east
//	{{ zeroVariant .Enum }}          -> the variant at zero, or nil
//	{{ outOfRange .Enum }}           -> 4
//
// The bundle whose absence cost the most. An enum generator reaches
// for six of these, and without a bundle each one becomes a Go
// function on the plugin whose whole body is the call plus a reshape
// — which is where a generator re-decides something this package
// already decided. One that paired the underlying type with a format
// verb by hand, and drifted from [FormatVerb], printed
// `%!d(float64=0.5)` in a consumer's repository.
func EnumFuncMap() template.FuncMap {
	return template.FuncMap{
		"enumForm":       EnumFormOf,
		"enumUnderlying": EnumUnderlying,
		"variantText":    VariantText,
		"enumTexts":      EnumTexts,
		"enumTextLit":    EnumTextLiteral,
		"duplicateText":  TemplateDuplicateText,
		"zeroVariant":    TemplateZeroVariant,
		"outOfRange":     TemplateOutOfRange,
		"outOfRangeText": TemplateOutOfRangeText,
		"enumMethods":    EnumMethods,
		"enumDeclares":   EnumDeclares,
		"isIotaDerived":  IsIotaDerived,
	}
}

// ShapeFuncMap returns the standard-library shape matchers.
//
//	{{ if implementsStringer .Type }} … {{ end }}
//	{{ if isErrorMethod .Method }} … {{ end }}
//
// Thirty-five predicates over a method or a method set, none of them
// previously reachable. A template branching on whether a type
// already declares `String` is the case: without this the plugin
// answers in Go and parks a bool on an emit value, which is a field
// carrying a question rather than an answer.
func ShapeFuncMap() template.FuncMap {
	return template.FuncMap{
		"isErrorMethod":       IsErrorMethod,
		"isUnwrapMethod":      IsUnwrapMethod,
		"isIsMethod":          IsIsMethod,
		"isAsMethod":          IsAsMethod,
		"isStringMethod":      IsStringMethod,
		"isWriteMethod":       IsWriteMethod,
		"isReadMethod":        IsReadMethod,
		"isCloseMethod":       IsCloseMethod,
		"isScanMethod":        IsScanMethod,
		"isValuerMethod":      IsValuerMethod,
		"isEqualMethod":       IsEqualMethod,
		"isCompareMethod":     IsCompareMethod,
		"isCloneMethod":       IsCloneMethod,
		"isResetMethod":       IsResetMethod,
		"isValidateMethod":    IsValidateMethod,
		"isLenMethod":         IsLenMethod,
		"isLessMethod":        IsLessMethod,
		"isSwapMethod":        IsSwapMethod,
		"implementsError":     ImplementsError,
		"implementsStringer":  ImplementsStringer,
		"implementsWriter":    ImplementsWriter,
		"implementsReader":    ImplementsReader,
		"implementsSorter":    ImplementsSorter,
		"isMarshalBinary":     IsMarshalBinary,
		"isUnmarshalBinary":   IsUnmarshalBinary,
		"isMarshalText":       IsMarshalText,
		"isUnmarshalText":     IsUnmarshalText,
		"isMarshalJSON":       IsMarshalJSON,
		"isUnmarshalJSON":     IsUnmarshalJSON,
		"isGobEncode":         IsGobEncode,
		"isGobDecode":         IsGobDecode,
		"codecs":              Codecs,
		"isByteSliceAny":      IsByteSliceAny,
		"recommendedReceiver": RecommendedReceiver,
		"sameSignature":       SameSignature,
	}
}

// EmbedFuncMap returns the embedding and satisfaction helpers.
//
//	{{ range promotedFields .Struct nil }} … {{ end }}
//
// Every entry takes a [Resolver], which a template cannot construct —
// it is passed the one the plugin was handed, or nil for the
// first-level answer. That is why these are here rather than folded
// into [QueryFuncMap]: the resolver argument is the thing a template
// has to be given, and grouping them says so.
func EmbedFuncMap() template.FuncMap {
	return template.FuncMap{
		"embedIdent":        TemplateEmbedIdent,
		"embedTarget":       EmbedTarget,
		"fieldSet":          TemplateFieldSet,
		"promotedFields":    TemplatePromotedFields,
		"exportedFieldSet":  TemplateExportedFieldSet,
		"promotedMethods":   TemplatePromotedMethods,
		"embedsType":        EmbedsType,
		"underlyingOf":      UnderlyingOf,
		"comparableDeep":    TemplateComparableDeep,
		"satisfies":         TemplateSatisfies,
		"receiverIsPointer": ReceiverIsPointerDecl,
	}
}

// GenericsFuncMap returns the type-parameter and witness helpers.
//
//	{{ typeParamNames .TypeParams }} -> K, V
//	{{ witnessUse .TypeParams }}     -> [string, int]
//
// A generic double's entry point is written at concrete types, and
// choosing them is what [Witnesses] does. A template appending the
// instantiation is the natural spelling; without the bundle the
// plugin renders the bracket list in Go and hands over a string,
// which is the one shape that cannot be checked against the type
// parameters it was derived from.
func GenericsFuncMap() template.FuncMap {
	return template.FuncMap{
		"typeParamsOf":   TypeParamsOf,
		"typeParamDecls": TypeParamDecls,
		"typeParamNames": TypeParamNames,
		"typeParamRefs":  TypeParamRefs,
		"selfRef":        SelfRef,
		"witnesses":      Witnesses,
		"witnessNames":   WitnessNames,
		"witnessUse":     WitnessUse,
		"isGeneric":      IsGeneric,
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
