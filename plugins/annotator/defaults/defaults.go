// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package defaults

import (
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier. It appears in diagnostics,
// in every generated file's provenance header, and in the cache key.
const Name = "defaults"

// Version composes into the pipeline's plugin fingerprint, which
// frontends fold into their cache keys — so a change here invalidates
// a warm cache populated when this annotator stamped differently.
//
// Bump it whenever what gets stamped changes: a different parse, a
// changed precedence rule, a new key.
const Version = "1.0.0"

// Capability is the label the plugin advertises so a reader can
// declare a documentary dependency on defaults having been stamped.
const Capability = "defaults"

// DirectiveName is the bare directive name, without the `+gen:`
// prefix, that declares a field's default.
const DirectiveName sdk.DirectiveName = "default"

// ValueArg names the directive's single positional argument. The
// schema and every diagnostic spell it from here, so the word a user
// sees in a validation failure is the word the schema declared.
const ValueArg = "value"

// DefaultTagKey is the struct-tag key read when [Options.TagKey] is
// unset — `Port int ` + "`default:\"8080\"`" + `.
//
// The obvious word, and one that costs nothing to change: a codebase
// already using `default` for something else sets [Options.TagKey] to
// whatever it does mean.
const DefaultTagKey = "default"

// MetaDefault holds the declared default as source text. Absent, or
// the empty string, means the declaration named none — which is
// distinct from a default of zero, and the reason the stamp is not a
// typed value.
//
//nolint:gochecknoglobals // meta key registration, immutable after init.
var MetaDefault = sdk.EnsureKey(Name+".value", sdk.StringParser)

// MetaDefaultPackage holds the import path a qualified default
// resolved to, empty for a plain literal.
//
//nolint:gochecknoglobals // meta key registration, immutable after init.
var MetaDefaultPackage = sdk.EnsureKey(Name+".package", sdk.StringParser)

// MetaDefaultSource records which form declared the value —
// [SourceDirective] or [SourceTag].
//
// Not needed to render a default, and carried anyway: a generator
// reporting on what it found says "tagged" or "annotated" without
// re-reading the declaration, and a run auditing its own conventions
// can count the two. A reader that does not care never asks.
//
//nolint:gochecknoglobals // meta key registration, immutable after init.
var MetaDefaultSource = sdk.EnsureKey(Name+".source", sdk.StringParser)

// The two forms a default can be declared in, as [MetaDefaultSource]
// records them.
const (
	SourceDirective = "directive"
	SourceTag       = "tag"
)

// DefaultOf returns the declaration's default as source text, or empty
// when it declared none.
//
// One function per key, and no caller reaches [sdk.Key.Get] directly:
// that returns the presence flag alongside the value and makes every
// reader decide what an absent key means, which is a decision this
// package has already made. The empty string is the absence.
func DefaultOf(bag *sdk.Bag) string {
	out, _ := MetaDefault.Get(bag)
	return out
}

// DefaultPackage returns the import path a qualified default resolved
// to, empty when the default is a plain literal.
func DefaultPackage(bag *sdk.Bag) string {
	out, _ := MetaDefaultPackage.Get(bag)
	return out
}

// DefaultSource returns which form declared the value, empty when
// nothing did.
func DefaultSource(bag *sdk.Bag) string {
	out, _ := MetaDefaultSource.Get(bag)
	return out
}

// Options carries the plugin's user-tunable settings.
type Options struct {
	// TagKey is the struct-tag key a field's default is read from.
	// Defaults to [DefaultTagKey]. Stop reading tags with
	// [Options.NoTags] rather than by emptying this; an unset key
	// means "use the default", which is what a zero value has to mean
	// for a caller that set only the other field.
	TagKey string `eidos:"tag-key,default=default"`

	// NoTags stops the plugin reading tags at all, leaving the
	// directive as the only way to declare a default.
	//
	// For a codebase whose tags are owned by something else and whose
	// authors should not discover that a serialisation tag started
	// seeding constructors.
	NoTags bool `eidos:"no-tags"`
}

// Plugin is the defaults annotator. The zero value is unusable; go
// through [New].
type Plugin struct {
	*sdk.Base
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance.
//
// Every language the plugin can read a declared default from is
// declared here, through the same [sdk.Builder.For] a generator
// declares what it renders with — one namespace of language names,
// two halves of one declaration. A consumer registers the plugin and
// nothing else; which half applies is decided per package from the
// language that package was written in. A second language is a
// sibling `defaults_<lang>.go` and one more For call.
//
// The declarations carry no template tree and no output, because this
// plugin emits no file. It stamps metadata a later generator reads,
// and an output it never writes would give Layout a filename to
// compose for a contribution that never arrives.
//
// The shape bucket, so the stamp is on the graph before any generator
// walks it. A reader in the same bucket would see a field stamped or
// not depending on registration order, which is the kind of ordering
// nobody can predict from the outside.
func New() *Plugin {
	p := &Plugin{
		Base: sdk.NewPlugin(Name).
			Version(Version).
			Priority(sdk.AnnotatorShape).
			Provides(Capability).
			Directives(directives()...).
			For(goSupport()).
			Build(),
	}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// directives declares the schema for [DirectiveName].
//
// One required positional, because a default with no value is a line
// the author did not finish writing. No keys: the value is the whole
// directive, and a key beside it would name something no reader has.
// Negation is denied — a default exists exactly where one is
// declared, so deleting the line is the suppression.
func directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			Describe(
				"Seeds the annotated declaration with a value when a generated "+
					"constructor is called without one. Takes one positional "+
					"argument, written as the literal it will render as — a "+
					"quoted string, a number, a keyword, or a package-qualified "+
					"symbol. Repeating the directive takes the last value "+
					"written, and a `default:\"…\"` tag declares the same thing "+
					"where the directive is absent.",
			).
			Positional(ValueArg, sdk.Required()).
			On(sdk.NodeKindField, sdk.NodeKindAlias).
			DenyKeys().
			DenyNegation().
			Build(),
	}
}

// Annotate stamps every declaration's default, reading each package
// through the binding registered for the frontend that produced it.
//
// Per package rather than over the store's flat views, because the
// frontend marker is a fact about a package: a run mixing a Go
// frontend with a protobuf one holds both, and a single global answer
// would read one language's declarations with the other's rules.
//
// Malformed input is reported and dropped rather than guessed at. A
// value the language cannot render would otherwise reach a template
// unexamined and emerge as a syntax error in generated code,
// attributed to the generator rather than to the line that caused it.
func (p *Plugin) Annotate(ctx *sdk.AnnotatorContext) error {
	unhandled := map[string]bool{}
	for _, pkg := range ctx.Reader.Packages().Slice() {
		lang := sdk.LanguageOf(pkg)
		rules, known := p.Source(lang)
		if !known {
			p.reportUnhandled(ctx, pkg, lang, unhandled)
			continue
		}
		p.annotatePackage(ctx, pkg, rules)
	}
	return nil
}

// annotatePackage stamps every declaration in one package.
func (p *Plugin) annotatePackage(
	ctx *sdk.AnnotatorContext, pkg *sdk.Package, rules sdk.SourceRules,
) {
	for _, s := range pkg.Structs {
		// Resolved once per struct rather than per field: every field
		// of a struct is declared in the struct's own file, so the
		// answer cannot differ between them.
		file := rules.FileOf(pkg, s)
		for _, f := range s.Fields {
			p.annotateField(ctx, rules, pkg, file, s.Name, f)
		}
	}
	// The type-level arm. A named type has no field to carry a tag,
	// so the only way to declare its default is the directive — which
	// is why this loop asks for one form and the field loop for two.
	for _, a := range pkg.Aliases {
		if value, ok := directiveValue(a.Directives()); ok {
			stamp(ctx, rules, rules.FileOf(pkg, a), a.Name, a.Name,
				a.Pos(), a.EnsureMeta(), value, SourceDirective)
		}
	}
}

// annotateField stamps one field, taking the directive over the tag.
//
// See this package's documentation for why the directive wins: it is
// the more specific statement, and a rule letting the tag win would
// leave no way to correct one.
func (p *Plugin) annotateField(
	ctx *sdk.AnnotatorContext, rules sdk.SourceRules, pkg *sdk.Package,
	file *sdk.File, owner string, f *sdk.Field,
) {
	if value, ok := directiveValue(f.Directives()); ok {
		stamp(ctx, rules, file, owner, f.Name, f.Pos(), f.EnsureMeta(), value, SourceDirective)
		return
	}
	if p.opts.NoTags {
		return
	}
	value, tagged := rules.Tag(f, p.tagKey())
	if tagged && value != "" {
		value = p.tagLiteral(ctx, rules, pkg, file, owner, f, value)
		if value == "" {
			return
		}
	}
	if !tagged || value == "" {
		// An empty tag value is not a declared default. `default:""`
		// on a string field reads as "seed this to the empty string",
		// which is that field's zero — and a stamp carrying the empty
		// string is this package's spelling of "nothing declared", so
		// the two cannot be told apart. An author who means the empty
		// string writes the directive, where the quotes survive.
		return
	}
	stamp(ctx, rules, file, owner, f.Name, f.Pos(), f.EnsureMeta(), value, SourceTag)
}

// tagLiteral resolves a tag's value into the source text to stamp,
// reporting the empty string when it cannot be one.
//
// Three answers in order, and the order is the whole of it.
//
// A name the package declares is a reference: an author writing
// `default:"DefaultHost"` beside `const DefaultHost = "localhost"`
// means the constant, and quoting it would stamp its own spelling as
// a string. Looked up before the type is consulted, because a textual
// member makes both readings plausible and only one is what was
// written.
//
// Otherwise the type decides, through [sdk.SourceRules.LiteralFor],
// which is given the declaring file so a name qualified against an
// import stays a reference. Go's tag grammar has already consumed one
// layer of quoting, so the bare text is the right literal for a number
// and the wrong one for a string — the member's type is what says
// which, and the file is what says whether the text is a value of that
// type at all.
//
// A value the type cannot admit is reported here, at the declaration.
// Stamped anyway it reaches the consumer's compiler as an error in
// generated source, naming a line the author did not write.
func (p *Plugin) tagLiteral(
	ctx *sdk.AnnotatorContext, rules sdk.SourceRules, pkg *sdk.Package,
	file *sdk.File, owner string, f *sdk.Field, value string,
) string {
	if declaresName(pkg, value) {
		return value
	}
	literal, ok := rules.LiteralFor(file, f.Type, value, ctx.Reader)
	if !ok {
		ctx.Diag.Errorf(f.Pos(),
			"%s: %s.%s has a %s tag of %q, which is not a value its type admits "+
				"and names nothing this package declares",
			Name, owner, f.Name, p.tagKey(), value)
		return ""
	}
	return literal
}

// declaresName reports whether pkg declares a constant or variable
// under this name.
func declaresName(pkg *sdk.Package, name string) bool {
	for _, c := range pkg.Constants {
		if c.Name == name {
			return true
		}
	}
	for _, v := range pkg.Variables {
		if v.Name == name {
			return true
		}
	}
	return false
}

// directiveValue returns the last declared default and whether one
// was declared.
//
// Last write wins. A declaration carrying the directive twice states
// two intentions; taking the last matches how a reader scans a line
// list and is what the schema's description promises.
// [sdk.Node.Directive] answers "is this declared" and is first-wins,
// which is the opposite rule — hence [sdk.Last] rather than the
// method.
func directiveValue(directives []*sdk.Directive) (string, bool) {
	dir := sdk.Last(directives, DirectiveName)
	if dir == nil || len(dir.Args) == 0 {
		return "", false
	}
	return dir.Args[0], true
}

// stamp resolves one declared value and records it.
//
// A plain function rather than a method: everything it needs arrives
// as an argument, and a receiver it never reads would suggest the
// stamp depends on plugin state. It does not — which is what makes it
// safe for the two callers to differ only in the form they read.
func stamp(
	ctx *sdk.AnnotatorContext,
	rules sdk.SourceRules,
	file *sdk.File,
	owner, name string,
	pos sdk.Pos,
	bag *sdk.Bag,
	value, source string,
) {
	pkg, symbol, err := rules.ResolveValue(file, value)
	if err != nil {
		// The value itself comes from the error: [sdk.SourceRules.ResolveValue]
		// quotes it, because the tag form has nothing else to name.
		ctx.Diag.Errorf(pos, "%s: default on %s.%s: %v", Name, owner, name, err)
		return
	}
	MetaDefault.Set(bag, symbol, Name)
	MetaDefaultSource.Set(bag, source, Name)
	if pkg != "" {
		MetaDefaultPackage.Set(bag, pkg, Name)
	}
}

// reportUnhandled warns once per language this plugin cannot read.
//
// Skipping in silence is the failure this exists to prevent: every
// default in those packages would go unstamped, every constructor
// would seed nothing, and the generated output would be a plausible
// file that ignored the source. Once per language rather than per
// package, because one missing declaration is one thing to fix.
//
// An *unmarked* package is not that failure and is passed over
// quietly. The marker names the language a package was written in, so
// its absence means nothing claimed it — a fixture, a bridge, a
// synthesised graph. Those carry no declared defaults to miss, and
// warning about them would put a diagnostic on every unit test that
// builds a store by hand, which is where the real warning would then
// go unread.
func (p *Plugin) reportUnhandled(
	ctx *sdk.AnnotatorContext, pkg *sdk.Package, lang string, seen map[string]bool,
) {
	if lang == "" || seen[lang] {
		return
	}
	seen[lang] = true
	ctx.Diag.Warnf(pkg.Pos(),
		"%s: declarations written in %q are not read, so defaults declared in its "+
			"packages are not stamped; this plugin reads: %v",
		Name, lang, p.Languages())
}

// tagKey returns the configured tag key, or [DefaultTagKey] when the
// option binder has not applied a caller-supplied value — which is
// the case in a unit test constructing the plugin directly.
func (p *Plugin) tagKey() string {
	if p.opts.TagKey != "" {
		return p.opts.TagKey
	}
	return DefaultTagKey
}
