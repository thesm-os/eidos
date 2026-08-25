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
// match. The contribution carries the raw [sdk.Struct], the
// option-resolved [Type.Suffix], and the verbatim
// `defaults=` directive value — every classification /
// identifier-convention / directive-parsing rule is deferred
// to the active backend's language.
//
// The Go adapter ships as the sibling `builder_go.go`,
// owning the output suffix, the embedded template tree, and
// the one template helper the template consumes. [sdk.Base]
// answers the declaration methods and keys every one of them
// to Go, so a second target language is more than an extra
// adapter file: it needs `builder_<lang>.go`, a
// `templates/<lang>/...` tree, and Outputs / Templates /
// TemplateFuncs redeclared on [Plugin] to dispatch across
// both. Nothing in the neutral core below changes for it.
package builder

import (
	"fmt"

	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "builder"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

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

// SlotName is the [sdk.EmitFile] slot the plugin appends its
// rendered [Type] emit values into. `top` renders the
// content between the package clause and the first core decl
// — the natural placement for a self-contained method-and-
// function block. Universal across target languages.
const SlotName = "top"

// KindType is the plugin-defined [sdk.EmitNode.Kind] every
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
// unusable; go through [New] so the embedded [sdk.Holder]
// binds to the options field.
type Plugin struct {
	*sdk.Base
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance with the options holder
// bound. The pipeline overlays caller-supplied option values
// via [Plugin.SetOptions] (promoted from [sdk.Holder]) at
// Build time.
//
// The foundation bucket runs ahead of the composition and
// cross-cutting buckets, so a plugin walking the
// post-generation emit graph finds the builder types this pass
// queued.
//
// [Capability] is published so a consumer can declare a
// documentary dependency on builder generation through its own
// Requires list. Nothing is required in return: the plugin
// reads source nodes only and has no upstream plugin
// dependency.
//
// [GoDefaultsExpr] is the plugin's one template helper, and
// the base registers it under the plugin's name prefix — so
// the template calls it as `builder_defaultsExpr`. The
// shape-classification and identifier-convention helpers the
// same template reaches for are canonical backend entries and
// stay unprefixed. No backend builtin is replaced; the
// template renders through the canonical set as it stands.
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

// directives declares the `+gen:builder` / `-gen:builder`
// schema on source struct types. The optional `defaults=`
// keyword arg pins the explicit factory function the
// additional `New<Name>WithDefaults` constructor seeds from;
// parsing of the value is delegated to the active language's
// funcmap.
func directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(sdk.NodeKindStruct).
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

// Type is the plugin-defined emit kind the rendered
// emit value reports. The matching `builder.type.tmpl`
// template in each language tree renders it as the full
// builder surface for one source struct — type declaration,
// [New<Name>], optional [New<Name>WithDefaults],
// [New<Name>From], per-field setters, [Mutate], [Clone], and
// [Build].
//
// The struct is intentionally minimal: a raw [sdk.Struct]
// pointer the template walks, an option-derived suffix, and
// the verbatim `defaults=` directive value. All shape
// detection and identifier-convention rules live in the
// active language's funcmap; the data graph carries no
// language-specific projection.
type Type struct {
	sdk.BaseEmit

	// Source is the raw source [sdk.Struct] the template
	// walks. The active language's funcmap classifies field
	// shapes, projects exported fields, and lifts type
	// parameters / arguments from this single root.
	Source *sdk.Struct

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
	c := sdk.NewProvenance(Name)
	for s := range ctx.Reader.Structs().All() {
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
func defaultsValue(s *sdk.Struct) string {
	d := s.Directive(DirectiveName)
	if d == nil {
		return ""
	}
	return d.KV[DefaultsKey]
}
