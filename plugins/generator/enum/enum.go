// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package enum generates an enumerated type's textual surface and the
// checks that hold it to what the declaration says.
//
// An enumeration is often a convention rather than a language
// feature: a defined type and a block of typed constants. Nothing
// stops a conversion admitting a value outside the set, nothing
// notices when a variant is added without the handling it needs, and
// nothing relates the type's textual form to the values it was
// declared with. Each is a one-line mistake that compiles.
//
// # Language-neutral core
//
// This file names no language. Every question about the declaration
// is asked through [sdk.EnumRules], which answers what each variant
// renders as, which of them is the zero, what lies outside the set,
// and what the set as a whole forbids. The identifiers the surface is
// declared under are composed through [sdk.SourceRules.TypeName] from
// words the language declares — see [sdk.LanguageSupport.Words] — and
// the signatures around them are spelled in the templates, which are
// per-language by construction.
//
// See the package README for what is generated, how the textual form
// is decided, what the checks assert, and the limits.
package enum

import (
	"fmt"

	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "enum"

// Version composes into the pipeline's plugin fingerprint, which
// frontends fold into their cache keys — so bumping it invalidates a
// warm cache populated when this plugin emitted something else.
const Version = "2.0.0"

// Capability is the label the plugin advertises so a downstream
// consumer can declare a documentary dependency on enum generation.
const Capability = "enum"

// DirectiveName is the bare directive name, without the `+gen:`
// prefix, that opts a declaration in.
const DirectiveName sdk.DirectiveName = "enum"

// MethodsKey suppresses the surface entirely, leaving the checks:
//
//	//+gen:enum methods=off
//
// For a type whose surface is already written by hand and only wants
// pinning. A single member that already exists is skipped without
// this — the key is for declaring the intent up front rather than
// discovering it one member at a time.
const MethodsKey = "methods"

// MethodsOff is the only value [MethodsKey] accepts.
const MethodsOff = "off"

// The surface this plugin generates, named by what each part does
// rather than by what a language calls it.
//
// Keys, not identifiers. `String` and `MarshalText` are Go's
// spellings; another language renders and encodes under names of its
// own, and a constant here holding one would write that language's
// answer into every other language's output. What each key is spelled
// as comes from [sdk.LanguageSupport.Words], joined to the type name
// by [sdk.SourceRules.TypeName].
const (
	// SurfaceRender turns a value into its textual form.
	SurfaceRender = "render"

	// SurfaceParse turns text back into a value, and is the half that
	// makes the round trip a law rather than a coincidence.
	SurfaceParse = "parse"

	// SurfaceValues returns the declared set, so a caller can iterate
	// it without restating it.
	SurfaceValues = "values"

	// SurfaceValid reports whether a value is one the declaration
	// admits — the only thing standing between a conversion and the
	// rest of the program.
	SurfaceValid = "valid"

	// SurfaceEncode and SurfaceDecode are the encoding pair, which
	// travel together: a type that encodes as text and does not decode
	// from text leaves the program one way and comes back another.
	SurfaceEncode = "encode"
	SurfaceDecode = "decode"
)

// The keys this plugin's per-language vocabulary is declared under.
//
// One per surface member, plus the two the refusal's identifier is
// composed from. How the words join the type name is the language's
// too, through [sdk.SourceRules.TypeName].
const (
	// WordRender, WordParse, WordValues, WordValid, WordEncode and
	// WordDecode name the six surface members.
	WordRender = SurfaceRender
	WordParse  = SurfaceParse
	WordValues = SurfaceValues
	WordValid  = SurfaceValid
	WordEncode = SurfaceEncode
	WordDecode = SurfaceDecode

	// WordUnknown is the subject the parse refusal is named for —
	// `Unknown` giving `ErrUnknownStatus` in a language that prefixes.
	WordUnknown = "unknown"
)

// TestOutputTag is the tag the check output advertises.
//
// A tag names an output within the plugin's own namespace, which is
// what a routing override and a per-output directive address. Neutral
// by nature: it is this plugin's word for the second file, not a
// spelling any language decides.
const TestOutputTag = "test"

// FileSlot is the [sdk.EmitFile] slot both outputs land in. `top`
// renders between the package clause and the first core declaration,
// which is where a block of whole declarations belongs.
const FileSlot = "top"

// The slots this plugin hands out.
//
// A slot is reached by name, so the generator handing the region out
// and the one filling it both spell it — from here, because a
// misspelling on either side mints a second, unconstrained region
// under a near-miss name rather than failing.
const (
	// SlotSurface is the API file's declaration block, after the
	// members this plugin derives.
	//
	// For a contributor adding a member this plugin does not model — a
	// database codec pair, a flag binding, a schema description. Each
	// of those is derived from the same declared set, and without the
	// slot a second generator emits a whole file beside this one and
	// re-derives the set to fill it.
	SlotSurface = "surface"

	// SlotChecks is the check file's function block, after the checks
	// this plugin derives. For an assertion this plugin cannot see: a
	// wire format the set has to stay compatible with, or a mapping
	// onto a neighbouring enumeration.
	SlotChecks = "checks"
)

// KindAPI and KindTests are the plugin-defined emit kinds. The backend
// resolves a template by the kind's string value, so each constant
// doubles as the name its template defines.
const (
	KindAPI   sdk.Kind = "enum.api"
	KindTests sdk.Kind = "enum.test"
)

// MetaParse and MetaSentinel name the parse function and the refusal
// this plugin generated, stamped on the declaration itself.
//
// The coupling a second generator needs. One constructing a value
// from text otherwise re-derives the naming convention from a
// literal, and a run that configured [Options.ParseWord] leaves it
// naming a function nothing declares. Reading the stamp costs one
// lookup and cannot drift.
//
//nolint:gochecknoglobals // meta key registration, immutable after init.
var (
	MetaParse    = sdk.EnsureKey(Name+".parse", sdk.StringParser)
	MetaSentinel = sdk.EnsureKey(Name+".sentinel", sdk.StringParser)
)

// Options carries the plugin's user-tunable settings.
type Options struct {
	// ParseWord overrides the word the parse function's identifier
	// carries beside the type it parses.
	//
	// Unset takes the language's own word. Set, it applies to every
	// language, which is what a caller asking for `Decode` means: the
	// word is theirs, and only the joining stays the language's.
	ParseWord string `eidos:"parse-word"`

	// SentinelWord overrides the subject the parse refusal is named
	// for — `Unrecognised` rather than `Unknown`.
	SentinelWord string `eidos:"sentinel-word"`

	// NoTests suppresses the check file, leaving the surface alone.
	//
	// For a repository whose generated tests are written by something
	// else. Off by default: a generated surface nothing exercises is a
	// round trip asserted by nobody, and the checks are the cheapest
	// place to notice a set that renders two variants alike.
	NoTests bool `eidos:"no-tests"`
}

// Plugin is the enum generator. Zero value is unusable; go through
// [New] so the embedded [sdk.Holder] binds to the options field.
type Plugin struct {
	*sdk.Base
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance.
//
// The foundation bucket runs ahead of composition and cross-cutting,
// so a plugin walking the post-generation emit graph finds the
// surfaces this pass queued.
//
// [Capability] is published so a consumer can declare a documentary
// dependency on enum generation. Nothing is required in return: the
// plugin reads source declarations and depends on no other plugin
// having run.
func New() *Plugin {
	p := &Plugin{Base: sdk.NewPlugin(Name).
		Version(Version).
		Priority(sdk.GeneratorFoundation).
		Provides(Capability).
		Directives(directives()...).
		For(goSupport()).
		Build()}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// directives declares the `+gen:enum` schema.
func directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			Describe(
				"Generates an enumeration's textual and validity surface — a " +
					"renderer, a parser and its refusal, an encoding pair, the " +
					"declared set and a validity test — plus checks over what the " +
					"declaration says. A member the type already declares is never " +
					"generated; `methods=off` suppresses all of them and leaves the " +
					"checks. The negated form is rejected — removing the directive " +
					"is the suppression.",
			).
			AllowedKeys(MethodsKey).
			On(sdk.NodeKindEnum).
			DenyNegation().
			Build(),
	}
}

// Variant is one declared constant, projected.
type Variant struct {
	// Name is the identifier.
	Name string

	// Ref qualifies it, so a check routed into another package names
	// it rather than relying on being in scope.
	Ref *sdk.Expr

	// Text is the variant's textual form as a literal, ready to
	// render — quoted by the language rather than in the template, so
	// the quoting rule lives beside the derivation that decides it.
	Text string
}

// Names are the identifiers this run's surface is declared under.
//
// Composed once and carried, rather than recomposed wherever one is
// needed. The plugin derives each from a word and a type name, and a
// second derivation at a call site is the copy that stops agreeing
// the day a word is configured.
type Names struct {
	// Render, Parse, Values, Valid, Encode and Decode are the six
	// surface members' identifiers, keyed in the same order the
	// surface constants declare them.
	Render, Parse, Values, Valid, Encode, Decode string

	// Sentinel is the parse refusal's identifier.
	Sentinel string
}

// byKey returns the identifier a surface key is spelled as.
func (n Names) byKey(key string) string {
	switch key {
	case SurfaceRender:
		return n.Render
	case SurfaceParse:
		return n.Parse
	case SurfaceValues:
		return n.Values
	case SurfaceValid:
		return n.Valid
	case SurfaceEncode:
		return n.Encode
	case SurfaceDecode:
		return n.Decode
	default:
		return ""
	}
}

// API is the emit value rendered into the primary output.
type API struct {
	sdk.BaseEmit
	Names

	// TypeName is the enumeration's own identifier, and TypeRef
	// qualifies it. A surface declaring members on the type can only
	// be written in the type's own package, so the two agree — but the
	// reference is what registers an import where a language needs one.
	TypeName string
	TypeRef  *sdk.Expr

	// PackageName is the declaring package's identifier, which
	// prefixes the refusal's message.
	//
	// The package rather than the type: a message is read in a log
	// beside messages from everywhere else, and what a reader needs
	// first is which package raised it. The type name goes in the
	// body, where it distinguishes this refusal from its neighbours.
	PackageName string

	// Fallback is the type an undeclared value converts through before
	// anything prints it, and Format is the token that prints the
	// result faithfully. Derived together — see [sdk.EnumInfo].
	Fallback sdk.Ref
	Format   string

	// Form is where the variants' textual forms come from.
	Form sdk.EnumForm

	Variants []Variant

	// generate lists the surface keys this run emits — every one the
	// type does not already declare, or none when the directive said
	// so.
	generate map[string]bool

	surface *sdk.Slot
}

// Emits reports whether the named surface key is this run's to write.
func (a *API) Emits(key string) bool { return a.generate[key] }

// Any reports whether anything at all is generated, which decides
// whether the primary file is worth emitting.
func (a *API) Any() bool { return len(a.generate) > 0 }

// Textual reports that the variants' text comes from their declared
// values rather than from their identifiers.
//
// A method rather than a comparison in the template, because
// [sdk.EnumForm] is a named string type and text/template's `eq`
// compares dynamic types before values — `eq .Form "value"` is false
// for every enumeration, silently.
func (a *API) Textual() bool { return a.Form == sdk.EnumFormValue }

// Surface returns the slot rendered after this plugin's own
// declarations.
func (a *API) Surface() *sdk.Slot {
	if a.surface == nil {
		a.surface = sdk.NewSlot(SlotSurface, "")
		a.surface.Owner = a
	}
	return a.surface
}

// Slot satisfies [sdk.SlotHost] so the backend's `slot` helper reaches
// the region by name. An unknown name yields an empty slot rather than
// nil, so a template asking for one this kind does not have renders
// nothing instead of failing.
func (a *API) Slot(name string) *sdk.Slot {
	if name == SlotSurface {
		return a.Surface()
	}
	return sdk.NewSlot(name, "")
}

// Kind returns [KindAPI].
func (*API) Kind() sdk.Kind { return KindAPI }

var (
	_ sdk.EmitNode = (*API)(nil)
	_ sdk.SlotHost = (*API)(nil)
)

// Tests is the emit value rendered into the tagged check output.
//
// The checks land in the external test package of wherever the
// surface was routed, so nothing in the declaring package is
// reachable unqualified.
type Tests struct {
	sdk.BaseEmit
	Names

	TypeName string
	TypeRef  *sdk.Expr

	// ParseRef, ValuesRef and SentinelRef qualify the surface these
	// checks drive.
	ParseRef, ValuesRef, SentinelRef *sdk.Expr

	Form     sdk.EnumForm
	Variants []Variant

	// ZeroName is the variant whose value is the type's zero, empty
	// when none is, and ZeroRef is that same variant as a reference.
	// The two cases read as opposite assertions, and which one an
	// enumeration earns is what a check exists to tell apart.
	//
	// The reference is carried rather than the check rebuilding the
	// variant by position. Declaration order and zero-ness are
	// different questions and agree only for a set declaring its zero
	// first — so `US Region = "us-east"; Unset Region = ""` asserted
	// that the zero equalled US, and failed naming a variant the
	// assertion did not mention.
	ZeroName string
	ZeroRef  *sdk.Expr

	// UnknownText is the literal a parse-refusal probe submits, empty
	// when the declared set already contains the marker.
	UnknownText string

	// OutOfRange is a value past the declared set as a literal, empty
	// when none could be derived. Used to check that an undeclared
	// value does not render as a declared one.
	OutOfRange string

	// Each reports whether the surface a check drives actually exists.
	//
	// Parses and Enumerates track what this run generated rather than
	// what the type has: both are declared beside the type rather than
	// on it, so one the author wrote is invisible to the declaration,
	// and a check assuming it would name something that may not be
	// there. The rest are members, so a hand-written one is visible
	// and counts.
	Renders, Parses, Marshals, Encodes, Validates, Enumerates bool

	checks *sdk.Slot
}

// Textual reports that the variants' text comes from their declared
// values — see [API.Textual] for why this is a method.
func (t *Tests) Textual() bool { return t.Form == sdk.EnumFormValue }

// Count returns how many variants the declaration carries, which is
// the arity a check pins.
func (t *Tests) Count() int { return len(t.Variants) }

// Checks returns the slot rendered after this plugin's own checks.
func (t *Tests) Checks() *sdk.Slot {
	if t.checks == nil {
		t.checks = sdk.NewSlot(SlotChecks, "")
		t.checks.Owner = t
	}
	return t.checks
}

// Slot satisfies [sdk.SlotHost] so the backend's `slot` helper reaches
// the region by name.
func (t *Tests) Slot(name string) *sdk.Slot {
	if name == SlotChecks {
		return t.Checks()
	}
	return sdk.NewSlot(name, "")
}

// Kind returns [KindTests].
func (*Tests) Kind() sdk.Kind { return KindTests }

var (
	_ sdk.EmitNode = (*Tests)(nil)
	_ sdk.SlotHost = (*Tests)(nil)
)

// Generate projects every annotated declaration into a surface and the
// checks over it.
//
// Per package, because the language a declaration is read with is a
// fact about the package that produced it — see [sdk.LanguageOf].
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name)
	constants := ctx.Reader.Constants().Slice()
	unread := map[string]bool{}
	for _, pkg := range ctx.Reader.Packages().Slice() {
		rules, lang, ok := p.SourceOf(pkg)
		if !ok {
			p.report(ctx, pkg, sdk.LanguageOf(pkg), unread,
				"are not read, so no enum surface is generated for them")
			continue
		}
		er, ok := rules.(sdk.EnumRules)
		if !ok {
			p.report(ctx, pkg, lang, unread,
				"describe no enumerations, so no enum surface is generated for them")
			continue
		}
		if err := p.generatePackage(ctx, c, pkg, rules, er, lang, constants); err != nil {
			return err
		}
	}
	return nil
}

// generatePackage queues the outputs for every annotated declaration
// in one package.
func (p *Plugin) generatePackage(
	ctx *sdk.GeneratorContext, c *sdk.Provenance, pkg *sdk.Package,
	rules sdk.SourceRules, er sdk.EnumRules, lang string, constants []*sdk.Constant,
) error {
	for _, e := range pkg.Enums {
		if !e.HasPositiveDirective(DirectiveName) {
			continue
		}
		info := er.EnumOf(e, constants)
		if !usable(ctx, e, info) {
			continue
		}
		names := p.namesOf(rules, lang, e.Name)
		api := apiOf(ctx, c, e, info, names, pkg.Name)
		MetaParse.Set(e.EnsureMeta(), names.Parse, Name)
		MetaSentinel.Set(e.EnsureMeta(), names.Sentinel, Name)

		if err := sdk.QueueEmit(
			ctx.Store.Emit(), c, FileSlot, e, p.queued(e, api, info)...,
		); err != nil {
			// Wrapped even though the queue names the plugin and the
			// slot: what it cannot name is which declaration the run was
			// on, which is the only part a reader needs to find the line.
			return fmt.Errorf("%s: queue %q: %w", Name, e.Name, err)
		}
	}
	return nil
}

// usable reports whether the declaration is one this plugin can
// generate a truthful surface for, reporting why when it is not.
//
// Each refusal is a case where generating anyway produces answers
// that are confidently false rather than merely incomplete, which is
// the distinction: an absent probe drops one check, and a set the
// projection cannot see makes every check about that set a lie.
func usable(ctx *sdk.GeneratorContext, e *sdk.Enum, info sdk.EnumInfo) bool {
	switch {
	case len(e.Variants) == 0:
		ctx.Diag.Errorf(e.Pos(),
			"%s: %q carries +gen:%s but declares no variant",
			Name, e.QName(), DirectiveName)
		return false
	case len(info.Foreign) > 0:
		// Legal, and silently wrong. Variants declared elsewhere never
		// reach the set, so a validity test would reject a value
		// someone declared and an arity check would pin a count that is
		// not the truth. Reported rather than absorbed: reaching across
		// a package boundary would make the generated set depend on
		// which packages a run happened to include.
		ctx.Diag.Errorf(e.Pos(),
			"%s: %q has variants declared in %v; move them beside the type "+
				"or the generated set will exclude them",
			Name, e.QName(), info.Foreign)
		return false
	case info.Duplicate != "":
		ctx.Diag.Errorf(e.Pos(),
			"%s: %q renders two variants as %s; pin one with a value override",
			Name, e.QName(), info.Duplicate)
		return false
	default:
		return true
	}
}

// apiOf projects one declaration into the primary output's value.
func apiOf(
	ctx *sdk.GeneratorContext, c *sdk.Provenance,
	e *sdk.Enum, info sdk.EnumInfo, names Names, pkgName string,
) *API {
	return &API{
		BaseEmit:    sdk.EmitBase(c, e),
		Names:       names,
		TypeName:    e.Name,
		TypeRef:     sdk.NewExternal(e.Package, e.Name),
		PackageName: pkgName,
		Fallback:    info.Fallback,
		Format:      info.FallbackFormat,
		Form:        info.Form,
		Variants:    variantsOf(e.Package, info),
		generate:    generated(ctx, e, names),
	}
}

// queued returns the emit values one declaration contributes.
//
// The surface is skipped when nothing is left to generate — a type
// declaring every member already, or one that asked for none. A file
// carrying only a generated-by header reads as a generator that
// failed.
func (p *Plugin) queued(e *sdk.Enum, api *API, info sdk.EnumInfo) []sdk.EmitNode {
	var out []sdk.EmitNode
	if api.Any() {
		out = append(out, api)
	}
	if p.opts.NoTests {
		return out
	}
	return append(out, testsOf(e, api, info))
}

// testsOf projects the checks over what apiOf decided to emit.
func testsOf(e *sdk.Enum, api *API, info sdk.EnumInfo) *Tests {
	declares := func(key string) bool { return e.MethodByName(api.byKey(key)) != nil }
	return &Tests{
		BaseEmit:    sdk.EmitBaseTagged(api.BaseEmit, TestOutputTag),
		Names:       api.Names,
		TypeName:    api.TypeName,
		TypeRef:     api.TypeRef,
		ParseRef:    sdk.NewExternal(e.Package, api.Parse),
		ValuesRef:   sdk.NewExternal(e.Package, api.Values),
		SentinelRef: sdk.NewExternal(e.Package, api.Sentinel),
		Form:        api.Form,
		Variants:    api.Variants,
		ZeroName:    info.Zero,
		ZeroRef:     zeroRef(e.Package, info.Zero),
		UnknownText: info.UnknownText,
		OutOfRange:  info.OutOfRange,
		Renders:     api.Emits(SurfaceRender) || declares(SurfaceRender),
		Parses:      api.Emits(SurfaceParse),
		Marshals:    api.Emits(SurfaceEncode) && api.Emits(SurfaceDecode),
		Encodes:     api.Emits(SurfaceEncode) || declares(SurfaceEncode),
		Validates:   api.Emits(SurfaceValid) || declares(SurfaceValid),
		Enumerates:  api.Emits(SurfaceValues),
	}
}

// zeroRef qualifies the zero variant, or nil when the set has none.
func zeroRef(pkg, name string) *sdk.Expr {
	if name == "" {
		return nil
	}
	return sdk.NewExternal(pkg, name)
}

// variantsOf lifts the projected variants, qualifying each.
func variantsOf(pkg string, info sdk.EnumInfo) []Variant {
	out := make([]Variant, 0, len(info.Variants))
	for _, v := range info.Variants {
		out = append(out, Variant{
			Name: v.Name,
			Ref:  sdk.NewExternal(pkg, v.Name),
			Text: v.Text,
		})
	}
	return out
}

// namesOf composes the identifiers this run's surface is declared
// under.
//
// The part order is this plugin's — a parser is named for what it does
// and then for what it parses, a set accessor for what it holds and
// then for what it returns — and the spelling is the language's. A
// core concatenating the parts itself would write one language's
// casing and word order into every other language's output.
func (p *Plugin) namesOf(rules sdk.SourceRules, lang, typeName string) Names {
	word := func(key, override string) string {
		if override != "" {
			return override
		}
		return p.Word(lang, key)
	}
	return Names{
		Render: rules.TypeName(word(WordRender, "")),
		Parse:  rules.TypeName(word(WordParse, p.opts.ParseWord), typeName),
		Values: rules.TypeName(typeName, word(WordValues, "")),
		Valid:  rules.TypeName(word(WordValid, "")),
		Encode: rules.TypeName(word(WordEncode, "")),
		Decode: rules.TypeName(word(WordDecode, "")),
		Sentinel: sentinelName(rules,
			rules.TypeName(word(WordUnknown, p.opts.SentinelWord), typeName)),
	}
}

// sentinelName spells the refusal's identifier through the language's
// own error convention where it declares one.
//
// Falling back to the composed subject rather than to a prefix of this
// plugin's choosing: `Err` is Go's convention and a core carrying it
// would name every language's refusal the Go way. A language
// describing no error protocol gets `UnknownStatus`, which reads as
// what it is.
func sentinelName(rules sdk.SourceRules, subject string) string {
	if er, ok := rules.(sdk.ErrorRules); ok {
		return er.SentinelName(subject)
	}
	return subject
}

// generated returns the surface keys this run writes: every one the
// type does not already declare, unless the directive suppressed all
// of them.
//
// Skipping silently rather than reporting a clash. An author who wrote
// their own renderer meant to keep it, and a generator that refused to
// run until they deleted it would be demanding they give up the more
// specific statement.
func generated(
	ctx *sdk.GeneratorContext, e *sdk.Enum, names Names,
) map[string]bool {
	out := map[string]bool{}
	if suppressed(e) {
		return out
	}
	declares := func(key string) bool { return e.MethodByName(names.byKey(key)) != nil }
	for _, key := range []string{SurfaceRender, SurfaceEncode, SurfaceValid} {
		if !declares(key) {
			out[key] = true
		}
	}
	// The parser and the set accessor are declared beside the type
	// rather than on it, so a same-named declaration is not something
	// the enumeration can see. They ride with the renderer: a type
	// keeping its own renderer almost always keeps its own parser, and
	// generating one that shadows theirs is the worse guess.
	if out[SurfaceRender] {
		out[SurfaceParse] = true
		out[SurfaceValues] = true
	}
	if declares(SurfaceDecode) {
		return out
	}
	// The decoder is written in terms of a parser, so it needs one to
	// exist: the generated one where this run writes it, and the
	// author's under the same derived name where they wrote it
	// themselves. With neither, the file would name a function nothing
	// declares — and the encoder goes too rather than shipping half a
	// pair, since a type that encodes as text and decodes from
	// something else is what no author asks for.
	switch {
	case out[SurfaceParse], declaresParse(ctx, e, names.Parse):
		out[SurfaceDecode] = true
	case out[SurfaceEncode]:
		delete(out, SurfaceEncode)
	}
	return out
}

// declaresParse reports that the declaring package already has the
// parse function under the name this run derives.
//
// Asked of the run rather than of the declaration, because the
// function sits beside the type rather than on it. The old rule
// guessed from the renderer instead, which gave a type keeping its own
// renderer — and therefore its own parser — the encoder alone.
func declaresParse(ctx *sdk.GeneratorContext, e *sdk.Enum, name string) bool {
	_, found := ctx.Reader.Functions().Where(func(fn *sdk.Function) bool {
		return fn.Name == name && fn.Package == e.Package
	}).First()
	return found
}

// suppressed reports whether the directive asked for no surface at
// all.
//
// The last declaration wins, matching every other per-declaration key
// in this repository. [sdk.Node.Directive] is first-wins and answers a
// different question — whether the directive is there at all.
func suppressed(e *sdk.Enum) bool {
	dir := sdk.Last(e.Directives(), DirectiveName)
	return dir != nil && dir.KV[MethodsKey] == MethodsOff
}

// report warns once per language this plugin cannot generate for.
//
// An unmarked package is passed over quietly: the marker names the
// language a package was written in, so its absence means nothing
// claimed it — a fixture, a bridge, a synthesised graph. Warning about
// those would put a diagnostic on every unit test that builds a store
// by hand, which is where the real warning would then go unread.
func (p *Plugin) report(
	ctx *sdk.GeneratorContext, pkg *sdk.Package, lang string,
	seen map[string]bool, because string,
) {
	if lang == "" || seen[lang] {
		return
	}
	seen[lang] = true
	ctx.Diag.Warnf(pkg.Pos(),
		"%s: declarations written in %q %s; this plugin reads: %v",
		Name, lang, because, p.Languages())
}
