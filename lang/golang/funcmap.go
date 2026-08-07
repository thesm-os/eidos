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
	}
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
	} {
		maps.Copy(out, bundle)
	}
	return out
}
