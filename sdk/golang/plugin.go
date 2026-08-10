// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"embed"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"text/template"
	"unicode"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/sdk"
)

// DefaultTemplateDir is where a plugin keeps its Go templates,
// relative to its own package.
//
// A convention between plugins rather than a choice any one of them
// makes: the backend registers templates by their base filename, so
// the directory never appears in a template reference and varying it
// buys nothing. [Builder.TemplateDir] overrides it for a plugin whose
// tree is laid out differently.
const DefaultTemplateDir = "templates/golang"

// FuncPrefixSeparator joins a plugin's name to its template function
// names.
//
// The backend rejects two plugins registering the same extension
// name outright, so an unprefixed bundle fails every run rather than
// one output. Applied here rather than by the caller: a plugin that
// spells its own prefix can spell it differently from the one the
// backend attributes its templates to, and nothing catches that
// until the collision.
const FuncPrefixSeparator = "_"

// FuncPrefix composes the template-function prefix for a plugin name.
//
// text/template requires every registered function name to be a Go
// identifier — letters, digits and underscore, with a non-digit first
// rune — and rejects anything else by panicking inside
// [template.Template.Funcs]. Plugin names carry no such constraint:
// `debug-weaver` and `if-match` are ordinary, and a name is a
// user-visible identity that directive scoping and provenance
// attribution both key on, so it cannot be bent to suit the template
// engine.
//
// Every rune the engine will not accept is therefore folded to an
// underscore here — `debug-weaver` prefixes as `debug_weaver_`. The
// fold is not injective, so two plugins named `a-b` and `a.b` would
// share a prefix; the backend rejects duplicate extension names
// outright, so that collides loudly at registration rather than
// silently at render.
//
// Templates are unaffected: a hyphenated helper name could never be
// referenced from a template in the first place, because the template
// parser reads `debug-weaver_x` as a subtraction.
func FuncPrefix(name string) string {
	out := []rune(name + FuncPrefixSeparator)
	for i, r := range out {
		if !isIdentRune(r, i) {
			out[i] = '_'
		}
	}
	return string(out)
}

// isIdentRune reports whether r is legal at position i of a Go
// identifier, matching the rule text/template's own `goodName` applies.
func isIdentRune(r rune, i int) bool {
	switch {
	case r == '_':
		return true
	case unicode.IsLetter(r):
		return true
	case unicode.IsDigit(r):
		return i > 0
	default:
		return false
	}
}

// Builder accumulates a plugin's declarations. Terminated by
// [Builder.Build], which freezes them into a [Base].
//
// Not safe for concurrent use, and does not need to be: it exists
// for the length of one New call.
type Builder struct {
	base    Base
	dir     string
	tree    embed.FS
	set     bool
	builtin bool
}

// NewPlugin starts a builder for a plugin that ships no templates —
// an annotator, or a generator contributing only into other plugins'
// slots. [NewGenerator] is the shorter path for anything that emits
// a file of its own.
func NewPlugin(name string) *Builder {
	return &Builder{
		base: Base{name: name, priority: sdk.DefaultPriority},
		dir:  DefaultTemplateDir,
	}
}

// NewGenerator starts a builder for a plugin that renders Go files,
// taking the three things every one of them needs.
//
// Positional because they are not optional: a generator without a
// template tree renders nothing, and one without an output set gives
// Layout no filename to compose. The rest of the declaration chains
// off the result.
func NewGenerator(name string, templates embed.FS, outputs ...sdk.Output) *Builder {
	return NewPlugin(name).Templates(templates).Outputs(outputs...)
}

// Version sets the plugin's version identifier.
//
// It composes into the pipeline's plugin fingerprint, which
// frontends fold into their cache keys — so a plugin declaring none
// contributes an empty string and can never invalidate a warm cache
// populated when it behaved differently.
func (b *Builder) Version(v string) *Builder {
	b.base.version = v
	return b
}

// BuiltinTemplates declares that the plugin owns a file but renders it
// entirely through the backend's own kind templates.
//
// A plugin needs a template tree only for emit kinds it *defines*. One
// that emits nothing but standard decls — a struct, its fields, its
// methods — has nothing a plugin-local template could resolve, and the
// backend already knows how to render every one of them.
//
// Declared rather than inferred from the absence of a tree, because
// the two mistakes are opposite and only the plugin knows which it is
// making: a generator that defines a [sdk.Kind] and forgot its
// templates renders a short file and fails nowhere, which is the case
// [Builder.Build] panics on. Saying so here keeps that diagnostic
// pointed at the accident while letting the deliberate shape through.
//
// Suppresses [Base.TemplateFuncs], which would otherwise register the
// shared helper bundle for a plugin that has no template able to call
// it: a plugin's helpers are bound only to its own templates at parse
// time, so a bundle without a tree is unreachable by construction.
func (b *Builder) BuiltinTemplates() *Builder {
	b.builtin = true
	return b
}

// Templates sets the embedded tree the plugin's Go templates live
// in, rooted at [DefaultTemplateDir] unless [Builder.TemplateDir]
// says otherwise.
func (b *Builder) Templates(tree embed.FS) *Builder {
	b.tree, b.set = tree, true
	return b
}

// TemplateDir overrides the subdirectory [Builder.Templates] is
// rooted at.
func (b *Builder) TemplateDir(dir string) *Builder {
	b.dir = dir
	return b
}

// Outputs sets the ordered file set the plugin emits for Go.
//
// Order is load-bearing: Layout composes the primary filename from
// the entry at index 0.
func (b *Builder) Outputs(outputs ...sdk.Output) *Builder {
	b.base.outputs = slices.Clone(outputs)
	return b
}

// Priority sets the bucket the plugin runs in. Unset leaves
// [sdk.DefaultPriority], which is where a plugin declaring no
// capability already sits.
func (b *Builder) Priority(p sdk.Priority) *Builder {
	b.base.priority = p
	return b
}

// Provides declares the capability labels downstream plugins may
// depend on.
func (b *Builder) Provides(capabilities ...string) *Builder {
	b.base.provides = slices.Clone(capabilities)
	return b
}

// Requires declares the capability labels this plugin depends on.
func (b *Builder) Requires(capabilities ...string) *Builder {
	b.base.requires = slices.Clone(capabilities)
	return b
}

// Directives declares the directive schemas the plugin reads.
func (b *Builder) Directives(schemas ...sdk.DirectiveSchema) *Builder {
	b.base.directives = slices.Clone(schemas)
	return b
}

// Funcs adds template helpers on top of the shared Go bundle.
//
// Keys are prefixed with the plugin's name exactly as the shared
// bundle's are, so an author never writes the prefix and cannot
// collide with another plugin. A key matching a shared helper
// replaces it for this plugin's templates only, which is the
// supported way to specialise one — dropping the base method to get
// there would forfeit the other eleven.
func (b *Builder) Funcs(m template.FuncMap) *Builder {
	if b.base.funcs == nil {
		b.base.funcs = template.FuncMap{}
	}
	maps.Copy(b.base.funcs, m)
	return b
}

// Overrides replaces backend builtins by name.
//
// Not prefixed: an override is identified by the builtin it stands
// in for, so renaming it would only add a helper nothing calls.
func (b *Builder) Overrides(m template.FuncMap) *Builder {
	if b.base.overrides == nil {
		b.base.overrides = template.FuncMap{}
	}
	maps.Copy(b.base.overrides, m)
	return b
}

// Build freezes the declaration into a [Base].
//
// # Failure mode
//
// Panics on a declaration the pipeline cannot serve. Every check
// here is a mistake in a plugin's own New — not in its input — so it
// fires on the first construction in any test, before a run exists.
// See the package docs for why that is preferred to an error return.
//
//nolint:forbidigo // construction-time contract violation; see the package docs.
func (b *Builder) Build() *Base {
	if b.base.name == "" {
		panic("sdk/golang: plugin name is empty; the pipeline keys registration, " +
			"provenance and directive scoping on it")
	}
	if len(b.base.outputs) > 0 && !b.set && !b.builtin {
		panic(fmt.Sprintf("sdk/golang: plugin %q declares %d output(s) and no template "+
			"tree; the backend resolves a template per emit kind and would find none. "+
			"Call Builder.BuiltinTemplates if it renders through the backend's own "+
			"kind templates", b.base.name, len(b.base.outputs)))
	}
	if b.set && b.builtin {
		panic(fmt.Sprintf("sdk/golang: plugin %q declares both a template tree and "+
			"BuiltinTemplates; the second says there is no tree to register",
			b.base.name))
	}
	seen := map[string]struct{}{}
	for i, o := range b.base.outputs {
		if o.Suffix == "" {
			panic(fmt.Sprintf("sdk/golang: plugin %q output %d has no suffix; Layout "+
				"composes every filename from one", b.base.name, i))
		}
		if _, dup := seen[o.Tag]; dup {
			panic(fmt.Sprintf("sdk/golang: plugin %q declares output tag %q twice; "+
				"directive scoping and CLI overrides address an output by tag",
				b.base.name, o.Tag))
		}
		seen[o.Tag] = struct{}{}
	}

	if b.set {
		// The error is discarded rather than branched on: the directory
		// is a value the //go:embed directive already validated against
		// the tree, so the branch is unreachable from a test and a
		// plugin carrying one would carry a line no reader can account
		// for. A directory absent from the tree yields an empty FS,
		// which the backend reports as a plugin shipping no templates.
		sub, _ := fs.Sub(b.tree, b.dir)
		b.base.templates = sub
	}

	if !b.builtin {
		prefix := FuncPrefix(b.base.name)
		merged := golang.AllFuncMap(prefix)
		for name, fn := range b.base.funcs {
			merged[prefix+name] = fn
		}
		b.base.funcs = merged
	}

	frozen := b.base
	return &frozen
}

// Base answers the pipeline's declaration methods for a Go plugin.
// Embed it by pointer and construct it through [Builder].
//
// # Concurrency
//
// Every field is written once by [Builder.Build] and read
// afterwards, including by the backend's render pool. Nothing here
// locks, because nothing here mutates.
type Base struct {
	name       string
	version    string
	priority   sdk.Priority
	provides   []string
	requires   []string
	directives []sdk.DirectiveSchema
	outputs    []sdk.Output
	templates  fs.FS
	funcs      template.FuncMap
	overrides  template.FuncMap
}

// Name returns the plugin's stable identifier.
func (b *Base) Name() string { return b.name }

// Version returns the plugin's version identifier.
func (b *Base) Version() string { return b.version }

// Priority returns the bucket the plugin runs in.
func (b *Base) Priority() sdk.Priority { return b.priority }

// Provides returns the capability labels the plugin advertises.
func (b *Base) Provides() []string { return slices.Clone(b.provides) }

// Requires returns the capability labels the plugin depends on.
func (b *Base) Requires() []string { return slices.Clone(b.requires) }

// Directives returns the directive schemas the plugin declares.
func (b *Base) Directives() []sdk.DirectiveSchema { return slices.Clone(b.directives) }

// Outputs returns the plugin's output set for Go, and nil for any
// other language.
//
// Nil rather than an empty slice is the load-bearing part: it makes
// Layout report a missing provider rather than compose Go-shaped
// filenames for a backend that is not Go.
//
// # Allocation
//
// A copy per call. The set is a declaration the framework hands to
// callers that may sort or filter it, and the first one to reorder
// it in place would rewrite what every later caller sees — including
// Layout, which composes the primary filename from index 0.
func (b *Base) Outputs(lang string) []sdk.Output {
	if lang != golang.Language {
		return nil
	}
	return slices.Clone(b.outputs)
}

// Templates returns the embedded Go template tree, which the backend
// reads once at Build time and registers every `*.tmpl` under.
func (b *Base) Templates(lang string) (fs.FS, bool) {
	if lang != golang.Language || b.templates == nil {
		return nil, false
	}
	return b.templates, true
}

// TemplateFuncs returns the shared Go helpers plus the plugin's own,
// under the plugin's prefix.
//
// # Allocation
//
// A copy per call, for the same reason [Base.Outputs] clones: the
// backend merges the returned map into its own registry and a caller
// mutating it would rewrite this plugin's bundle for the run.
func (b *Base) TemplateFuncs(lang string) template.FuncMap {
	if lang != golang.Language {
		return nil
	}
	return maps.Clone(b.funcs)
}

// TemplateOverrides returns the backend builtins this plugin
// replaces, nil when it replaces none.
//
// Present even for a plugin that overrides nothing, because the
// capability is all-or-nothing: a plugin supplying Templates and
// TemplateFuncs without this does not satisfy
// [sdk.TemplateProvider], and the pipeline's assertion fails by
// treating it as shipping no templates at all.
func (b *Base) TemplateOverrides(lang string) template.FuncMap {
	if lang != golang.Language || len(b.overrides) == 0 {
		return nil
	}
	return maps.Clone(b.overrides)
}
