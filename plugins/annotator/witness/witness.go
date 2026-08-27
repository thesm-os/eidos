// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package witness lets an author name the concrete type a generated
// entry point instantiates a type parameter at.
//
// A generated check is an ordinary function, and an ordinary function
// cannot take type parameters — so anything exercising a generic
// declaration has to name concrete types somewhere. A language derives
// them where the constraint's type set is knowable without loading the
// package that declares it, which in Go is `any` and `comparable` and
// nothing else. A parameter bounded by `constraints.Ordered`, or by a
// project's own interface, is a reference into a package the generator
// never read: no witness can be derived, and every check over the
// declaration is withheld rather than written against a guess.
//
// Naming the type is the way in:
//
//	//+gen:witness T=int
//	//+gen:builder
//	type Sorted[T constraints.Ordered] struct{ Items []T }
//
// # Why the stamp is read by the language
//
// Nothing here is read by a generator. This plugin resolves the names
// and stamps them; [sdk.SourceRules.Witnesses] prefers them over what
// it would derive, so every consumer that asks a language which types
// to instantiate at gets the authored ones without knowing this plugin
// exists. The same arrangement the sample annotator uses, for the same
// reason: a stamp each generator has to remember to prefer is a stamp
// two of them will forget.
//
// # Why its own plugin
//
// A directive may be registered once per run, and this one has more
// than one reader by construction — it is consumed inside the
// language, which every generator reaches. Declaring it inside any one
// generator would make every other generator's instantiations depend
// on that generator being registered.
package witness

import (
	"strings"

	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "witness"

// Version composes into the pipeline's plugin fingerprint, which
// frontends fold into their cache keys — so bumping it invalidates a
// warm cache populated when this annotator stamped differently.
const Version = "1.0.0"

// Capability is the label the plugin advertises so a consumer can
// declare a documentary dependency on authored witnesses.
const Capability = "witness"

// DirectiveName is the bare directive name, without the `+gen:`
// prefix, that names a declaration's witnesses.
const DirectiveName sdk.DirectiveName = "witness"

// Plugin is the witness annotator. Zero value is unusable; go through
// [New] so the embedded [sdk.Holder] binds to the options field.
type Plugin struct {
	*sdk.Base
	*sdk.Holder[Options]
	opts Options
}

// Options carries the plugin's user-tunable settings.
//
// Empty today. The directive says which type each parameter takes and
// there is nothing about that a project-wide setting could sensibly
// change; the struct exists so one can land without changing the
// plugin's shape.
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

// directives declares the `+gen:witness` schema.
//
// Every argument is a KV pair naming one parameter, so the schema
// declares no positional and names no allowed key — an empty allowed
// set accepts any, which is what this needs: the keys are the
// declaration's own parameter names, and they differ per declaration.
// A key naming no parameter is reported in [Plugin.Annotate], where
// the declaration is in hand.
//
// Negation is denied: a named witness exists exactly where one is
// written, so deleting the line is the suppression.
func directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			Describe(
				"Names the concrete type each of the annotated declaration's "+
					"type parameters is instantiated at, as `<param>=<type>` per "+
					"parameter. For a declaration whose constraints a language "+
					"cannot reason about, which otherwise gets no generated checks "+
					"at all — a check is an ordinary function and cannot take type "+
					"parameters. A bare name is used as written, so it must need no "+
					"import; a qualified name or a full import path reaches a type "+
					"in another package. The negated form is rejected — deleting "+
					"the line is the suppression.",
			).
			On(sdk.NodeKindStruct, sdk.NodeKindInterface, sdk.NodeKindAlias).
			DenyNegation().
			Build(),
	}
}

// Annotate stamps every declaration's named witnesses.
//
// Per package, because the language a declaration is read with is a
// fact about the package that produced it — see [sdk.LanguageOf].
//
// All three kinds the schema admits are walked. A kind declared and
// not walked is the worst shape this plugin can take: the directive
// validates, nothing is stamped, no diagnostic is raised, and
// [sdk.SourceRules.Witnesses] falls back to derivation — which answers
// nil for exactly the constraints an authored witness exists to serve.
// The author's line is discarded and the run stays green.
func (p *Plugin) Annotate(ctx *sdk.AnnotatorContext) error {
	var unread sdk.LanguageReporter
	for _, pkg := range ctx.Reader.Packages().Slice() {
		// SourceOf rather than a lookup on the marked language, for the
		// reason the sample annotator gives: a package nothing marked
		// is ordinary input — a synthesised graph, a bridge, a fixture
		// — and asking for the marked language alone skips every one of
		// them, which is indistinguishable from a declaration that
		// named no witness.
		rules, lang, ok := p.SourceOf(pkg)
		if !ok {
			unread.Report(ctx.Diag, pkg, Name, lang,
				"are not read, so a witness named on one is not stamped and the "+
					"declaration keeps whatever the language derives", p.Languages())
			continue
		}
		for _, s := range pkg.Structs {
			annotate(ctx, rules, rules.FileOf(pkg, s), s.QName(), s.TypeParams, s)
		}
		// Interfaces carry the weight here rather than being an
		// afterthought: a generator that doubles a contract works over
		// interfaces exclusively, so leaving them out put the whole
		// authored half of the mechanism out of reach for one.
		for _, i := range pkg.Interfaces {
			annotate(ctx, rules, rules.FileOf(pkg, i), i.QName(), i.TypeParams, i)
		}
		for _, a := range pkg.Aliases {
			annotate(ctx, rules, rules.FileOf(pkg, a), a.QName(), a.TypeParams, a)
		}
	}
	return nil
}

// annotate stamps one declaration's named witnesses.
//
// Last write wins, matching every other per-declaration directive in
// this repository: a declaration carrying the directive twice states
// two intentions, and taking the last matches how a reader scans a
// line list.
func annotate(
	ctx *sdk.AnnotatorContext, rules sdk.SourceRules,
	file *sdk.File, owner string, params []*sdk.TypeParam, n sdk.Node,
) {
	dir := sdk.Last(n.Directives(), DirectiveName)
	if dir == nil {
		return
	}
	if len(params) == 0 {
		ctx.Diag.Errorf(dir.Pos,
			"%s: +gen:%s on %s, which declares no type parameter; "+
				"there is nothing to instantiate",
			Name, DirectiveName, owner)
		return
	}
	if len(dir.KV) == 0 {
		ctx.Diag.Errorf(dir.Pos,
			"%s: +gen:%s on %s names no witness; write one %s=<type> pair "+
				"per parameter, or delete the line",
			Name, DirectiveName, owner, params[0].Name)
		return
	}
	byName := make(map[string]*sdk.TypeParam, len(params))
	for _, param := range params {
		if param != nil {
			byName[param.Name] = param
		}
	}
	for key, value := range dir.KV {
		param, declared := byName[key]
		if !declared {
			// Named rather than counted: a typo in a parameter name is
			// the likeliest way to write this directive wrongly, and it
			// is otherwise silent — the parameter keeps its derived
			// witness, or the declaration keeps none.
			ctx.Diag.Errorf(dir.Pos,
				"%s: +gen:%s on %s names %q, which is not one of its type "+
					"parameters (%s)",
				Name, DirectiveName, owner, key, paramNames(params))
			continue
		}
		stamp(ctx, rules, file, owner, dir.Pos, param.EnsureMeta(), key, value)
	}
}

// stamp resolves one witness and records it on the parameter.
//
// A bare name keeps an empty package, which is the difference from an
// authored sample: a sample names a function, which always lives
// somewhere, while a witness is commonly a builtin whose spelling
// needs no import. Defaulting to the declaring package would qualify
// `string`.
func stamp(
	ctx *sdk.AnnotatorContext, rules sdk.SourceRules,
	file *sdk.File, owner string, pos sdk.Pos, bag *sdk.Bag, param, value string,
) {
	pkg, symbol, err := rules.ResolveValue(file, value)
	if err != nil {
		ctx.Diag.Errorf(pos, "%s: on %s, witness for %s: %v", Name, owner, param, err)
		return
	}
	sdk.MetaWitness.Set(bag, symbol, Name)
	if pkg != "" {
		sdk.MetaWitnessPackage.Set(bag, pkg, Name)
	}
}

// paramNames lists a declaration's parameters for a diagnostic.
func paramNames(params []*sdk.TypeParam) string {
	out := make([]string, 0, len(params))
	for _, p := range params {
		if p != nil {
			out = append(out, p.Name)
		}
	}
	return strings.Join(out, ", ")
}
