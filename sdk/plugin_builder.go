// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import (
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"text/template"

	"go.thesmos.sh/eidos/core/diag"
)

// TemplateDirFor is where a plugin keeps a language's templates,
// relative to its own package.
//
// A convention between plugins rather than a choice any one of them
// makes: the backend registers templates by their base filename, so
// the directory never appears in a template reference and varying it
// buys nothing. [LanguageSupport.TemplateDir] overrides it for a
// plugin whose tree is laid out differently.
func TemplateDirFor(lang string) string { return "templates/" + lang }

// LanguageSupport is everything a plugin declares for one target
// language.
//
// The four fields are exactly the ones the pipeline asks about per
// language — [plugin.TemplateProvider] and the output set have taken
// a lang argument since they were written. Grouping them here rather
// than at the top of [Builder] is what lets one plugin answer for
// several: the suffix `_builder.go` and the suffix `_builder.rs` are
// the same declaration about different languages, not two
// declarations about one.
type LanguageSupport struct {
	// Templates is the plugin's template tree for this language,
	// usually an embed.FS rooted at the plugin's package. Nil for a
	// plugin that renders through the backend's own kind templates —
	// see Builtin.
	Templates fs.FS

	// TemplateDir is the subdirectory of Templates holding this
	// language's files. Empty means [TemplateDirFor] of the language.
	TemplateDir string

	// Outputs is the file set the plugin emits for this language.
	// Suffixes are language-specific by nature, which is the reason
	// this field is per-language rather than shared.
	Outputs []Output

	// Funcs are the plugin's own template helpers, registered under
	// the names given. A template calls a helper by the name its
	// plugin declared, and a plugin naming an entry the language
	// bundle also provides replaces it.
	Funcs template.FuncMap

	// Overrides replace canonical backend funcmap entries. Reserved
	// names are rejected by the backend at Build time.
	Overrides template.FuncMap

	// Builtin declares that the plugin owns a file for this language
	// but renders it through the backend's own kind templates, so the
	// missing tree is deliberate rather than an omission.
	Builtin bool

	// Words is the plugin's own vocabulary for this language — the
	// words its generated identifiers carry, keyed by whatever the
	// plugin calls them.
	//
	// A generated identifier has two halves, and they belong to
	// different owners. `Builder` in `UserBuilder` is the generator's
	// word: it says what the type is for, and a repository preferring
	// `Factory` changes it without the generator knowing. How that
	// word joins `User` — concatenated and Pascal-cased, or
	// lowercased and separated — is the language's, answered through
	// [SourceRules.TypeName].
	//
	// Declared per language because the word itself can differ: a
	// convention that reads as `Defaults` in one language reads as
	// `default` in another, and a generator holding one constant
	// would be spelling every language's output in the first
	// language's idiom.
	//
	// Read through [Base.Word], which falls back to the empty string
	// for a language that declared none — a generator treats that as
	// "this language has no word for it" rather than substituting its
	// own.
	Words map[string]string

	// Replaces names the templates this plugin deliberately replaces
	// for this language — see [plugin.TemplateReplacer].
	//
	// Empty for almost every plugin. A name here is one another plugin
	// ships and this one supersedes, which is how a consumer changes
	// what a generator emits without forking it.
	Replaces []string

	// Source is how the plugin reads declarations written in this
	// language — see [SourceRules]. Nil for a plugin that renders the
	// language but never reads it.
	//
	// Here rather than in a registry of its own because there is one
	// namespace of language names: a frontend stamps the language it
	// parsed and a backend answers to the same name. What differs is
	// which language a caller looks up, not where the answer is kept.
	Source SourceRules
}

// Builder accumulates a plugin's declarations, then freezes them into
// a [Base] via [Builder.Build].
//
// Not safe for concurrent use, and does not need to be: it exists for
// the length of one New call.
type Builder struct {
	base Base
	// langs holds each declared language in registration order, so
	// Build reports the first offender rather than a random one.
	langs []string
	specs map[string]LanguageSupport
}

// NewPlugin starts a builder for a plugin.
//
// A plugin that ships no templates at all — an annotator, or a
// generator contributing only into other plugins' slots — declares no
// language and is language-agnostic by construction. Composition
// happens at the emit layer, where there is no language yet.
func NewPlugin(name string) *Builder {
	return &Builder{
		base:  Base{name: name, priority: DefaultPriority},
		specs: map[string]LanguageSupport{},
	}
}

// For declares what the plugin emits and renders for one language.
// Calling it twice for the same language replaces the earlier
// declaration.
func (b *Builder) For(lang string, s LanguageSupport) *Builder {
	if _, dup := b.specs[lang]; !dup {
		b.langs = append(b.langs, lang)
	}
	b.specs[lang] = s
	return b
}

// Version sets the plugin's version identifier.
//
// It composes into the pipeline's plugin fingerprint, which frontends
// fold into their cache keys — so a plugin declaring none contributes
// an empty string and can never invalidate a warm cache populated
// when it behaved differently.
func (b *Builder) Version(v string) *Builder {
	b.base.version = v
	return b
}

// Priority sets the bucket the plugin runs in.
func (b *Builder) Priority(p Priority) *Builder {
	b.base.priority = p
	return b
}

// Provides declares the capabilities the plugin publishes.
func (b *Builder) Provides(capabilities ...string) *Builder {
	b.base.provides = append(b.base.provides, capabilities...)
	return b
}

// Requires declares the capabilities the plugin depends on.
func (b *Builder) Requires(capabilities ...string) *Builder {
	b.base.requires = append(b.base.requires, capabilities...)
	return b
}

// Directives declares the directive schemas the plugin owns.
func (b *Builder) Directives(schemas ...DirectiveSchema) *Builder {
	b.base.directives = append(b.base.directives, schemas...)
	return b
}

// langData is one language's frozen declarations.
type langData struct {
	outputs   []Output
	templates fs.FS
	funcs     template.FuncMap
	overrides template.FuncMap
	replaces  []string
	words     map[string]string
	source    SourceRules
}

// Build freezes the declarations into a [Base].
//
// # Failure mode
//
// Panics on a declaration the pipeline cannot serve. Every check here
// is a mistake in a plugin's own New — not in its input — so it fires
// on the first construction in any test, before a run exists.
//
//nolint:forbidigo // construction-time contract violation; see the package docs.
func (b *Builder) Build() *Base {
	if b.base.name == "" {
		panic("sdk: plugin name is empty; the pipeline keys registration, " +
			"provenance and directive scoping on it")
	}
	b.base.langs = make(map[string]langData, len(b.specs))

	for _, lang := range b.langs {
		s := b.specs[lang]
		b.validate(lang, s)

		data := langData{
			outputs:   slices.Clone(s.Outputs),
			overrides: s.Overrides,
			replaces:  slices.Clone(s.Replaces),
			words:     maps.Clone(s.Words),
			source:    s.Source,
		}
		if s.Templates != nil {
			dir := s.TemplateDir
			if dir == "" {
				dir = TemplateDirFor(lang)
			}
			// The error is discarded rather than branched on: the
			// directory is a value the //go:embed directive already
			// validated against the tree, so the branch is unreachable
			// from a test. A directory absent from the tree yields an
			// empty FS, which the backend reports as a plugin shipping
			// no templates.
			sub, _ := fs.Sub(s.Templates, dir)
			data.templates = sub
		}
		if !s.Builtin {
			data.funcs = maps.Clone(s.Funcs)
		}
		b.base.langs[lang] = data
	}

	frozen := b.base
	return &frozen
}

// validate reports a per-language declaration the pipeline cannot
// serve.
//
//nolint:forbidigo // construction-time contract violation; see Build.
func (b *Builder) validate(lang string, s LanguageSupport) {
	if len(s.Outputs) > 0 && s.Templates == nil && !s.Builtin {
		panic(fmt.Sprintf("sdk: plugin %q declares %d output(s) for %q and no "+
			"template tree; the backend resolves a template per emit kind and "+
			"would find none. Set LanguageSupport.Builtin if it renders through "+
			"the backend's own kind templates", b.base.name, len(s.Outputs), lang))
	}
	if s.Templates != nil && s.Builtin {
		panic(fmt.Sprintf("sdk: plugin %q declares both a template tree and Builtin "+
			"for %q; the second says there is no tree to register",
			b.base.name, lang))
	}
	seen := map[string]struct{}{}
	for i, o := range s.Outputs {
		if o.Suffix == "" {
			panic(fmt.Sprintf("sdk: plugin %q output %d for %q has no suffix; Layout "+
				"composes every filename from one", b.base.name, i, lang))
		}
		if _, dup := seen[o.Tag]; dup {
			panic(fmt.Sprintf("sdk: plugin %q declares output tag %q twice for %q; "+
				"directive scoping and CLI overrides address an output by tag",
				b.base.name, o.Tag, lang))
		}
		seen[o.Tag] = struct{}{}
	}
}

// Base answers the pipeline's declaration methods for a plugin.
// Embed it by pointer and construct it through [Builder].
//
// # Concurrency
//
// Every field is written once by [Builder.Build] and read afterwards,
// including by the backend's render pool. Nothing here locks, because
// nothing here mutates.
type Base struct {
	name       string
	version    string
	priority   Priority
	provides   []string
	requires   []string
	directives []DirectiveSchema
	langs      map[string]langData
}

// Name returns the plugin's stable identifier.
func (b *Base) Name() string { return b.name }

// Version returns the plugin's version identifier.
func (b *Base) Version() string { return b.version }

// Priority returns the bucket the plugin runs in.
func (b *Base) Priority() Priority { return b.priority }

// Provides returns the capabilities the plugin publishes.
func (b *Base) Provides() []string { return slices.Clone(b.provides) }

// Requires returns the capabilities the plugin depends on.
func (b *Base) Requires() []string { return slices.Clone(b.requires) }

// Directives returns the directive schemas the plugin owns.
func (b *Base) Directives() []DirectiveSchema { return slices.Clone(b.directives) }

// Languages returns the languages the plugin declared, in
// registration order.
func (b *Base) Languages() []string { return slices.Clone(b.langsInOrder()) }

// langsInOrder returns declared languages sorted, so the answer is
// stable across runs regardless of map iteration.
func (b *Base) langsInOrder() []string {
	return slices.Sorted(maps.Keys(b.langs))
}

// Outputs returns the file set the plugin emits for lang, and nothing
// for a language it does not target.
func (b *Base) Outputs(lang string) []Output {
	return slices.Clone(b.langs[lang].outputs)
}

// Templates returns the plugin's template tree for lang.
//
// The bool distinguishes "no templates for this language" from an
// empty tree, which is what lets the backend tell a plugin that does
// not target the language from one whose tree failed to embed.
func (b *Base) Templates(lang string) (fs.FS, bool) {
	d, ok := b.langs[lang]
	if !ok || d.templates == nil {
		return nil, false
	}
	return d.templates, true
}

// TemplateFuncs returns the plugin's template helpers for lang.
func (b *Base) TemplateFuncs(lang string) template.FuncMap {
	return maps.Clone(b.langs[lang].funcs)
}

// TemplateOverrides returns the canonical funcmap entries the plugin
// replaces for lang.
func (b *Base) TemplateOverrides(lang string) template.FuncMap {
	return maps.Clone(b.langs[lang].overrides)
}

// Word returns the plugin's word for key in lang, empty when the
// language declared none.
//
// Empty is the honest answer rather than a default: a generator
// substituting its own would spell one language's idiom into
// another's output, which is the reason the words are declared per
// language at all.
func (b *Base) Word(lang, key string) string {
	return b.langs[lang].words[key]
}

// ReplacesTemplates returns the template names the plugin replaces
// for lang, satisfying [plugin.TemplateReplacer].
func (b *Base) ReplacesTemplates(lang string) []string {
	return slices.Clone(b.langs[lang].replaces)
}

// Source returns how the plugin reads declarations written in lang.
//
// The bool distinguishes "this plugin does not read that language"
// from a language it renders but never reads, which are different
// answers: the first is a plugin asked about something it never
// declared, the second a deliberate nil. An annotator resolves the
// language from the package it is looking at — see [LanguageOf] —
// rather than from the language the run renders, because a run may
// parse one and render another.
func (b *Base) Source(lang string) (SourceRules, bool) {
	d, ok := b.langs[lang]
	if !ok || d.source == nil {
		return nil, false
	}
	return d.source, true
}

// SourceOf returns how the plugin reads declarations in pkg.
//
// The lookup a generator makes, resolving the package's own language
// through [LanguageOf] — a declaration is read with the rules that
// parsed it, not with the language the run renders.
//
// An unmarked package falls back to the plugin's only declared
// language, where it has exactly one. Nothing claimed the package, so
// there is no marker to disagree with, and a plugin that speaks one
// language has no ambiguity to resolve: a fixture, a bridge or a
// synthesised graph reads under the rules the plugin has. A plugin
// declaring several refuses instead, because picking one would be a
// guess that renders plausible output from the wrong rules.
//
// The resolved language is returned beside the rules, because a
// caller reading the plugin's own per-language declarations —
// [Base.Word] among them — needs the name the fallback settled on
// rather than the empty marker it started from.
//
// False when the package names a language the plugin does not read,
// which is a caller's signal to report rather than to skip: every
// declaration in it would otherwise go unemitted with nothing saying
// why.
func (b *Base) SourceOf(pkg *Package) (rules SourceRules, lang string, ok bool) {
	if named := LanguageOf(pkg); named != "" {
		r, found := b.Source(named)
		return r, named, found
	}
	langs := b.langsInOrder()
	if len(langs) != 1 {
		return nil, "", false
	}
	r, found := b.Source(langs[0])
	return r, langs[0], found
}

// LanguageReporter warns once per language a plugin cannot read.
//
// The counterpart to [Base.SourceOf]'s false: that result is a
// caller's signal to report rather than skip, and this is the
// reporting. Every generator wrote it — three in this repository and
// two downstream, each a `seen[lang]` guard and one Warnf naming
// [Base.Languages] — and the copies had already begun to differ in
// what they said while agreeing on what they meant.
//
// The failure it prevents is the invisible one. A run over a language
// nothing reads emits nothing for it and ends green; the output is
// short rather than wrong, and a reader has no line to notice. That
// is the same shape as the sixteen hand-written language dispatches
// [Base] absorbed, where two compared against a local constant and
// silently matched nothing.
//
// The zero value is usable: a nil map is allocated on first use, so a
// caller declaring one as a local `var seen sdk.LanguageReporter`
// need not make it.
type LanguageReporter map[string]bool

// Report warns once that declarations in lang go unread, ending the
// sentence on because — "are not read, so no builder is generated for
// them".
//
// The clause is the caller's because it is the only part that differs:
// what a generator does not produce is its own to say, and a shared
// sentence would either name no output or name one generator's.
//
// An unmarked package is passed over in silence. The marker names the
// language a package was written in, so its absence means nothing
// claimed it — a fixture, a bridge, a synthesised graph — and warning
// about those would put a diagnostic on every unit test that builds a
// store by hand, which is where the real warning would then go unread.
func (r *LanguageReporter) Report(
	sink *diag.Sink, pkg *Package, plugin, lang, because string, langs []string,
) {
	if lang == "" || sink == nil || pkg == nil {
		return
	}
	if *r == nil {
		*r = make(LanguageReporter, 1)
	}
	if (*r)[lang] {
		return
	}
	(*r)[lang] = true
	sink.Warnf(pkg.Pos(),
		"%s: declarations written in %q %s; this plugin reads: %v",
		plugin, lang, because, langs)
}
