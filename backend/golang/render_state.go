// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"sync"
	"text/template"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/writer"
)

// ErrTemplateMissing is returned by render-dispatch helpers when no
// template is registered for an entity's [emit.Node.Kind]. The
// wrapping error names the offending kind so the diagnostic is
// actionable without a stack trace.
var ErrTemplateMissing = errors.New("golang: no template registered for kind")

// renderState carries the per-Target rendering context: a cloned
// template tree with funcmap closures bound to this state, a fresh
// [writer.ImportSet] that accumulates every `imp` call the
// templates make during render, and the plugin topo order that
// drives slot-contribution sequencing.
//
// The clone-per-Target pattern is the package's foundational
// race-freedom property. Targets render independently — concurrent
// dispatch is safe by construction:
//
//   - The parent template tree (parsed once via [loadTemplates]) is
//     immutable from the moment construction completes; it carries
//     parse-time placeholder funcs so the parser's name-and-arity
//     validation succeeds.
//   - Each Target render gets its own [template.Template.Clone] of
//     the parent tree. Clones share the parse tree (cheap) but own
//     their funcmap namespace.
//   - The funcmap attached to the clone is built from this state
//     struct; the closures bind `s` rather than reading any global,
//     so per-Target import tracking is naturally isolated.
//
// This is the extension point plugin-template merge (later phases)
// builds on: plugin-supplied templates and funcmap entries layer on
// top of the per-Target clone, never the parent. The parent stays
// the deterministic, immutable canonical state.
type renderState struct {
	tmpl    *template.Template
	imports *writer.ImportSet

	// pluginOrder lists plugin names in capability-topo order — the
	// resolved sequence the pipeline produces via
	// [plugin.BackendContext.Ordered]. Slot contributions are
	// rendered grouped by this order so the same input emits the
	// same output across runs. Empty when no plugins participated
	// (typical for direct backend tests); contributions then render
	// in raw append order.
	pluginOrder []string

	// bridgeImports maps each source-side path that carries a
	// bridge-stamped `go.import` value to the Go import path the
	// bridge composed. Cross-language frontends (the protobuf
	// frontend, future variants) populate node.Package.Path with the
	// source language's qualifier — `eidos.test.buildfixture` for
	// proto — which isn't a valid Go import path. Reference plugins
	// thread that qualifier through emit.ExternalRef.Package
	// verbatim; the renderer translates here so the rendered import
	// block and same-package elision both read the Go form. The map
	// is populated once at [Backend.Render] entry from the source
	// store; Go-source pipelines see an empty map and the per-ref
	// translation collapses to a no-op.
	bridgeImports map[string]string

	// selfPackages maps each import path this run writes output into
	// to the package name that output declares. Populated once at
	// [Backend.Render] entry from the run's resolved Targets.
	//
	// Go binds an unaliased import to the *declared package name*,
	// not to the last segment of the path, but
	// [writer.DefaultAlias] can only guess from the path. The two
	// agree for the overwhelming majority of packages and diverge
	// exactly when a `pkg=` override renames an output away from its
	// directory — `out=testkit/ pkg=storetest` produces a directory
	// `testkit` holding `package storetest`. Without this map a
	// reference into that package renders `testkit.Thing` against an
	// import that actually bound `storetest`, and the generated file
	// does not compile.
	//
	// Only the run's own outputs are covered. Third-party packages
	// whose name diverges from their directory stay on the derived
	// alias, because nothing in the emit graph knows their package
	// clause. Nothing corrects that afterwards any more — the
	// resolve pass that used to is gone — so a plugin referencing
	// such a package must supply the alias itself, through
	// [emit.Import.Alias] or a `imp`-side registration.
	selfPackages map[string]string
}

// applySelfAliases pre-registers an explicit import alias for every
// output package in the run whose declared name differs from the
// alias [writer.DefaultAlias] would derive from its path.
//
// Registration has to happen before any template runs:
// [writer.ImportSet.Alias] rejects an override for a path already
// imported ([writer.ErrAliasAfterImp]), so a lazy fix at the first
// reference would arrive too late for the file that needed it.
//
// selfPath is the rendering file's own import path, which is skipped
// — it is handled by [writer.ImportSet.SetSelf], which both elides
// self-references and reserves the file's own package identifier
// against collision.
func (s *renderState) applySelfAliases(selfPath string) {
	for path, pkg := range s.selfPackages {
		if path == "" || pkg == "" || path == selfPath {
			continue
		}
		// Skipping the agreeing case is not required for correct
		// output — the writer omits an alias that matches the
		// derived one. It keeps the intervention to the packages
		// that need it, so an explicit registration never perturbs
		// collision resolution for an import that was already fine.
		if writer.DefaultAlias(path) == pkg {
			continue
		}
		// The only error is ErrAliasAfterImp, unreachable here: this
		// runs before template execution, so nothing has imported yet.
		_ = s.imports.Alias(path, pkg)
	}
}

// resolveImportPath returns the Go-canonical import path for a
// source-language path. When path matches a source package whose
// bridge meta stamped `go.import`, the helper returns that Go
// path. Otherwise path passes through verbatim. Centralising the
// lookup at one site lets callers (SetSelf, [renderState.renderType])
// translate consistently without re-implementing the bridge-meta
// read.
func (s *renderState) resolveImportPath(path string) string {
	if path == "" {
		return ""
	}
	if mapped, ok := s.bridgeImports[path]; ok && mapped != "" {
		return mapped
	}
	return path
}

// newRenderState clones root, attaches a fresh funcmap whose
// closures bind the returned state, layers any plugin-supplied
// funcmap extensions and overrides on top, and returns the ready-
// to-execute rendering context for one Target. pluginOrder carries
// the resolved capability-topo plugin sequence used to order slot
// contributions. extensions and overrides are the per-Render
// merged plugin contributions (see [mergePluginContributions]):
// extensions never collide with core or each other; overrides
// replace the non-reserved canonical entries they target. Pass
// nil for either when running without plugin contributions
// (typical for direct backend tests).
func newRenderState(
	root *template.Template,
	pluginOrder []string,
	extensions template.FuncMap,
	overrides template.FuncMap,
) *renderState {
	clone := template.Must(root.Clone())
	s := &renderState{
		tmpl:        clone,
		imports:     writer.NewImportSet(nil),
		pluginOrder: pluginOrder,
	}
	fm := s.funcMap()
	maps.Copy(fm, extensions)
	maps.Copy(fm, overrides)
	clone.Funcs(fm)
	return s
}

// reservedFuncNames returns the set of canonical funcmap entry
// names the backend ships — the dispatch, slot-composition, and
// import-collection helpers core templates depend on. The set is
// computed once (lazily) from a probe renderState's funcMap so the
// reserved set tracks the funcmap's actual contents without a
// separate hardcoded list. Repeat calls share the cached set —
// safe for concurrent use.
//
// Plugin extensions ([plugin.TemplateProvider.TemplateFuncs]) may
// not use any of these names; plugin overrides
// ([plugin.TemplateProvider.TemplateOverrides]) likewise may not
// target these names. Both rules surface as Build-time
// diagnostics — [ErrTemplateFuncCollision] and
// [ErrReservedFuncName] respectively.
var reservedFuncNames = sync.OnceValue(func() map[string]struct{} {
	probe := &renderState{imports: writer.NewImportSet(nil)}
	fm := probe.coreFuncMap()
	out := make(map[string]struct{}, len(fm))
	for name := range fm {
		out[name] = struct{}{}
	}
	return out
})

// funcMap returns the canonical core funcmap with every closure
// bound to s. The reserved-name set surfaced here covers the
// render-dispatch and import-collection families; plugin overrides
// for these names are rejected at Build time.
func (s *renderState) funcMap() template.FuncMap {
	fm := s.coreFuncMap()
	maps.Copy(fm, extrasFuncMap())
	return fm
}

// coreFuncMap returns just the reserved canonical funcmap entries —
// the dispatch, canonical-render, slot-composition, collision-
// handling, and canonical-metadata categories. Plugin overrides
// targeting these names fail at Build with [ErrReservedFuncName].
// Overrideable entries (Naming, Meta-read, String, Debug) ride on
// [extrasFuncMap] and are excluded from the reserved set so
// plugins may replace them.
func (s *renderState) coreFuncMap() template.FuncMap {
	return template.FuncMap{
		"render":                 s.render,
		"renderType":             s.renderType,
		"renderDocs":             renderDocs,
		"renderTypeParams":       s.renderTypeParams,
		"renderParams":           s.renderParams,
		"renderReceiver":         s.renderReceiver,
		"renderReturns":          s.renderReturns,
		"renderExpr":             s.renderExpr,
		"renderStmt":             s.renderStmt,
		"renderStructFields":     s.renderStructFields,
		"renderStructEmbeds":     s.renderStructEmbeds,
		"renderStructMethods":    s.renderStructMethods,
		"renderInterfaceEmbeds":  s.renderInterfaceEmbeds,
		"renderInterfaceMethods": s.renderInterfaceMethods,
		"renderEnumVariants":     s.renderEnumVariants,
		"renderFunctionBody":     s.renderFunctionBody,
		"renderMethodBody":       s.renderMethodBody,
		"renderFunctionParams":   s.renderFunctionParams,
		"renderMethodParams":     s.renderMethodParams,
		"renderFunctionReturns":  s.renderFunctionReturns,
		"renderMethodReturns":    s.renderMethodReturns,
		"imp":                    s.imp,
		"external":               emit.NewExternal,
		"slot":                   slot,
		"provenance":             provenance,
	}
}

// imp is the template-facing import helper — `{{ imp "path" }}`
// registers path with the file's import set and returns the
// qualifier to spell references with.
//
// It exists as a method rather than as the method value
// `s.imports.Imp` because Go binds a method value's receiver when
// the value is created, not when it is called. [renderTarget]
// replaces s.imports once per target, so a value bound at
// funcmap-construction time would keep writing into the set the
// state has already moved on from and nothing the template declared
// would reach the file. A method on s dereferences the field at
// call time, which is the invariant every other entry in this map
// already satisfies.
func (s *renderState) imp(path string) (string, error) {
	return s.imports.Imp(path)
}

// render executes the template named after n's [emit.Node.Kind] and
// returns the rendered text inline. Returns [ErrTemplateMissing]
// wrapped with the kind when no template is registered.
func (s *renderState) render(n emit.Node) (string, error) {
	kind := string(n.Kind())
	if s.tmpl.Lookup(kind) == nil {
		return "", fmt.Errorf("%w: %s", ErrTemplateMissing, kind)
	}
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, kind, n); err != nil {
		return "", fmt.Errorf("backend/golang: render %s: %w", kind, err)
	}
	return buf.String(), nil
}
