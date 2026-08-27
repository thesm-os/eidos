// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package sample lets an author name the values a generated check
// writes for a type.
//
// A language derives what a value looks like from what the type is,
// and for most types that is the whole answer. For two it is not. A
// declaration whose values obey rules its structure does not state
// gets a derivation that satisfies the compiler and nothing else. And
// an interface has no literal form at all, so a member of one is
// refused a value — and every check that needed one is dropped rather
// than written against a guess.
//
// Naming a function is the way in:
//
//	//+gen:sample NewTestAccount alternate=NewOtherAccount
//	type Account struct{ ID, Email string }
//
// # Why the stamp is read by the language
//
// Nothing here is read by a generator. This plugin resolves the names
// and stamps them; [sdk.SourceRules.SamplesOf] prefers them over what
// it would derive, so every consumer that asks a language for a value
// gets the authored one without knowing this plugin exists.
//
// The alternative — each generator reading the stamp itself — is the
// arrangement that puts the same rule in three places and has two of
// them forget it. A check written against a derivation its author
// declared wrong is worse than no check, because it passes.
//
// # Why its own plugin
//
// A directive may be registered once per run, and this one has more
// than one reader by construction: it is consumed inside the language,
// which every generator reaches. Declaring it inside any one generator
// would make every other generator's values depend on that generator
// being registered.
package sample

import (
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "sample"

// Version composes into the pipeline's plugin fingerprint, which
// frontends fold into their cache keys — so bumping it invalidates a
// warm cache populated when this annotator stamped differently.
const Version = "1.0.0"

// Capability is the label the plugin advertises so a consumer can
// declare a documentary dependency on authored sample values.
const Capability = "sample"

// DirectiveName is the bare directive name, without the `+gen:`
// prefix, that names a type's values.
const DirectiveName sdk.DirectiveName = "sample"

// ValueArg is the positional slot naming the function that produces a
// value of the annotated type.
//
// Positional because it is the one every use of this directive has an
// answer for, and a key would make the common case read as
// configuration rather than as the statement it is.
const ValueArg = "value"

// AlternateKey names the function producing a second value, distinct
// from the first.
//
// A key rather than a second positional. The two are not
// interchangeable — a check comparing against a single value passes
// whenever the subject already held it, which is what the second one
// exists to prevent — and two bare names in a row say nothing about
// which is which.
const AlternateKey = "alternate"

// Plugin is the sample annotator. Zero value is unusable; go through
// [New] so the embedded [sdk.Holder] binds to the options field.
type Plugin struct {
	*sdk.Base
	*sdk.Holder[Options]
	opts Options
}

// Options carries the plugin's user-tunable settings.
//
// Empty today. The directive says what the values are and there is
// nothing about that a project-wide setting could sensibly change; the
// struct exists so one can land without changing the plugin's shape.
type Options struct{}

// New returns a fresh plugin instance.
//
// An annotator with no output: it stamps metadata the language reads,
// and declaring a file it never writes would give Layout a filename to
// compose for a contribution that never arrives.
func New() *Plugin {
	p := &Plugin{Base: sdk.NewPlugin(Name).
		Version(Version).
		Priority(sdk.AnnotatorRefinement).
		Provides(Capability).
		Directives(directives()...).
		For(goSupport()).
		Build()}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// directives declares the `+gen:sample` schema.
//
// Neither slot is required by the schema, because either alone is a
// complete statement: a type may need its first value named and accept
// the derived second, or the reverse. A directive naming neither is
// rejected in [Plugin.Annotate], where the two can be looked at
// together.
//
// Negation is denied: a named value exists exactly where one is
// written, so deleting the line is the suppression.
func directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			Describe(
				"Names the values a generated check writes for the annotated "+
					"type, for one whose derived values are wrong or whose form "+
					"admits none. The positional argument names a function taking "+
					"nothing and returning the type; `alternate=` names a second "+
					"such function, returning a value that differs. Either may be "+
					"given alone; the other stays derived. A qualified name or a "+
					"full import path reaches a function in another package. The "+
					"negated form is rejected — deleting the line is the "+
					"suppression.",
			).
			Positional(ValueArg).
			AllowedKeys(AlternateKey).
			On(sdk.NodeKindStruct, sdk.NodeKindAlias, sdk.NodeKindInterface).
			DenyNegation().
			Build(),
	}
}

// Annotate stamps every type's named values.
//
// Per package, because the language a declaration is read with is a
// fact about the package that produced it — see [sdk.LanguageOf].
func (p *Plugin) Annotate(ctx *sdk.AnnotatorContext) error {
	var unread sdk.LanguageReporter
	for _, pkg := range ctx.Reader.Packages().Slice() {
		// SourceOf rather than a lookup on the marked language. The
		// marker names the language a frontend parsed, and a package
		// nothing marked is ordinary input — a synthesised graph, a
		// bridge, a fixture. Asking for the marked language alone skips
		// every one of them, and skipping is indistinguishable from a
		// declaration that named no value.
		rules, lang, ok := p.SourceOf(pkg)
		if !ok {
			unread.Report(ctx.Diag, pkg, Name, lang,
				"are not read, so a value named on one is not stamped and the derived "+
					"value stands", p.Languages())
			continue
		}
		annotatePackage(ctx, pkg, rules)
	}
	return nil
}

// annotatePackage stamps every annotated declaration in one package.
//
// All three kinds a value can be drawn for. An interface is the one
// with the least choice in it: a struct with awkward values still has
// a derivation, and an interface has none — so a member of one loses
// every check that needed a value until this names it.
func annotatePackage(ctx *sdk.AnnotatorContext, pkg *sdk.Package, rules sdk.SourceRules) {
	for _, s := range pkg.Structs {
		annotate(ctx, rules, rules.FileOf(pkg, s), s.Name, s.Package, s)
	}
	for _, a := range pkg.Aliases {
		annotate(ctx, rules, rules.FileOf(pkg, a), a.Name, a.Package, a)
	}
	for _, i := range pkg.Interfaces {
		annotate(ctx, rules, rules.FileOf(pkg, i), i.Name, i.Package, i)
	}
}

// annotate stamps one declaration's named values.
//
// Last write wins, matching every other per-declaration directive in
// this repository: a declaration carrying the directive twice states
// two intentions, and taking the last matches how a reader scans a
// line list. [sdk.Node.Directive] is first-wins and answers a
// different question — whether the directive is there at all.
func annotate(
	ctx *sdk.AnnotatorContext, rules sdk.SourceRules,
	file *sdk.File, owner, pkgPath string, n sdk.Node,
) {
	dir := sdk.Last(n.Directives(), DirectiveName)
	if dir == nil {
		return
	}
	var value string
	if len(dir.Args) > 0 {
		value = dir.Args[0]
	}
	alternate := dir.KV[AlternateKey]
	if value == "" && alternate == "" {
		ctx.Diag.Errorf(dir.Pos,
			"%s: +gen:%s on %s names no value; give the function positionally, "+
				"as %s=, or delete the line",
			Name, DirectiveName, owner, AlternateKey)
		return
	}
	bag := n.EnsureMeta()
	stamp(ctx, rules, file, owner, pkgPath, dir.Pos, bag, value,
		sdk.MetaSample, sdk.MetaSamplePackage)
	stamp(ctx, rules, file, owner, pkgPath, dir.Pos, bag, alternate,
		sdk.MetaAlternate, sdk.MetaAlternatePackage)
}

// stamp resolves one named function and records it.
//
// The package is recorded even where the function sits in the
// declaring package's own file. A consumer renders the call from
// wherever its output was routed, and a bare name there binds to
// whatever else is in scope rather than failing — so the qualifier is
// what makes a wrong answer a compile error instead of a silent one.
func stamp(
	ctx *sdk.AnnotatorContext, rules sdk.SourceRules,
	file *sdk.File, owner, pkgPath string, pos sdk.Pos, bag *sdk.Bag,
	value string, symbolKey, pkgKey sdk.Key[string],
) {
	if value == "" {
		return
	}
	pkg, symbol, err := rules.ResolveValue(file, value)
	if err != nil {
		// The value comes from the error: ResolveValue quotes what it
		// was handed, because its other callers read a tag and have
		// nothing else to name.
		ctx.Diag.Errorf(pos, "%s: on %s: %v", Name, owner, err)
		return
	}
	if pkg == "" {
		pkg = pkgPath
	}
	symbolKey.Set(bag, symbol, Name)
	pkgKey.Set(bag, pkg, Name)
}
