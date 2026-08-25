// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"strings"
	"text/template"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/plugin"
)

// ErrTemplateFuncCollision is returned by the funcmap-extension pass
// when a plugin registers a name that another plugin already
// registered or that collides with a core canonical entry. The
// wrapped message names the offending entry and both contributors
// so the diagnostic locates the conflict without a stack trace.
//
// Plugins use [plugin.TemplateProvider.TemplateOverrides] for
// intentional replacement of an existing entry — extensions are for
// genuinely-new names only.
var ErrTemplateFuncCollision = errors.New("backend: template func collision")

// ErrReservedFuncName is returned by the funcmap-override pass when
// a plugin attempts to override a reserved canonical entry — the
// render-dispatch, slot-composition, and import-collection helpers
// the core templates depend on. Plugins that need different slot
// semantics ship their own emit kind and template rather than
// overriding the canonical helpers.
//
// The reserved set is computed from the merged funcmap shipped by
// the backend; it grows when the backend ships additional
// canonical helpers.
var ErrReservedFuncName = errors.New("backend: reserved funcmap entry overridden")

// ErrTemplateNameCollision is returned by the template-parse pass
// when two plugins both ship a template under the same name. The
// wrapped message names both contributors and the offending
// template so the diagnostic locates the conflict without a stack
// trace.
//
// Plugin-defined emit kinds typically use a plugin-prefixed kind
// name (`sagagen.saga`, `repogen.repository`) so collisions across
// plugins are rare; this sentinel surfaces the case when two
// plugins claim the same emit kind by accident.
var ErrTemplateNameCollision = errors.New("backend: template name collision")

// ErrReservedTemplatePrefix is returned when a plugin ships a
// template whose name uses a reserved prefix. The `fragment.`
// prefix is held for shared-partial templates whether or not the
// backend currently ships any — plugin-defined emit kinds must
// pick a different namespace (typically `<pluginname>.<kind>`)
// to avoid colliding with future core partials.
var ErrReservedTemplatePrefix = errors.New("backend: reserved template-name prefix")

// reservedTemplatePrefixes lists the template-name prefixes core
// holds for its own use. Plugin templates whose `define` name
// starts with any of these prefixes are rejected during the
// per-plugin parse pass via [ErrReservedTemplatePrefix].
//
//nolint:gochecknoglobals // immutable lookup table.
var reservedTemplatePrefixes = []string{"fragment."}

// coreContributor is the marker the merge layer stamps for template
// names and funcmap entries that originate from the backend itself
// (not a plugin). It surfaces in [ErrTemplateFuncCollision] and
// override Info diagnostics ("(was: core)") so users can tell core
// content apart from plugin content in the override trail.
const coreContributor = "core"

// mergedTemplateSet holds the result of the per-Render template +
// funcmap merge: the template tree carrying every plugin's parsed
// templates alongside the core set, plus the merged plugin funcmap
// contributions (extensions + overrides combined; overrides
// override extensions and core during the per-Target Clone bind).
//
// The set is built once per Render call and passed to every Target's
// [newRenderState]; the parent template tree stays read-only.
type mergedTemplateSet struct {
	// tmpl is the parent-clone-with-plugin-templates parsed in.
	// Each Target's [renderState] clones from here.
	tmpl *template.Template

	// extensions are the plugin TemplateFuncs registered in Pass 1
	// — names guaranteed not to collide with core or each other.
	extensions template.FuncMap

	// overrides are the plugin TemplateOverrides registered in
	// Pass 2 — names guaranteed not to be in the reserved set.
	// Bind these after the core funcmap so they override the
	// non-reserved canonical entries they target.
	overrides template.FuncMap

	// importAware are the plugins whose helpers are built per
	// rendered file, against that file's import set.
	//
	// Held as the plugins rather than as their maps, because the maps
	// cannot exist yet: each is bound to a file, and no file is being
	// rendered when this set is built. Names were validated against
	// the reserved set here all the same, by invoking each factory
	// once against a registrar that records nothing — a collision
	// found at Build names the plugin, while one found at render
	// names a template line in a file the author did not write.
	importAware []importAwarePlugin
}

// importAwarePlugin pairs a plugin with the name Build-time
// diagnostics attribute its helpers to.
type importAwarePlugin struct {
	name  string
	funcs plugin.ImportAwareFuncs
}

// mergePluginContributions runs the two-pass template + funcmap
// merge. It returns a [mergedTemplateSet] suitable for per-Target
// rendering, or an error when any plugin contribution violates the
// reserved-set, cross-plugin uniqueness, or template-name-uniqueness
// rules. Successful overrides emit an Info diagnostic naming the
// plugin that won and the previous owner.
//
// The merge is per-Render rather than per-Target: every Target in a
// single Render call sees the same merged set so output stays
// deterministic and the cost of parsing plugin templates is paid
// once.
//
// The parent template tree (`parent`) is not mutated; the function
// clones it before parsing plugin templates so concurrent Render
// calls remain race-free.
func mergePluginContributions(
	ctx *plugin.BackendContext,
	parent *template.Template,
	reserved map[string]struct{},
	ps *diag.PluginSink,
) (*mergedTemplateSet, error) {
	importAware, err := collectImportAware(ctx, reserved)
	if err != nil {
		return nil, err
	}
	tmpl, err := parseTemplatesFromPlugins(ctx, parent, ps, importAwarePlaceholders(ctx, importAware))
	if err != nil {
		return nil, err
	}
	extensions, extensionOwners, err := mergeExtensions(ctx, reserved)
	if err != nil {
		return nil, err
	}
	overrides, err := mergeOverrides(ctx, reserved, extensions, extensionOwners, ps)
	if err != nil {
		return nil, err
	}
	return &mergedTemplateSet{
		tmpl:        tmpl,
		extensions:  extensions,
		overrides:   overrides,
		importAware: importAware,
	}, nil
}

// parseTemplatesFromPlugins clones parent and parses every
// [plugin.TemplateProvider.Templates] filesystem into the clone.
// Cross-plugin template-name collisions surface as
// [ErrTemplateNameCollision] wrapped with the offending template
// name and both contributors.
//
// Each plugin's filesystem walk targets *.tmpl files. Templates
// are loaded by their `define` name (the inside-template name),
// which is the dispatching key for [renderState.render]; the file
// path inside the FS is irrelevant to dispatch.
func parseTemplatesFromPlugins(
	ctx *plugin.BackendContext,
	parent *template.Template,
	ps *diag.PluginSink,
	importAware template.FuncMap,
) (*template.Template, error) {
	merged, err := parent.Clone()
	if err != nil {
		return nil, fmt.Errorf("backend/golang: clone parent template: %w", err)
	}
	// Track which plugin contributed each template name so collisions
	// surface the conflicting contributors in the diagnostic.
	contributors := map[string]string{}
	for _, name := range merged.Templates() {
		contributors[name.Name()] = coreContributor
	}
	// Replacers parse last, so a declared replacement wins whatever
	// the registration order — otherwise "who replaces whom" would be
	// decided by capability topology, which orders plugins for a
	// different reason entirely.
	for _, p := range replacersLast(ctx) {
		tp, ok := p.(plugin.TemplateProvider)
		if !ok {
			continue
		}
		fsys, has := tp.Templates(ctx.Lang)
		if !has {
			continue
		}
		pluginFuncs := template.FuncMap{}
		// Import-aware helpers are not bound until a file is rendering,
		// so the parser is given a stand-in of the right arity. Without
		// it a template calling one fails to parse — which would make
		// the whole extension point unreachable from a template, its
		// only caller.
		maps.Copy(pluginFuncs, importAware)
		maps.Copy(pluginFuncs, tp.TemplateFuncs(ctx.Lang))
		maps.Copy(pluginFuncs, tp.TemplateOverrides(ctx.Lang))
		if err := parseOnePluginFS(merged, fsys, p.Name(), pluginFuncs, contributors,
			declaredReplacements(p, ctx.Lang), ps); err != nil {
			return nil, err
		}
	}
	return merged, nil
}

// parseOnePluginFS walks the supplied filesystem for *.tmpl files,
// parses them into a plugin-private template tree, then merges the
// parsed defines into merged. The private tree isolates the
// plugin's `define` directives so we can identify the names this
// plugin actually contributes (versus names already present from
// core or earlier plugins, which text/template silently replaces
// on Parse).
//
// Two collision rules apply:
//
//   - Plugin templates may override core templates (the canonical
//     extension story); the override is recorded silently.
//   - Two plugins both defining the same template name surface as
//     [ErrTemplateNameCollision] wrapped with the offending name
//     and both contributors.
func parseOnePluginFS(
	merged *template.Template,
	fsys fs.FS,
	pluginName string,
	pluginFuncs template.FuncMap,
	contributors map[string]string,
	replaces map[string]bool,
	ps *diag.PluginSink,
) error {
	tmplFiles, err := collectTmplFiles(fsys)
	if err != nil {
		return fmt.Errorf("backend/golang: walk plugin %q templates: %w", pluginName, err)
	}
	// Bind the plugin's own funcmap entries alongside the core
	// placeholders so the plugin's templates can reference its
	// extensions / overrides at parse time. Real bindings are
	// supplied per-Target via [newRenderState]; the parse step
	// only checks name and arity.
	private := template.New(pluginName).Funcs(parsePlaceholders).Funcs(pluginFuncs)
	for _, tf := range tmplFiles {
		body, readErr := fs.ReadFile(fsys, tf)
		if readErr != nil {
			return fmt.Errorf("backend/golang: read plugin %q template %s: %w", pluginName, tf, readErr)
		}
		if _, parseErr := private.New(tf).Parse(string(body)); parseErr != nil {
			return fmt.Errorf("backend/golang: parse plugin %q template %s: %w", pluginName, tf, parseErr)
		}
	}
	for _, t := range private.Templates() {
		name := t.Name()
		if name == pluginName {
			continue
		}
		for _, prefix := range reservedTemplatePrefixes {
			if strings.HasPrefix(name, prefix) {
				return fmt.Errorf(
					"%w: plugin %s defines %q (prefix %q is reserved)",
					ErrReservedTemplatePrefix, pluginName, name, prefix,
				)
			}
		}
		prior, exists := contributors[name]
		if exists && prior != coreContributor && prior != pluginName && !replaces[name] {
			return fmt.Errorf(
				"%w: %q defined by %s and %s; declare it in the plugin's "+
					"Replaces list to supersede deliberately",
				ErrTemplateNameCollision, name, prior, pluginName,
			)
		}
		if _, addErr := merged.AddParseTree(name, t.Tree); addErr != nil {
			return fmt.Errorf(
				"backend/golang: add plugin %q template %q: %w",
				pluginName, name, addErr,
			)
		}
		if exists && ps != nil {
			ps.Infof(position.Pos{}, "plugin %s overrode template %s (was: %s)", pluginName, name, prior)
		}
		contributors[name] = pluginName
	}
	return nil
}

// collectTmplFiles walks fsys depth-first and returns every file
// path with a ".tmpl" extension. Directory entries are skipped.
func collectTmplFiles(fsys fs.FS) ([]string, error) {
	var out []string
	if err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || path.Ext(p) != ".tmpl" {
			return nil
		}
		out = append(out, p)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("backend/golang: walk template fs: %w", err)
	}
	return out, nil
}

// mergeExtensions walks ctx.Plugins and collects every
// [plugin.TemplateProvider.TemplateFuncs] entry. Names that collide
// with the reserved set OR with another plugin's extension fail
// with [ErrTemplateFuncCollision] wrapping the offending name and
// both contributors. The returned owners map records the plugin
// name that contributed each extension — used by
// [mergeOverrides] to attribute the "(was: …)" surface of override
// Info diagnostics.
func mergeExtensions(
	ctx *plugin.BackendContext,
	reserved map[string]struct{},
) (template.FuncMap, map[string]string, error) {
	out := template.FuncMap{}
	owners := map[string]string{}
	for _, p := range ctx.Plugins {
		tp, ok := p.(plugin.TemplateProvider)
		if !ok {
			continue
		}
		funcs := tp.TemplateFuncs(ctx.Lang)
		for name, fn := range funcs {
			if _, reservedHit := reserved[name]; reservedHit {
				return nil, nil, fmt.Errorf(
					"%w: extension %q from plugin %s collides with core canonical entry",
					ErrTemplateFuncCollision, name, p.Name(),
				)
			}
			if prior, exists := owners[name]; exists {
				return nil, nil, fmt.Errorf(
					"%w: extension %q registered by both plugin %s and plugin %s",
					ErrTemplateFuncCollision, name, prior, p.Name(),
				)
			}
			out[name] = fn
			owners[name] = p.Name()
		}
	}
	return out, owners, nil
}

// mergeOverrides walks ctx.Ordered (capability-topo order) and
// collects every [plugin.TemplateProvider.TemplateOverrides]
// entry. Names in the reserved set fail with [ErrReservedFuncName].
// Each successful override emits an Info diagnostic naming the
// winning plugin and the previous owner so users can audit the
// override trail (`plugin <id> overrode <name> (was: <previous>)`).
//
// Overrides apply to entries that are NOT reserved — that is,
// either plugin extensions registered in Pass 1 or non-reserved
// canonical helpers core ships under the overrideable bucket. The
// "previous owner" the Info diagnostic cites is the prior plugin
// that registered or overrode the same name; "core" when no plugin
// touched it yet.
func mergeOverrides(
	ctx *plugin.BackendContext,
	reserved map[string]struct{},
	extensions template.FuncMap,
	extensionOwners map[string]string,
	ps *diag.PluginSink,
) (template.FuncMap, error) {
	out := template.FuncMap{}
	// previous tracks the most-recent contributor per name. Names
	// registered as plugin extensions in Pass 1 seed with their
	// plugin owner; names not yet touched register as "core".
	previous := map[string]string{}
	for name := range extensions {
		previous[name] = extensionOwners[name]
	}
	for _, p := range ctx.Ordered {
		tp, ok := p.(plugin.TemplateProvider)
		if !ok {
			continue
		}
		overrides := tp.TemplateOverrides(ctx.Lang)
		for name, fn := range overrides {
			if _, reservedHit := reserved[name]; reservedHit {
				return nil, fmt.Errorf(
					"%w: plugin %s tried to override %q",
					ErrReservedFuncName, p.Name(), name,
				)
			}
			out[name] = fn
			prior, exists := previous[name]
			if !exists {
				prior = coreContributor
			}
			if ps != nil {
				ps.Infof(position.Pos{}, "plugin %s overrode %s (was: %s)", p.Name(), name, prior)
			}
			previous[name] = p.Name()
		}
	}
	return out, nil
}

// collectImportAware gathers the plugins whose helpers bind to the
// file being rendered, validating their names against the reserved
// set.
//
// Validation invokes each factory once against [discardRegistrar],
// which registers nothing. A factory is expected to return the same
// names whatever registrar it is given — the registrar decides how a
// reference is spelled, not which helpers exist — so one probe call
// settles the question for every file.
func collectImportAware(
	ctx *plugin.BackendContext, reserved map[string]struct{},
) ([]importAwarePlugin, error) {
	var out []importAwarePlugin
	for _, p := range ctx.Ordered {
		aware, ok := p.(plugin.ImportAwareFuncs)
		if !ok {
			continue
		}
		for name := range aware.TemplateFuncsFor(ctx.Lang, discardRegistrar{}) {
			if _, hit := reserved[name]; hit {
				return nil, fmt.Errorf(
					"%w: plugin %s tried to override %q through an import-aware helper",
					ErrReservedFuncName, p.Name(), name,
				)
			}
		}
		out = append(out, importAwarePlugin{name: p.Name(), funcs: aware})
	}
	return out, nil
}

// discardRegistrar answers the probe call that validates helper
// names, recording no import.
//
// The qualifier it returns is never rendered: the map the probe
// produces is thrown away, and only its keys are read.
type discardRegistrar struct{}

func (discardRegistrar) Import(string) (string, error) { return "", nil }

// importAwarePlaceholders returns a parse-time stand-in for every
// import-aware helper name.
//
// text/template resolves function names at parse time and binds them
// at execution, so a name absent from the parse funcmap is a parse
// error however it is bound later. The stand-in is invoked only if
// something executes a template without the real binding, which is a
// backend bug rather than a plugin one — hence the error rather than
// a value.
func importAwarePlaceholders(
	ctx *plugin.BackendContext, plugins []importAwarePlugin,
) template.FuncMap {
	out := template.FuncMap{}
	for _, p := range plugins {
		for name := range p.funcs.TemplateFuncsFor(ctx.Lang, discardRegistrar{}) {
			out[name] = func() (string, error) { return "", errPlaceholderInvoked }
		}
	}
	return out
}

// replacersLast orders plugins so every declared template
// replacement is parsed after the templates it replaces.
//
// text/template's last-write-wins is what makes a replacement take
// effect, so the order is the mechanism rather than a tidiness. A
// plugin declaring no replacement keeps its position relative to its
// peers, which keeps the diagnostics a collision produces stable.
func replacersLast(ctx *plugin.BackendContext) []plugin.Plugin {
	var plain, replacers []plugin.Plugin
	for _, p := range ctx.Plugins {
		if r, ok := p.(plugin.TemplateReplacer); ok && len(r.ReplacesTemplates(ctx.Lang)) > 0 {
			replacers = append(replacers, p)
			continue
		}
		plain = append(plain, p)
	}
	return append(plain, replacers...)
}

// declaredReplacements returns the template names p supersedes.
func declaredReplacements(p plugin.Plugin, lang string) map[string]bool {
	r, ok := p.(plugin.TemplateReplacer)
	if !ok {
		return nil
	}
	out := map[string]bool{}
	for _, name := range r.ReplacesTemplates(lang) {
		out[name] = true
	}
	return out
}
