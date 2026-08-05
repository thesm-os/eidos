// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package builder is the production single-output generator for
// fluent test builders. Annotating a source struct with
// `+gen:builder` opts the struct in for a per-source-file
// builder-companion carrying the type's `<Name>Builder`, its
// constructors, per-field setters keyed off each field's
// shape (scalar, slice, map, bytes), [Mutate], [Clone], and
// [Build] entries.
//
// # Language-agnostic core, language-keyed adapters
//
// This file holds the language-neutral plugin core: the
// directive schema, the [Options] surface, the [Type]
// emit value, and the [Plugin.Generate] pass that walks
// annotated source structs and queues one contribution per
// match. The contribution carries the raw [node.Struct], the
// option-resolved [Type.Suffix], and the verbatim
// `defaults=` directive value — every classification /
// identifier-convention / directive-parsing rule is deferred
// to the active backend's language.
//
// Each supported language ships its own sibling file
// (`builder_<lang>.go`) that owns the per-language output
// suffix, the embedded template tree, and the funcmap entries
// the template consumes. The dispatchers below
// ([Plugin.Outputs], [Plugin.Templates], [Plugin.TemplateFuncs])
// route by language to the matching adapter. Adding a new
// target language is a two-step extension: ship a
// `builder_<lang>.go` adapter and a `templates/<lang>/...`
// template tree. No code in this file changes beyond the
// dispatch switch.
package builder

import (
	"fmt"
	"io/fs"
	"text/template"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "builder"

// Capability is the capability label the plugin advertises so
// downstream consumers can declare a documentary dependency
// through their own `Requires` list.
const Capability = "builder"

// DirectiveName is the bare directive name (without the `+gen:`
// or `-gen:` prefix) the plugin reads from each source struct.
const DirectiveName sdk.DirectiveName = "builder"

// DefaultsKey is the directive keyword that pins the explicit
// defaults factory: `+gen:builder defaults=<value>`. The raw
// value is threaded into the rendered template; the active
// language's funcmap parses it according to that language's
// module-path conventions.
const DefaultsKey = "defaults"

// SlotName is the [emit.File] slot the plugin appends its
// rendered [Type] emit values into. `top` renders the
// content between the package clause and the first core decl
// — the natural placement for a self-contained method-and-
// function block. Universal across target languages.
const SlotName = "top"

// KindType is the plugin-defined [emit.Node.Kind] every
// [Type] emit value reports. Universal across target
// languages; the matching template per language lives at
// `templates/<lang>/builder.type.tmpl`.
const KindType sdk.Kind = "builder.type"

// DefaultSuffix is the suffix appended to the source struct's
// name to form the rendered builder identifier
// (`<Type><Suffix>`) when [Options.Suffix] is unset. The
// `Builder` naming convention is widely used across target
// languages; teams that prefer a different convention
// override via [Options.Suffix].
const DefaultSuffix = "Builder"

// Options carries the plugin's user-tunable settings.
type Options struct {
	// Suffix is appended to the source struct's name to form
	// the rendered builder's identifier (`<Type><Suffix>`).
	// Defaults to [DefaultSuffix]. The setting is universal
	// across target languages.
	Suffix string `eidos:"suffix,default=Builder"`
}

// Plugin is the fluent-builder generator. Zero value is
// unusable; go through [New] so the embedded [opt.Holder]
// binds to the options field.
type Plugin struct {
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance with the options holder
// bound. The pipeline overlays caller-supplied option values
// via [Plugin.SetOptions] (promoted from [sdk.Holder]) at
// Build time.
func New() *Plugin {
	p := &Plugin{}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// Name returns [Name].
func (*Plugin) Name() string { return Name }

// Priority places the plugin in the foundation generator
// bucket so composition / cross-cutting plugins see emitted
// builder types when they iterate the post-generation emit
// graph.
func (*Plugin) Priority() sdk.Priority { return sdk.GeneratorFoundation }

// Provides advertises [Capability].
func (*Plugin) Provides() []string { return []string{Capability} }

// Requires returns nil — the builder plugin reads source
// nodes only.
func (*Plugin) Requires() []string { return nil }

// Directives declares the `+gen:builder` / `-gen:builder`
// schema on source struct types. The optional `defaults=`
// keyword arg pins the explicit factory function the
// additional `New<Name>WithDefaults` constructor seeds from;
// parsing of the value is delegated to the active language's
// funcmap.
func (*Plugin) Directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(node.KindStruct).
			AllowedKeys(DefaultsKey).
			Describe(
				"Opts the host struct in for fluent-builder generation. " +
					"`defaults=<value>` adds a `New<Name>WithDefaults` " +
					"constructor seeded from the named factory; no " +
					"auto-discovery is performed. A bare name resolves " +
					"beside the annotated type; a qualified one names " +
					"another package. The value's parsing convention is " +
					"defined per target language.",
			).
			Build(),
	}
}

// Outputs dispatches to the per-language adapter for the
// requested language. Adding a new language adds a `case`
// arm; the unknown-language path returns nil so the
// framework's "no Outputs for this language" semantics
// continues to apply.
func (*Plugin) Outputs(lang string) []sdk.Output {
	if lang == golang.Language {
		return GoOutputs()
	}
	return nil
}

// Templates dispatches to the per-language adapter's embedded
// template tree. The Go backend reads the returned filesystem
// once at Build time to register every `*.tmpl` under
// `templates/<lang>/`.
func (*Plugin) Templates(lang string) (fs.FS, bool) {
	if lang == golang.Language {
		return GoTemplates()
	}
	return nil, false
}

// TemplateFuncs dispatches to the per-language adapter's
// funcmap. The funcmap holds every language-specific
// classification, identifier-convention, and directive-arg
// parsing rule the matching template tree consumes — so
// adding a language is a self-contained per-language
// extension.
func (*Plugin) TemplateFuncs(lang string) template.FuncMap {
	if lang == golang.Language {
		return GoFuncMap()
	}
	return nil
}

// TemplateOverrides returns nil — no per-language adapter
// currently overrides a canonical funcmap entry. Adding an
// override later means registering it on the per-language
// adapter and dispatching here.
func (*Plugin) TemplateOverrides(string) template.FuncMap { return nil }

// Type is the plugin-defined emit kind the rendered
// emit value reports. The matching `builder.type.tmpl`
// template in each language tree renders it as the full
// builder surface for one source struct — type declaration,
// [New<Name>], optional [New<Name>WithDefaults],
// [New<Name>From], per-field setters, [Mutate], [Clone], and
// [Build].
//
// The struct is intentionally minimal: a raw [node.Struct]
// pointer the template walks, an option-derived suffix, and
// the verbatim `defaults=` directive value. All shape
// detection and identifier-convention rules live in the
// active language's funcmap; the data graph carries no
// language-specific projection.
type Type struct {
	sdk.BaseEmit

	// Source is the raw source [node.Struct] the template
	// walks. The active language's funcmap classifies field
	// shapes, projects exported fields, and lifts type
	// parameters / arguments from this single root.
	Source *node.Struct

	// Suffix is the option-resolved suffix appended to the
	// source struct's name to form the builder identifier
	// (`<Source.Name><Suffix>`). Defaults to [DefaultSuffix].
	Suffix string

	// DefaultsArg is the verbatim `defaults=` directive value
	// or the empty string when the arg is absent. The active
	// language's funcmap parses the value at render time,
	// resolving a bare identifier against [Builder.Source]'s
	// package; malformed values surface as render-time errors.
	DefaultsArg string
}

// Kind returns [KindType].
func (*Type) Kind() sdk.Kind { return KindType }

// Compile-time confirmation that *Type satisfies
// [sdk.EmitNode].
var _ sdk.EmitNode = (*Type)(nil)

// Generate walks every source struct the reader exposes and,
// for each opted-in struct, queues one [Type]
// contribution against the file the struct lives in. The
// Layout phase resolves the contribution to a sibling
// builder file via the standard routing precedence; multiple
// annotated structs declared in the same source file collate
// into one rendered output.
//
// Source structs without `+gen:builder` are skipped silently.
// The pass is language-neutral: the contribution carries the
// raw source struct, the suffix, and the raw directive value;
// the active backend's template + funcmap pair produce the
// rendered text.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name, sdk.EmitTarget{})
	for _, s := range ctx.Reader.Structs().Slice() {
		if !s.HasPositiveDirective(DirectiveName) {
			continue
		}
		bt := &Type{
			BaseEmit: sdk.BaseEmit{
				OriginNode: s,
				SetByName:  c.SetBy(),
				SourcePos:  s.Pos(),
			},
			Source:      s,
			Suffix:      p.suffix(),
			DefaultsArg: defaultsValue(s),
		}
		prov := c.Provenance("builder.type." + s.Name)
		if err := ctx.Store.Emit().AppendOriginSlot(s, SlotName, bt, prov); err != nil {
			return fmt.Errorf("%s: append slot for %q: %w", Name, s.Name, err)
		}
	}
	return nil
}

// suffix returns the configured builder-name suffix, or
// [DefaultSuffix] when the option binder has not yet applied
// a caller-supplied value (e.g. unit tests bypassing
// SetOptions).
func (p *Plugin) suffix() string {
	if p.opts.Suffix != "" {
		return p.opts.Suffix
	}
	return DefaultSuffix
}

// defaultsValue returns the raw `defaults=` value on s's
// directive, or the empty string when the keyword arg is
// absent. Parsing happens in the active language's funcmap.
func defaultsValue(s *node.Struct) string {
	d := s.Directive(DirectiveName)
	if d == nil {
		return ""
	}
	return d.KV[DefaultsKey]
}
