// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import (
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"text/template"
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
