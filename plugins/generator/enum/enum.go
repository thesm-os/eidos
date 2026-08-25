// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package enum is the production multi-output generator for
// idiomatic enum types. A source [sdk.Enum] (e.g. a Go
// `type Status int` plus its grouped const variants) drives
// the generation of two files: a per-source-basename file
// carrying the production API surface (`String`, `Parse<Type>`,
// `MarshalJSON`, `UnmarshalJSON`) and a paired test file
// pinning the API's contract.
//
// # Language-agnostic core, language-keyed adapters
//
// This file holds the plugin's language-neutral core: the
// directive schema, the [Options] surface, the [API] and
// [Tests] emit values, and the [Plugin.Generate] pass that
// walks annotated source enums and queues one contribution
// against each output. Per-language behaviour — the file
// suffix and tag pair, the embedded template tree — lives in
// the sibling `enum_<lang>.go` adapter, which [New] hands to
// the embedded [sdk.Base]. The base answers the declaration
// methods for Go and reports *not provided* for every other
// language, so a second target language ships a
// `templates/<lang>/...` tree, an `enum_<lang>.go` adapter,
// and the per-language dispatch the base does not model.
//
// # Source detection
//
// The plugin opts in on each source [sdk.Enum] carrying a
// `+gen:enum` directive. The enum's [sdk.EnumVariant] list
// supplies the rendered string-form for each variant, with
// two resolution layers stacked low-to-high:
//
//  1. Default: the variant's [sdk.EnumVariant.Name] with
//     the enum's [sdk.Enum.Name] prefix stripped when
//     present and [Options.StripPrefix] is true (the
//     default). So `StatusActive` on `type Status int`
//     renders as the string `"Active"`.
//  2. Override: a `+gen:value <override>` directive on the
//     variant pins the rendered string verbatim, regardless
//     of [Options.StripPrefix]. Source authors reach for the
//     override when the default-derived string clashes with
//     an external protocol's spelling.
//
// # Output set
//
// Two outputs flow from one source enum:
//
//   - Primary file: hosts the [API] kind, rendered by the
//     `enum.api` template. Carries the full production
//     surface.
//   - Tagged "test" file: hosts the [Tests] kind, rendered
//     by the `enum.test` template. In the Go adapter the
//     `_test.go` suffix triggers the framework's automatic
//     `<pkg>_test` package shift so the tests live in an
//     external test package and can't accidentally read
//     private state.
//
// # Imports
//
// The plugin expresses every cross-package and stdlib
// reference through [sdk.NewExternal] expressions. The
// backend's `renderExpr` funcmap registers each referenced
// package on the rendered file's import set automatically —
// the templates carry no hard-coded import statements.
package enum

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "enum"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

// Capability is the capability label the plugin advertises
// so downstream consumers can declare
// `Requires: []string{Capability}` to document a dependency
// on enum-API generation.
const Capability = "enum"

// DirectiveName is the bare directive name (without the
// `+gen:` or `-gen:` prefix) the plugin reads from source
// enum types.
const DirectiveName sdk.DirectiveName = "enum"

// SlotName is the [sdk.EmitFile] slot the plugin appends its
// rendered [API] / [Tests] emit values into. `top` renders
// the content between the package clause and the first core
// decl — the natural placement for whole methods +
// functions emitted as a single template-rendered block.
// Universal across target languages.
const SlotName = "top"

// KindAPI is the plugin-defined [sdk.Kind] every [API] emit
// value reports. Universal across target languages; the
// matching template per language lives at
// `templates/<lang>/enum.api.tmpl`.
const KindAPI sdk.Kind = "enum.api"

// KindTests is the plugin-defined [sdk.Kind] every [Tests]
// emit value reports. Universal across target languages;
// the matching template per language lives at
// `templates/<lang>/enum.test.tmpl`.
const KindTests sdk.Kind = "enum.test"

// ErrEnumHasNoVariants is recorded as a diagnostic when an
// annotated source enum carries no [sdk.EnumVariant]
// entries — the plugin has nothing to render against. The
// run continues; the offending enum is skipped.
var ErrEnumHasNoVariants = errors.New("enum: source enum has no variants")

// Options carries the plugin's user-tunable settings.
//
// The default values reflect Go idioms — `Parse` /
// `ErrUnknown` prefixes match the canonical Go naming —
// because Go is the only language the plugin ships
// templates for today. Project configs in other-language
// projects override the defaults via the YAML.
type Options struct {
	// StripPrefix toggles the default name-to-string
	// resolution rule: when true (the default), a variant
	// whose [sdk.EnumVariant.Name] starts with the enum's
	// [sdk.Enum.Name] renders with the prefix stripped
	// (e.g. `StatusActive` → `"Active"`). The
	// `+gen:value <override>` directive on a variant
	// overrides both branches.
	StripPrefix bool `eidos:"strip_prefix,default=true"`

	// ParsePrefix is the prefix the plugin uses to form the
	// parse function's identifier. Combined with the enum's
	// type name yields `ParseStatus`. Defaults to
	// [GoDefaultParsePrefix].
	ParsePrefix string `eidos:"parse_prefix,default=Parse"`

	// SentinelPrefix is the prefix the plugin uses to form
	// the parse-error sentinel's identifier. Combined with
	// the enum's type name yields `ErrUnknownStatus`.
	// Defaults to [GoDefaultSentinelPrefix].
	SentinelPrefix string `eidos:"sentinel_prefix,default=ErrUnknown"`
}

// Plugin is the enum generator. Zero value is unusable; go
// through [New] so the embedded [sdk.Holder] binds to the
// options field.
type Plugin struct {
	*sdk.Base
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance with the options
// holder bound. The pipeline overlays caller-supplied
// option values via [Plugin.SetOptions] (promoted from
// [sdk.Holder]) at Build time.
//
// The foundation bucket runs before the composition and
// cross-cutting buckets, so a downstream plugin discovering
// the emitted API can read it during its own Generate pass.
//
// [Capability] is published so a consumer can declare a
// documentary dependency on enum-API generation through its
// own Requires list. Nothing is required in return: the
// plugin reads source nodes only and has no upstream plugin
// dependency.
//
// No template helper is registered. The templates reach only
// the canonical `renderExpr` / `external` entries and the
// shared Go-convention bundle, both of which the base already
// carries — so there is nothing here for a `.tmpl` to call
// under this plugin's name prefix.
func New() *Plugin {
	p := &Plugin{Base: sdk.NewPlugin(Name).
		For(goSupport()).
		Version(Version).
		Priority(sdk.GeneratorFoundation).
		Provides(Capability).
		Directives(directives()...).
		Build()}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// directives declares the `+gen:enum` schema on source enum
// types.
func directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(sdk.NodeKindEnum).
			Describe(
				"Opts the host enum type in for String / Parse / " +
					"MarshalJSON / UnmarshalJSON generation.",
			).
			Build(),
	}
}

// Variant is one rendered enum variant — the source-side
// const identifier plus the resolved string form the
// rendered API maps it to (via `String` / `Parse` / JSON).
type Variant struct {
	// ConstName is the source-side const identifier (e.g.
	// `StatusActive`).
	ConstName string

	// StringValue is the rendered string form — either the
	// auto-derived name (Name minus the enum's type prefix
	// when [Options.StripPrefix] is true) or the
	// `+gen:value` override.
	StringValue string

	// Ref is the [sdk.NewExternal] expression that renders
	// as the fully-qualified constant reference from the
	// external test package (e.g. `blog.StatusActive`).
	// Populated only on variants destined for the test
	// output; the primary output renders constants
	// unqualified because it lives in the source package.
	Ref *sdk.Expr
}

// API is the plugin-defined emit kind every production-
// output emit value reports. The matching `enum.api`
// template renders it as the full `String` / `Parse` /
// `MarshalJSON` / `UnmarshalJSON` surface for one source
// enum.
//
// The struct carries only language-neutral data. The
// per-language template reaches stdlib symbols through
// the backend's `external` funcmap entry (`{{ renderExpr
// (external "fmt" "Sprintf") }}`); the backend's
// `renderExpr` registers the matching import on the host
// file's import set automatically.
type API struct {
	sdk.BaseEmit

	// TypeName is the source enum's type identifier
	// (`Status`).
	TypeName string

	// ParseName is the parse function's identifier
	// (`ParseStatus`).
	ParseName string

	// SentinelName is the parse-error sentinel's identifier
	// (`ErrUnknownStatus`).
	SentinelName string

	// Underlying is the enum's underlying type name as the
	// frontend recorded it (`int`, `string`, `int64`, …), or
	// empty when the source model declares none.
	//
	// The rendered API is not uniform across underlying types:
	// the `String` fallback converts the value, and a numeric
	// conversion applied to a string-valued enum produces a file
	// that does not compile. Carried as the source fact rather
	// than a pre-rendered expression or a bool so the
	// per-language template decides what a given underlying type
	// means for its output — which is where the framework already
	// puts language interpretation.
	Underlying string

	// FallbackConv is the type an out-of-set value converts
	// through before the fallback prints it.
	//
	// The answer rather than the question, which is the
	// opposite of what [API.Underlying] carries and is
	// deliberate: this is the one part of the decision a
	// template cannot take correctly. A set whose underlying
	// type is declared in another package converts through a
	// qualified reference, and a template composing that
	// conversion from a name emits a file naming a package it
	// never imported — text cannot register an import. Carried
	// as an [sdk.Ref] so the backend's `renderType` registers
	// one.
	FallbackConv sdk.Ref

	// FallbackVerb is the printf verb that renders a value of
	// [API.FallbackConv] faithfully.
	//
	// Paired with FallbackConv by [sdk.EnumFallback] rather
	// than derived beside it, because the two drift: `%d`
	// against a set declared over `float64` renders
	// `%!d(float64=0.5)`, and `go vet` reports it in the
	// consuming repository, where nobody wrote it.
	//
	// A non-Go adapter reads FallbackConv and ignores this; a
	// printf verb means nothing to a language without printf.
	FallbackVerb string

	// Variants is the ordered variant list — declaration
	// order in the source enum, so iota-based numeric
	// values stay aligned with the rendered switch cases.
	Variants []Variant
}

// Kind returns [KindAPI].
func (*API) Kind() sdk.Kind { return KindAPI }

// Compile-time confirmation that *API satisfies
// [sdk.EmitNode].
var _ sdk.EmitNode = (*API)(nil)

// Tests is the plugin-defined emit kind every test-output
// emit value reports. The matching `enum.test` template
// renders it as the round-trip test suite that pins the
// production API's contract.
//
// The test file lives in the `<pkg>_test` external test
// package (the Go adapter's auto-shift for files ending in
// `_test.go`), so source-package identifiers route through
// [sdk.NewExternal] for cross-package qualification. The
// per-language template reaches stdlib symbols
// (`testing.T`, `errors.Is`, `encoding/json` helpers)
// through the backend's `external` funcmap entry, not a
// data-side ref.
type Tests struct {
	sdk.BaseEmit

	// TypeName is the source enum's type identifier (kept
	// for rendering in test-function names / log messages).
	TypeName string

	// TypeRef is the [sdk.NewExternal] expression that
	// renders as the fully-qualified type reference from
	// the external test package (e.g. `blog.Status`).
	TypeRef *sdk.Expr

	// ParseName is the parse function's identifier.
	ParseName string

	// ParseRef is the [sdk.NewExternal] expression that
	// renders as the fully-qualified parse function
	// reference (e.g. `blog.ParseStatus`).
	ParseRef *sdk.Expr

	// SentinelName is the parse-error sentinel's
	// identifier.
	SentinelName string

	// SentinelRef is the [sdk.NewExternal] expression that
	// renders as the fully-qualified sentinel reference
	// (e.g. `blog.ErrUnknownStatus`).
	SentinelRef *sdk.Expr

	// Variants is the variant list the test cases iterate.
	// Each variant's [Variant.Ref] is populated for
	// cross-package qualification.
	Variants []Variant
}

// Kind returns [KindTests].
func (*Tests) Kind() sdk.Kind { return KindTests }

// Compile-time confirmation that *Tests satisfies
// [sdk.EmitNode].
var _ sdk.EmitNode = (*Tests)(nil)

// Generate walks every source enum the reader exposes and,
// for each opted-in enum, queues one [API] contribution
// against the primary output and one [Tests] contribution
// against the test-tagged output. The Layout phase resolves
// each contribution's [sdk.EmitTarget] via the per-output
// routing pipeline; the rendered files appear alongside the
// source enum's file by default and follow project / CLI
// overrides otherwise.
//
// Source enums without `+gen:enum` are skipped silently.
// Annotated enums with no [sdk.EnumVariant] entries
// surface [ErrEnumHasNoVariants] as a positioned
// diagnostic; the run continues and the offending enum
// drops from the output set.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name)
	for e := range ctx.Reader.Enums().All() {
		if !e.HasPositiveDirective(DirectiveName) {
			continue
		}
		if len(e.Variants) == 0 {
			ctx.Diag.Errorf(e.Pos(), "%s: enum %q", ErrEnumHasNoVariants.Error(), e.QName())
			continue
		}
		underlying := golang.EnumUnderlying(e)
		fallbackConv, fallbackVerb := golang.EnumFallback(e)
		variants := p.collectVariants(e, underlying)
		parseName := p.parsePrefix() + e.Name
		sentinelName := p.sentinelPrefix() + e.Name

		api := &API{
			BaseEmit: sdk.BaseEmit{
				OriginNode: e,
				SetByName:  c.SetBy(),
				SourcePos:  e.Pos(),
			},
			TypeName:     e.Name,
			ParseName:    parseName,
			SentinelName: sentinelName,
			Underlying:   underlying,
			FallbackConv: fallbackConv,
			FallbackVerb: fallbackVerb,
			Variants:     variants,
		}
		if err := ctx.Store.Emit().AppendOriginSlot(e, SlotName, api, c.Provenance("enum.api."+e.Name)); err != nil {
			return fmt.Errorf("%s: append api slot: %w", Name, err)
		}

		testVariants := make([]Variant, len(variants))
		copy(testVariants, variants)
		for i := range testVariants {
			testVariants[i].Ref = sdk.NewExternal(e.Package, testVariants[i].ConstName)
		}
		tests := &Tests{
			BaseEmit: sdk.BaseEmit{
				OriginNode:    e,
				SetByName:     c.SetBy(),
				SourcePos:     e.Pos(),
				OutputTagName: GoTestOutputTag,
			},
			TypeName:     e.Name,
			TypeRef:      sdk.NewExternal(e.Package, e.Name),
			ParseName:    parseName,
			ParseRef:     sdk.NewExternal(e.Package, parseName),
			SentinelName: sentinelName,
			SentinelRef:  sdk.NewExternal(e.Package, sentinelName),
			Variants:     testVariants,
		}
		if err := ctx.Store.Emit().AppendOriginSlot(e, SlotName, tests, c.Provenance("enum.test."+e.Name)); err != nil {
			return fmt.Errorf("%s: append test slot: %w", Name, err)
		}
	}
	return nil
}

// collectVariants returns the variant list for e with each
// variant's StringValue resolved through the three-layer rule
// documented on [Plugin.resolveStringValue].
func (p *Plugin) collectVariants(e *sdk.Enum, underlying string) []Variant {
	out := make([]Variant, 0, len(e.Variants))
	for _, v := range e.Variants {
		out = append(out, Variant{
			ConstName:   v.Name,
			StringValue: p.resolveStringValue(e.Name, underlying, v),
		})
	}
	return out
}

// resolveStringValue applies the three-layer rule, highest
// precedence first:
//
//  1. A per-variant `+gen:value <override>` wins outright — it is
//     the author saying explicitly what the textual form is.
//  2. For a string-valued enum, the declared constant value.
//  3. Otherwise the variant's Name, with the enum's typeName prefix
//     stripped when [Options.StripPrefix] is true.
//
// Layer 2 exists because for a string enum the textual form is
// already written down. Deriving a different one from the
// identifier loses the only thing the declaration said: a `Region`
// declared `US Region = "us-east"` rendered its textual form as
// `"US"`, so a value read from JSON, a database column or an HTTP
// parameter did not parse, and one written through MarshalJSON
// emitted `"US"` rather than the declared value.
//
// It is gated on the underlying type rather than on Value being
// present. Every variant has a Value; for a numeric enum it is `1`,
// and rendering `String()` as `"1"` would be worse than the
// identifier. For a numeric enum the identifier is the only sensible
// textual form.
func (p *Plugin) resolveStringValue(typeName, underlying string, v *sdk.EnumVariant) string {
	if override := v.Directive(sdk.ValueDirective); override != nil && len(override.Args) > 0 {
		return override.Args[0]
	}
	if declared, ok := declaredStringValue(underlying, v); ok {
		return declared
	}
	if p.stripPrefix() && strings.HasPrefix(v.Name, typeName) {
		return strings.TrimPrefix(v.Name, typeName)
	}
	return v.Name
}

// declaredStringValue returns the unquoted constant value of a
// string-valued enum variant, reporting false for every other case.
//
// [sdk.EnumVariant.Value] holds the verbatim source form — go/types'
// ExactString — so a string constant arrives quoted (`"us-east"`,
// eight characters) while an integer arrives bare (`1`). Using the
// value unquoted would render `return "\"us-east\""`, which compiles
// and is wrong, so an unquote failure falls back to the identifier
// rather than emitting a literal nobody wrote.
func declaredStringValue(underlying string, v *sdk.EnumVariant) (string, bool) {
	if underlying != "string" || v.Value == "" {
		return "", false
	}
	unquoted, err := strconv.Unquote(v.Value)
	if err != nil {
		return "", false
	}
	return unquoted, true
}

// stripPrefix returns the configured StripPrefix value, or
// the documented default (true) when the option binder has
// not yet applied a caller-supplied value.
func (p *Plugin) stripPrefix() bool {
	return p.opts.StripPrefix
}

// parsePrefix returns the configured ParsePrefix value or
// the documented default.
func (p *Plugin) parsePrefix() string {
	if p.opts.ParsePrefix != "" {
		return p.opts.ParsePrefix
	}
	return GoDefaultParsePrefix
}

// sentinelPrefix returns the configured SentinelPrefix
// value or the documented default.
func (p *Plugin) sentinelPrefix() string {
	if p.opts.SentinelPrefix != "" {
		return p.opts.SentinelPrefix
	}
	return GoDefaultSentinelPrefix
}
