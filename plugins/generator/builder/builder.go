// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package builder generates a fluent builder for an annotated type,
// plus a companion file of checks over it.
//
// A composite literal restates every member at every call site, so a
// member added to the type breaks every literal at once and a reader
// cannot tell which members a given call actually cares about. A
// builder inverts that: the constructor supplies the rest, and each
// call states only what it varies.
//
// # Language-neutral core
//
// This file names no language. Which setters a member owes follows
// the shape of its type — sequence, mapping, set, optional — in the
// vocabulary [sdk.TypeShape] defines, and every question behind that
// projection is asked through [sdk.SourceRules]: what shape a type
// has, what a value of it looks like, which members a constructor can
// set, how an identifier is spelled.
//
// The identifiers are composed here and the words are not. `With`,
// `Append`, `Entry` and the PascalCase joining them are Go's
// conventions, declared through [sdk.LanguageSupport.Words] and joined
// by [sdk.SourceRules.TypeName], so this file picks which parts an
// identifier carries and in what order and never what they read as.
//
// Composed in the templates instead, they could not be checked: two
// members can reach one setter, and a template writing both emits a
// duplicate method that is reported against the consumer's build
// rather than the declaration that caused it.
//
// See the package README for what each shape owes, how seeding
// composes, what the generated checks assert, and the limits.
package builder

import (
	"fmt"
	"slices"
	"strings"

	"go.thesmos.sh/eidos/plugins/annotator/defaults"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "builder"

// Version composes into the pipeline's plugin fingerprint, which
// frontends fold into their cache keys — so bumping it invalidates a
// warm cache populated when this plugin emitted something else.
const Version = "2.0.0"

// Capability is the label the plugin advertises so a downstream
// consumer can declare a documentary dependency on builder generation.
const Capability = "builder"

// DirectiveName is the bare directive name, without the `+gen:`
// prefix, that opts a declaration in.
const DirectiveName sdk.DirectiveName = "builder"

// CompanionKey names the seeding function explicitly, for one that
// does not follow the convention or does not live beside the type:
//
//	//+gen:builder defaults=example.com/seed.UserDefaults
//
// The full-path notation matters: a companion in another package would
// otherwise need an import written only for this directive, which does
// not compile.
const CompanionKey = "defaults"

// The keys this plugin's per-language vocabulary is declared under —
// see [sdk.LanguageSupport.Words] and the language bindings beside
// this file.
//
// Keys rather than values: what word a language uses is that
// language's business, and a constant here holding `"Builder"` would
// be one language's idiom spelled into every other language's output.
// How the word joins the type name is the language's too, composed
// through [sdk.SourceRules.TypeName].
const (
	// WordBuilder names the generated type — `User` gives
	// `UserBuilder` in a language that concatenates.
	WordBuilder = "builder"

	// WordCompanion names the seeding function found beside a type.
	// A package holding several types gets one companion each, which
	// is why the identifier carries the type rather than being a bare
	// word.
	WordCompanion = "companion"

	// WordFrom names the seeding constructor beside the plain one —
	// `NewUser` and `NewUserFrom`.
	WordFrom = "from"

	// WordSet names the replacing setter every member owes —
	// `Username` gives `WithUsername`.
	WordSet = "set"

	// WordAppend names the setter that keeps what is already there,
	// which only an ordered member owes.
	WordAppend = "append"

	// WordText names the second setter a byte sequence owes, the one
	// taking the language's text type.
	WordText = "text"

	// WordEntry and WordEntries name the two setters a keyed member
	// owes: one key at a time, and several at once.
	WordEntry   = "entry"
	WordEntries = "entries"
)

// TestOutputTag is the tag the companion check output advertises.
//
// A tag names an output within the plugin's own namespace, which is
// what a routing override and a per-output directive address. Neutral
// by nature: it is this plugin's word for the second file, not a
// spelling any language decides.
const TestOutputTag = "test"

// SkipTag is the tag key excluding a member from the builder:
//
//	Internal string `builder:"-"`
//
// For a member a caller should never set but which cannot be hidden —
// something a neighbouring package reads directly. Any value other
// than [SkipValue] is rejected, so a typo is reported rather than
// silently keeping the setter.
const SkipTag = "builder"

// SkipValue is the only value [SkipTag] accepts.
const SkipValue = "-"

// FileSlot is the [sdk.EmitFile] slot both outputs land in. `top`
// renders between the package clause and the first core declaration,
// which is where a block of whole declarations belongs.
const FileSlot = "top"

// The slots this plugin hands out on the builder it emits.
//
// A slot is reached by name, so the generator handing the region out
// and the one filling it both spell it — from here, because a
// misspelling on either side mints a second, unconstrained region
// under a near-miss name rather than failing.
const (
	// SlotSetters is the builder's method block, after the setters
	// this plugin derives. For a contributor adding one the member
	// shapes do not model — a setter taking a domain type and filling
	// several members from it.
	SlotSetters = "setters"

	// SlotChecks is the check file's function block, after the checks
	// this plugin derives.
	//
	// For a contributor asserting something this plugin cannot see: a
	// validation generator checking that Build refuses what its rules
	// forbid, or a domain invariant no member shape implies. Without
	// it a second generator wanting one check has to emit a whole file
	// of its own beside this one.
	SlotChecks = "checks"

	// SlotBuild is inside Build, before the value is returned.
	//
	// Where a contribution makes the constructed value *correct*
	// rather than merely reachable: a normalisation, or a validation
	// generator's check. A setter slot alone cannot do that — it adds
	// ways to write a member and no way to constrain what was written.
	SlotBuild = "build"
)

// KindType and KindTests are the plugin-defined emit kinds. The
// backend resolves a template by the kind's string value, so each
// constant doubles as the name its template defines.
const (
	KindType  sdk.Kind = "builder.type"
	KindTests sdk.Kind = "builder.test"
)

// MetaType names the builder generated for a declaration, stamped on
// the declaration itself.
//
// The coupling a second generator needs. A fixture or double generator
// naming `UserBuilder` otherwise re-derives the suffix convention from
// a literal, and a run that configured [Options.Suffix] leaves it
// naming a type nothing declares. Reading the stamp costs one lookup
// and cannot drift.
//
//nolint:gochecknoglobals // meta key registration, immutable after init.
var MetaType = sdk.EnsureKey(Name+".type", sdk.StringParser)

// Options carries the plugin's user-tunable settings.
type Options struct {
	// Suffix overrides the word the builder's identifier carries
	// beside the type it builds.
	//
	// Unset takes the language's own word — see [WordBuilder]. Set, it
	// applies to every language, which is what a caller asking for
	// `Factory` means: the word is theirs, and only the joining stays
	// the language's.
	Suffix string `eidos:"suffix"`

	// CompanionWord overrides the word the seeding function's
	// identifier carries beside the type it seeds.
	//
	// For a repository whose seed functions are called something else
	// — `UserFixture`, `UserSeed`. Unset takes the language's own word;
	// the lookup composes it the same way the builder's name is
	// composed, so the two conventions cannot drift apart.
	CompanionWord string `eidos:"companion-word"`

	// NoTests suppresses the companion check file, leaving the builder
	// alone.
	//
	// For a repository whose generated tests are written by something
	// else. Off by default: a builder nothing exercises is a
	// constructor whose setters are asserted by nobody, and the checks
	// are the cheapest place to notice one that drops what it is
	// given.
	NoTests bool `eidos:"no-tests"`
}

// Plugin is the fluent-builder generator. Zero value is unusable; go
// through [New] so the embedded [sdk.Holder] binds to the options
// field.
type Plugin struct {
	*sdk.Base
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance.
//
// The foundation bucket runs ahead of composition and cross-cutting,
// so a plugin walking the post-generation emit graph finds the
// builders this pass queued.
//
// [Capability] is published so a consumer can declare a documentary
// dependency on builder generation. Nothing is required in return: the
// plugin reads source declarations and depends on no other plugin
// having run — including the defaults annotator, whose stamps it reads
// where they are present and does without where they are not.
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

// directives declares the `+gen:builder` schema.
//
// The directive takes no positional argument: a builder exists exactly
// where one is declared, so deleting the line is the suppression and a
// negated form would have nothing to act on.
func directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			Describe(
				"Generates a fluent builder for the annotated type, plus a " +
					"companion file checking it. Takes no positional argument. A " +
					"`<Type>Defaults()` function in the same package seeds the " +
					"constructor, `defaults=` names one explicitly, and per-member " +
					"`+gen:default` directives or `default:\"…\"` tags override it. " +
					"A member opts out with a `builder:\"-\"` tag. The negated form " +
					"is rejected — a builder exists only where declared, so " +
					"removing the directive is the suppression.",
			).
			AllowedKeys(CompanionKey).
			On(sdk.NodeKindStruct).
			DenyNegation().
			Build(),
	}
}

// Field is one member the builder can set.
//
// The projection the templates render. Every language-specific
// question behind it was asked through [sdk.SourceRules], so what
// reaches a template is the neutral answer and a second language
// changes nothing in this file.
type Field struct {
	// Name is the member identifier, which also names the setter —
	// `Username` gives `WithUsername`.
	Name string

	// Type is the member's declared type.
	Type sdk.Ref

	// Shape is what the type's structure is, in the vocabulary every
	// language shares. It decides which setters the member owes.
	Shape sdk.TypeShape

	// Elem is the inner type: a sequence's element, a mapping's value,
	// an optional's contents. Key is a mapping's or set's key. Both
	// nil for the shapes with none.
	Elem sdk.Ref
	Key  sdk.Ref

	// Default is the member's declared default as source text, empty
	// when it declared none. It renders straight into the
	// constructor's literal.
	Default string

	// DefaultRef qualifies a default naming a symbol in another
	// package, nil when the default is a plain literal. A rendered file
	// has to register the import, which only a reference carries.
	DefaultRef *sdk.Expr

	// DefaultIsZero reports that the declared default is the type's
	// zero, where no check can tell a constructor that applied it from
	// one that ignored it.
	//
	// Answered by the language during Generate, because the spellings
	// are its own: `nil` is Go's and `None` is another's.
	DefaultIsZero bool

	// Sample and Alternate are two distinct values of whatever the
	// setter takes, empty when the type admits none. Two rather than
	// one because a check comparing against a single value passes
	// whenever the subject already held it.
	Sample    sdk.Sample
	Alternate sdk.Sample

	// Set is the identifier the replacing setter is declared under,
	// and the four beside it are the extra setters a shape owes —
	// empty where it owes none.
	//
	// Derived here rather than composed in the template, though the
	// words and the joining are still the language's. Two members can
	// produce one identifier — `Data []byte` beside `DataString
	// string` both reach `WithDataString` — and a template composing
	// the names is a template that cannot see the second one coming:
	// the file is emitted, and the duplicate method is reported
	// against the consumer's build rather than against the
	// declaration that caused it.
	Set        string
	Append     string
	SetText    string
	SetEntry   string
	SetEntries string
}

// Names returns every identifier this member's setters are declared
// under, in declaration order, skipping the shapes that owe none.
func (f Field) Names() []string {
	out := make([]string, 0, 4)
	for _, name := range []string{f.Set, f.Append, f.SetText, f.SetEntry, f.SetEntries} {
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// Copies reports whether the member owns storage a clone must not
// share.
func (f Field) Copies() bool {
	switch f.Shape {
	case sdk.ShapeSequence, sdk.ShapeBytes, sdk.ShapeMapping, sdk.ShapeSet:
		return true
	default:
		return false
	}
}

// Keyed reports whether the member is addressed by key, which is the
// two shapes owing an entry setter.
func (f Field) Keyed() bool {
	return f.Shape == sdk.ShapeMapping || f.Shape == sdk.ShapeSet
}

// IsScalar reports the shape owing one plain replacing setter, and
// the five predicates below report the rest.
//
// Methods rather than a comparison in the template, because
// [sdk.TypeShape] is a named string type and text/template's `eq`
// compares dynamic types before values — `eq .Shape "sequence"` is
// false for every member, silently, and every branch falls to the
// scalar arm.
func (f Field) IsScalar() bool { return f.Shape == sdk.ShapeScalar }

// IsSequence reports an ordered run of one element type.
func (f Field) IsSequence() bool { return f.Shape == sdk.ShapeSequence }

// IsBytes reports a sequence the language can also spell as text.
func (f Field) IsBytes() bool { return f.Shape == sdk.ShapeBytes }

// IsMapping reports a keyed collection carrying a value per key.
func (f Field) IsMapping() bool { return f.Shape == sdk.ShapeMapping }

// IsSet reports a keyed collection carrying membership only.
func (f Field) IsSet() bool { return f.Shape == sdk.ShapeSet }

// IsOptional reports a value that may be absent.
func (f Field) IsOptional() bool { return f.Shape == sdk.ShapeOptional }

// Declared reports whether the member declares a default at all.
//
// Whether that default is the type's *zero* — where a check comparing
// against it asserts `0 == 0` and passes against a constructor that
// ignored the declaration — is a question about the language's
// literals, so a template asks it of the language rather than of this
// value. `nil` is Go's spelling and `None` is another's.
func (f Field) Declared() bool { return strings.TrimSpace(f.Default) != "" }

// Type is the emit value rendered into the primary output.
type Type struct {
	sdk.BaseEmit

	// TypeName is the builder's identifier — `<Source><Suffix>`.
	TypeName string

	// SourceName is the type's own identifier.
	SourceName string

	// CtorName and FromName are the two constructors' identifiers.
	//
	// Carried rather than composed in the template, because the checks
	// carry them too — and a constructor spelled once here and once
	// there is one a configured word can move on one side only, leaving
	// the checks calling a function the builder never declared.
	CtorName, FromName string

	// ValueRef qualifies the type the builder constructs. A builder
	// routed into another package cannot reach it unqualified, and
	// where the two share a package the backend renders it bare.
	ValueRef *sdk.Expr

	// TypeParams is the declaration's generic parameter list in
	// declaration form; TypeArgs is the same list in use position.
	TypeParams []*sdk.EmitTypeParam
	TypeArgs   string

	Fields []Field

	// Companion qualifies the seeding function the constructor calls,
	// nil when the package declares none.
	Companion *sdk.Expr

	setters, build *sdk.Slot
}

// Kind returns [KindType].
func (*Type) Kind() sdk.Kind { return KindType }

// Seeded reports whether any member declares a default, which decides
// whether the constructor builds a literal or an empty builder.
func (t *Type) Seeded() bool {
	return slices.ContainsFunc(t.Fields, func(f Field) bool { return f.Default != "" })
}

// Setters returns the slot rendered after this plugin's own setters.
func (t *Type) Setters() *sdk.Slot {
	if t.setters == nil {
		t.setters = sdk.NewSlot(SlotSetters, "")
		t.setters.Owner = t
	}
	return t.setters
}

// Build returns the slot rendered inside Build, before the value is
// returned.
func (t *Type) Build() *sdk.Slot {
	if t.build == nil {
		t.build = sdk.NewSlot(SlotBuild, "")
		t.build.Owner = t
	}
	return t.build
}

// Slot satisfies [sdk.SlotHost] so the backend's `slot` helper reaches
// either region by name. An unknown name yields an empty slot rather
// than nil, so a template asking for one this kind does not have
// renders nothing instead of failing.
func (t *Type) Slot(name string) *sdk.Slot {
	switch name {
	case SlotSetters:
		return t.Setters()
	case SlotBuild:
		return t.Build()
	default:
		return sdk.NewSlot(name, "")
	}
}

var (
	_ sdk.EmitNode = (*Type)(nil)
	_ sdk.SlotHost = (*Type)(nil)
)

// Tests is the emit value rendered into the tagged check output.
//
// The checks land in the external test package of wherever the builder
// was routed, so they reach neither the builder nor the type
// unqualified. The type's package is known during Generate; the
// builder's is not decided until Layout, which is why this implements
// [sdk.OutputPackageSetter].
type Tests struct {
	sdk.BaseEmit

	TypeName   string
	SourceName string

	// CtorName and FromName are the builder's two constructor
	// identifiers, spelled by the language during Generate.
	//
	// Carried rather than recomposed, because [Tests.SetOutputPackages]
	// runs after Layout and holds no language: an identifier built
	// there would be this file's guess at one language's convention,
	// which is exactly what the rest of the plugin avoids.
	CtorName, FromName string

	// CtorRef and FromRef qualify those two constructors. Set during
	// Generate against the source package as a provisional value, then
	// corrected once routing resolves — a wrong package is a compile
	// error naming the symbol, while a bare name silently binds to
	// whatever else is in scope.
	CtorRef, FromRef *sdk.Expr

	// ValueRef qualifies the type the builder constructs.
	ValueRef *sdk.Expr

	TypeParams []*sdk.EmitTypeParam

	// Witnesses are the concrete types the checks instantiate at,
	// empty for a plain declaration and for one whose constraints
	// admit none — the latter gets a note in place of its checks.
	//
	// References rather than the rendered `[string, int]` a template
	// once appended as text. An authored witness may name another
	// package, and only `renderType` registers the import the file
	// then needs — so the text form landed a qualified name in a file
	// that never imported it.
	Witnesses []sdk.Ref

	Fields []Field

	// Seeded mirrors [Type.Seeded] so the constructor's check asserts
	// what the constructor does rather than what it usually does.
	Seeded bool

	// Companion mirrors [Type.Companion]. With one and no declared
	// defaults the check compares the constructed value against the
	// companion's own return, which is exact — anything weaker would
	// pass against a constructor that called something else.
	Companion *sdk.Expr

	checks *sdk.Slot
}

// Checks returns the slot rendered after this plugin's own checks.
func (t *Tests) Checks() *sdk.Slot {
	if t.checks == nil {
		t.checks = sdk.NewSlot(SlotChecks, "")
		t.checks.Owner = t
	}
	return t.checks
}

// Slot satisfies [sdk.SlotHost] so the backend's `slot` helper reaches
// the region by name. An unknown name yields an empty slot rather than
// nil, so a template asking for one this kind does not have renders
// nothing instead of failing.
func (t *Tests) Slot(name string) *sdk.Slot {
	if name == SlotChecks {
		return t.Checks()
	}
	return sdk.NewSlot(name, "")
}

// Kind returns [KindTests].
func (*Tests) Kind() sdk.Kind { return KindTests }

// Generic reports that the declaration is parameterised, which is
// where the per-member checks are withheld.
//
// A test function cannot take type parameters, so a check naming one
// in a member position would not compile. The structural checks still
// run, instantiated at [Tests.Witnesses]; only the per-member setter
// checks, whose parameter types are the member's own, are dropped.
func (t *Tests) Generic() bool { return len(t.TypeParams) > 0 }

// Instantiable reports whether the checks can name the types they
// would run at.
//
// False for a parameterised declaration whose constraints admit no
// witness, which is the one case where no check can be written at all
// — and where the rendered file carries a note saying so rather than
// nothing.
func (t *Tests) Instantiable() bool {
	return len(t.TypeParams) == 0 || len(t.Witnesses) > 0
}

// Copies reports whether any member owns storage a clone must not
// share, which decides whether the independence check is emitted.
func (t *Tests) Copies() bool { return slices.ContainsFunc(t.Fields, Field.Copies) }

// Seedable returns the members a check can set to a named value.
//
// The seed for the round-trip checks, and the reason they assert
// anything: built with `var seed T` and compared against itself,
// `From(zero).Build() == zero` passes against a constructor that
// dropped every member it was given.
//
// Scalar shapes only, and the restriction is about what the *setter*
// accepts rather than what the sample is. A mapping's setter takes a
// mapping, a sequence's a variadic, a set's one entry at a time — so
// handing any of them a scalar sample is a type error. Each is driven
// through the shape it owns by its own per-member check.
func (t *Tests) Seedable() []Field {
	if t.Generic() && len(t.Witnesses) == 0 {
		// A parameterised declaration whose constraints admit no
		// witness seeds nothing: a check is an ordinary function, so it
		// names the declaration at concrete types, and there are none
		// to name. Withholding the seed is the honest answer; writing
		// it produces a file naming a generic type without
		// instantiation.
		//
		// Where witnesses do exist the members are sampled at them, so
		// a seed is spellable and this reads like any other
		// declaration.
		return nil
	}
	out := make([]Field, 0, len(t.Fields))
	for i := range t.Fields {
		if t.Fields[i].Shape == sdk.ShapeScalar && t.Fields[i].Sample.OK() {
			out = append(out, t.Fields[i])
		}
	}
	return out
}

// SetOutputPackages repoints the references at wherever Layout routed
// the builder.
func (t *Tests) SetOutputPackages(byTag map[string]string) {
	path, ok := sdk.PrimaryPackage(byTag)
	if !ok {
		return
	}
	t.CtorRef = sdk.NewExternal(path, t.CtorName)
	t.FromRef = sdk.NewExternal(path, t.FromName)
}

var (
	_ sdk.EmitNode            = (*Tests)(nil)
	_ sdk.SlotHost            = (*Tests)(nil)
	_ sdk.OutputPackageSetter = (*Tests)(nil)
)

// Generate projects every annotated declaration into a builder and
// the checks over it.
//
// Per package, because the language a declaration is read with is a
// fact about the package that produced it — see [sdk.LanguageOf]. A
// package written in a language this plugin cannot read is reported
// once rather than passed over, since every builder in it would
// otherwise go unemitted with nothing to say why.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name)
	unread := map[string]bool{}
	for _, pkg := range ctx.Reader.Packages().Slice() {
		rules, lang, ok := p.SourceOf(pkg)
		if !ok {
			p.reportUnread(ctx, pkg, sdk.LanguageOf(pkg), unread)
			continue
		}
		if err := p.generatePackage(ctx, c, pkg, rules, lang); err != nil {
			return err
		}
	}
	return nil
}

// generatePackage queues the outputs for every annotated declaration
// in one package.
func (p *Plugin) generatePackage(
	ctx *sdk.GeneratorContext, c *sdk.Provenance,
	pkg *sdk.Package, rules sdk.SourceRules, lang string,
) error {
	funcs := ctx.Reader.Functions().Slice()
	words := p.setterWords(lang)
	for _, s := range pkg.Structs {
		if !s.HasPositiveDirective(DirectiveName) {
			continue
		}
		fields := fieldsOf(ctx, rules, words, s)
		if len(fields) == 0 {
			ctx.Diag.Errorf(s.Pos(),
				"%s: %q carries +gen:%s but has no member a builder can set",
				Name, s.QName(), DirectiveName)
			continue
		}
		if collides(ctx, s, fields) {
			continue
		}

		ctor := rules.ConstructorName(s.Name)
		value := &Type{
			BaseEmit:   sdk.EmitBase(c, s),
			TypeName:   rules.TypeName(s.Name, p.word(lang, WordBuilder, p.opts.Suffix)),
			SourceName: s.Name,
			CtorName:   ctor,
			FromName:   rules.TypeName(ctor, p.word(lang, WordFrom, "")),
			ValueRef:   sdk.NewExternal(s.Package, s.Name),
			TypeParams: rules.TypeParams(s.TypeParams),
			TypeArgs:   rules.TypeArgs(s.TypeParams),
			Fields:     fields,
			Companion:  p.companionOf(ctx, rules, lang, s, funcs),
		}
		// Stamped before queueing, so a generator in a later bucket
		// reading the graph finds the name without re-deriving the
		// convention — and without knowing which language spelled it.
		MetaType.Set(s.EnsureMeta(), value.TypeName, Name)

		if err := sdk.QueueEmit(
			ctx.Store.Emit(), c, FileSlot, s, p.queued(rules, s, value)...,
		); err != nil {
			// Wrapped even though the queue names the plugin and the
			// slot: what it cannot name is which declaration the run was
			// on, which is the only part a reader needs to find the line.
			return fmt.Errorf("%s: queue %q: %w", Name, s.Name, err)
		}
	}
	return nil
}

// queued returns the emit values one declaration contributes.
func (p *Plugin) queued(rules sdk.SourceRules, s *sdk.Struct, value *Type) []sdk.EmitNode {
	if p.opts.NoTests {
		return []sdk.EmitNode{value}
	}
	return []sdk.EmitNode{value, &Tests{
		BaseEmit:   sdk.EmitBaseTagged(value.BaseEmit, TestOutputTag),
		TypeName:   value.TypeName,
		SourceName: s.Name,
		CtorName:   value.CtorName,
		FromName:   value.FromName,
		CtorRef:    sdk.NewExternal(s.Package, value.CtorName),
		FromRef:    sdk.NewExternal(s.Package, value.FromName),
		ValueRef:   sdk.NewExternal(s.Package, s.Name),
		TypeParams: value.TypeParams,
		// The witnesses, not the declaration's own parameter list: a
		// check is an ordinary function and has to name concrete types
		// where the declaration named parameters. The template renders
		// them, which is what registers an import for a witness naming
		// another package.
		Witnesses: rules.Witnesses(s.TypeParams),
		Fields:    testFields(rules, s, value.Fields),
		Seeded:    value.Seeded(),
		Companion: value.Companion,
	}}
}

// testFields returns the members as a check names them: the same
// projection, with each type substituted at the witnesses the checks
// instantiate at.
//
// A copy rather than a rewrite, because the builder itself keeps the
// declared spelling — its setters take a `T`, and a check calling one
// passes a `string`. The two views differ in exactly this, so sharing
// the slice put a concrete value into a variable the template had
// declared at a type parameter.
//
// Returns the input where nothing is parameterised, so the common case
// allocates nothing.
func testFields(rules sdk.SourceRules, s *sdk.Struct, fields []Field) []Field {
	if len(s.TypeParams) == 0 {
		return fields
	}
	out := make([]Field, len(fields))
	copy(out, fields)
	for i := range out {
		out[i].Type = rules.SubstituteRef(out[i].Type, s.TypeParams)
		out[i].Elem = rules.SubstituteRef(out[i].Elem, s.TypeParams)
		out[i].Key = rules.SubstituteRef(out[i].Key, s.TypeParams)
	}
	return out
}

// setterWords is this run's vocabulary for the setters a shape owes.
//
// Resolved once per package rather than per member: the words come
// from the language, which is a fact about the package, and looking
// them up per member would ask the same question once per field.
type setterWords struct{ set, add, text, entry, entries string }

// setterWords returns the words a language spells this plugin's
// setters with.
func (p *Plugin) setterWords(lang string) setterWords {
	return setterWords{
		set:     p.Word(lang, WordSet),
		add:     p.Word(lang, WordAppend),
		text:    p.Word(lang, WordText),
		entry:   p.Word(lang, WordEntry),
		entries: p.Word(lang, WordEntries),
	}
}

// collides reports whether two members reach one setter identifier,
// having said which two.
//
// Refused rather than emitted. A duplicate method does not compile, so
// the builder is broken either way — and the difference is whether the
// failure names the two members that caused it or lands in the
// consumer's build of a file they did not write.
func collides(ctx *sdk.GeneratorContext, s *sdk.Struct, fields []Field) bool {
	seen := make(map[string]string, len(fields)*2)
	found := false
	for i := range fields {
		f := &fields[i]
		for _, name := range f.Names() {
			if first, dup := seen[name]; dup {
				ctx.Diag.Errorf(s.Pos(),
					"%s: %s.%s and %s.%s both reach the setter %s; rename one or "+
						"exclude it with %s:%q",
					Name, s.Name, first, s.Name, f.Name, name, SkipTag, SkipValue)
				found = true
				continue
			}
			seen[name] = f.Name
		}
	}
	return found
}

// fieldsOf projects every member a builder can set.
//
// A plain function for the reason [skipped] is: the projection
// depends on the declaration and the language, not on how the plugin
// was configured.
func fieldsOf(
	ctx *sdk.GeneratorContext, rules sdk.SourceRules, words setterWords, s *sdk.Struct,
) []Field {
	members := rules.Settable(s)
	out := make([]Field, 0, len(members))
	for _, m := range members {
		if skipped(ctx, rules, s, m) {
			continue
		}
		f := Field{
			Name:    m.Name,
			Type:    m.Type,
			Default: defaults.DefaultOf(m.Meta),
		}
		if pkg := defaults.DefaultPackage(m.Meta); pkg != "" {
			f.DefaultRef = sdk.NewExternal(pkg, f.Default)
		}
		if m.Source != nil {
			info := rules.TypeOf(m.Source.Type, ctx.Reader)
			f.Shape, f.Elem, f.Key = info.Shape, info.Elem, info.Key
			// Sampled at the witnesses a check instantiates at rather
			// than at the declared type. A type parameter has no value
			// to write, so a parameterised member sampled as declared
			// refuses — and a check dropped for want of a sample is a
			// test function with an empty body, which passes.
			//
			// The declared type is what the sampler walks otherwise:
			// it reads declarations, and the substitution returns them
			// unchanged where nothing names a parameter. It unwraps
			// what it recognises on the way.
			sampled := rules.SubstituteParams(m.Source.Type, s.TypeParams)
			f.Sample, f.Alternate = rules.SamplesOf(sampled, m.Name, ctx.Reader)
			if zero, ok := rules.ZeroLiteral(sampled, ctx.Reader); ok {
				f.DefaultIsZero = f.Declared() && strings.TrimSpace(f.Default) == zero
			}
		}
		nameSetters(rules, words, &f)
		out = append(out, f)
	}
	return out
}

// nameSetters spells the identifiers this member's setters are
// declared under.
//
// The part order is this plugin's — the verb leads the member name,
// and the qualifier trails the whole — and the spelling is the
// language's, which is why each is composed through
// [sdk.SourceRules.TypeName] rather than concatenated here.
func nameSetters(rules sdk.SourceRules, words setterWords, f *Field) {
	f.Set = rules.TypeName(words.set, f.Name)
	switch {
	case f.IsSequence():
		f.Append = rules.TypeName(words.add, f.Name)
	case f.IsBytes():
		f.SetText = rules.TypeName(f.Set, words.text)
	case f.Keyed():
		f.SetEntry = rules.TypeName(f.Set, words.entry)
		f.SetEntries = rules.TypeName(f.Set, words.entries)
	}
}

// skipped reports whether the member opted out of a setter.
//
// A plain function rather than a method: the answer depends on the
// member and the language, not on how the plugin was configured, and
// a receiver it never reads would suggest otherwise.
func skipped(
	ctx *sdk.GeneratorContext, rules sdk.SourceRules, s *sdk.Struct, m sdk.Member,
) bool {
	if m.Source == nil {
		return false
	}
	raw, ok := rules.Tag(m.Source, SkipTag)
	if !ok {
		return false
	}
	if raw != SkipValue {
		ctx.Diag.Errorf(m.Pos,
			"%s: %s.%s carries %s:%q; the only value that excludes a member is %q",
			Name, s.Name, m.Name, SkipTag, raw, SkipValue)
		return false
	}
	return true
}

// companionOf finds the seeding function for s, or nil when none
// applies.
//
// A `defaults=` key names one explicitly, in whichever notations the
// language accepts — which is what lets a companion live in another
// package, including one imported only for this directive. Absent the
// key, the convention applies: a function named for the type and the
// configured word, beside it.
//
// The last declaration wins, matching the per-member directive.
// [sdk.Node.Directive] is first-wins and answers a different question
// — whether the directive is there at all — and two tie-break rules
// for two directives in one repository is a difference nobody can
// predict from the outside.
func (p *Plugin) companionOf(
	ctx *sdk.GeneratorContext, rules sdk.SourceRules, lang string,
	s *sdk.Struct, funcs []*sdk.Function,
) *sdk.Expr {
	dir := sdk.Last(s.Directives(), DirectiveName)
	if dir == nil || dir.KV[CompanionKey] == "" {
		word := p.word(lang, WordCompanion, p.opts.CompanionWord)
		fn := sdk.Companion(funcs, s.Package, rules.TypeName(s.Name, word), s.Name)
		if fn == nil {
			return nil
		}
		return sdk.NewExternal(fn.Package, fn.Name)
	}
	// The qualifier form resolves against the imports of the file that
	// declared the type, so the file is what the resolver needs.
	pkgNode, _ := ctx.Reader.PackageAt(s.Package)
	pkg, symbol, err := rules.ResolveValue(rules.FileOf(pkgNode, s), dir.KV[CompanionKey])
	if err != nil {
		ctx.Diag.Errorf(s.Pos(), "%s: %s on %s: %v", Name, CompanionKey, s.Name, err)
		return nil
	}
	if pkg == "" {
		pkg = s.Package
	}
	return sdk.NewExternal(pkg, symbol)
}

// reportUnread warns once per language this plugin cannot read.
//
// An unmarked package is passed over quietly: the marker names the
// language a package was written in, so its absence means nothing
// claimed it — a fixture, a bridge, a synthesised graph. Warning about
// those would put a diagnostic on every unit test that builds a store
// by hand, which is where the real warning would then go unread.
func (p *Plugin) reportUnread(
	ctx *sdk.GeneratorContext, pkg *sdk.Package, lang string, seen map[string]bool,
) {
	if lang == "" || seen[lang] {
		return
	}
	seen[lang] = true
	ctx.Diag.Warnf(pkg.Pos(),
		"%s: declarations written in %q are not read, so no builder is generated "+
			"for them; this plugin reads: %v",
		Name, lang, p.Languages())
}

// word returns the vocabulary entry to use for a language: the
// caller's override where one was configured, the language's own word
// otherwise.
//
// The order is the point. A configured word is the caller stating
// their repository's convention, which holds across languages; the
// declared one is the language stating its own, which is what a
// caller who said nothing wants. Neither is this file's to invent —
// an empty answer means the language declared no word, and the
// identifier is composed from the type name alone.
func (p *Plugin) word(lang, key, override string) string {
	if override != "" {
		return override
	}
	return p.Word(lang, key)
}
