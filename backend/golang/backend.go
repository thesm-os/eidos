// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"text/template"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
	"go.thesmos.sh/eidos/writer"
)

// Name is the stable plugin identifier the pipeline uses for
// registration, diagnostic attribution, and cache-key derivation.
// Plugin authors reference it when supplying backend-specific
// options via [pipeline.Builder.WithPluginOptions].
const Name = "backend.golang"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

// collectBridgeImports walks every source-side [node.Package] in
// s and records the pairs where the cross-language bridge stamped
// `go.import` meta on the package. The returned map is keyed by
// the source package's Path (the source-language qualifier — proto
// qualifier for proto-input pipelines, the Go import path for
// Go-input pipelines). Go-source packages stamp nothing, so the
// map stays empty for Go-only pipelines and the render-site
// translation collapses to a no-op.
//
// The walk runs once per [Backend.Render] call. Per-Target render
// states share the resulting map; concurrent reads are safe by
// virtue of the map's post-build immutability.
func collectBridgeImports(s *store.Store) map[string]string {
	out := map[string]string{}
	s.Nodes().Packages().Range(func(p *node.Package) bool {
		got, ok := goImportKey.Get(p.Meta())
		if !ok || got == "" {
			return true
		}
		out[p.Path] = got
		return true
	})
	return out
}

// Language is the target-language identifier shared between the
// backend, plugin-supplied template providers, and downstream
// tooling. Plugin authors target this exact string when
// implementing [plugin.TemplateProvider.Templates].
const Language = "golang"

// Backend renders emit graphs to Go source. The zero value is
// unusable; construct via [New]. A Backend instance is safe for
// concurrent use — [Backend.Render] holds no state across calls
// and dispatches per-target work through the supplied
// [plugin.BackendContext].
type Backend struct {
	tmpl *template.Template
}

// New returns a Backend ready for registration on a pipeline via
// [pipeline.Builder.WithBackend].
func New() *Backend {
	return &Backend{tmpl: loadTemplates()}
}

// Name reports the stable plugin identifier — returns [Name].
func (*Backend) Name() string { return Name }

// Version satisfies [plugin.Versioned].
func (*Backend) Version() string { return Version }

// Language reports the target-language identifier — returns
// [Language].
func (*Backend) Language() string { return Language }

// Render groups emit entities by their [emit.Target] and writes one
// gofmt-clean file per non-empty Target through ctx.Sink. The
// per-Target pipeline executes the merged template set against the
// entities sharing that Target, composes the file's package decl
// and resolved import block, runs the result through
// [go/format.Source] and the goimports library pass for canonical
// formatting, then writes the finalised bytes.
//
// Zero-valued Targets are filtered upstream by [store.EmitView]'s
// by-target index and never reach the loop. Template execution
// failures surface as Error diagnostics on ctx.Diag for the
// offending Target and the loop continues. Format and goimports
// failures surface as Warn diagnostics; the loop still writes
// whatever bytes are available so the user can debug. Sink write
// failures propagate as a wrapped error — they indicate I/O or
// backend-side faults rather than content defects.
func (b *Backend) Render(ctx *plugin.BackendContext) error {
	ps := ctx.Diag.For(Name)
	merged, err := mergePluginContributions(ctx, b.tmpl, reservedFuncNames(), ps)
	if err != nil {
		ps.Errorf(position.Pos{}, "%v", err)
		return nil
	}
	pluginOrder := pluginOrderFrom(ctx)
	bridgeImports := collectBridgeImports(ctx.Store)
	selfPackages := collectSelfPackages(ctx.Store)
	for _, target := range ctx.Store.Emit().ByTarget().Keys() {
		entities := ctx.Store.Emit().ByTarget().Get(target)
		state := newRenderState(merged.tmpl, pluginOrder, merged.extensions, merged.overrides)
		state.bridgeImports = bridgeImports
		state.selfPackages = selfPackages
		// Forward the target's own import path + short name to the
		// per-file import set. The path enables same-package
		// elision ([emit.ExternalRef] / [emit.ExprExternal]
		// references back into the same package render bare); the
		// short name is reserved in the alias collision table so a
		// cross-package import whose derived alias would shadow the
		// file's own `package <name>` clause falls back to a
		// numeric-suffixed alias.
		//
		// Cross-language frontends produce Target.ImportPath in the
		// source language's qualifier form (proto's
		// `eidos.test.buildfixture`); the same translation the
		// ExternalRef render-site applies runs here so a self-
		// reference under a bridge-stamped source package still
		// elides correctly against the rendered file's Go-canonical
		// import path.
		selfPath := state.resolveImportPath(target.ImportPath)
		state.imports.SetSelf(selfPath, target.Package)
		// Then pin the alias for every *other* output package whose
		// declared name diverges from its directory, so a cross-package
		// reference into a `pkg=`-renamed output resolves.
		state.applySelfAliases(selfPath)
		body, tracked, err := renderFile(state, target, entities, packageDocsFor(ctx, target))
		if err != nil {
			if errors.Is(err, ErrEmptyTarget) {
				continue
			}
			ps.Errorf(position.Pos{}, "%s: %v", target.JoinPath(), err)
			continue
		}
		body = finaliseBody(body, target, ps, tracked)
		out := composeFile(ctx, entities, body)
		if err := ctx.Sink.Write(target, out); err != nil {
			return fmt.Errorf("%s: sink write %s: %w", Name, target.JoinPath(), err)
		}
	}
	return nil
}

// collectSelfPackages maps each import path the run writes into to
// the package name that output declares, for every resolved Target.
//
// The pair is only interesting where the two disagree — see
// [renderState.selfPackages] — but collecting all of them keeps the
// filter in one place, at the point of use.
//
// Targets sharing an import path agree on their package name in any
// run that passes the layout phase's one-file-one-package check, so
// last-write-wins is not observable.
func collectSelfPackages(s *store.Store) map[string]string {
	out := map[string]string{}
	for _, t := range s.Emit().ByTarget().Keys() {
		if t.ImportPath == "" || t.Package == "" {
			continue
		}
		out[t.ImportPath] = t.Package
	}
	return out
}

// composeFile wraps the finalised body bytes in the canonical
// header / footer envelope: the run-context header (layout item 1),
// the body (items 2–8), and the provenance-hash footer (item 9).
// The hash is computed over the body bytes exactly as written, so
// two runs over the same input produce byte-identical files —
// header and footer included, given the header carries no
// timestamp.
func composeFile(ctx *plugin.BackendContext, entities []emit.Node, body []byte) []byte {
	header := renderHeader(ctx, entities)
	footer := renderFooter(ctx, body)
	out := make([]byte, 0, len(header)+len(body)+len(footer))
	out = append(out, header...)
	out = append(out, body...)
	out = append(out, footer...)
	return out
}

// packageDocsFor returns the doc-comment lines rendered above the
// `package <Name>` clause of target's output file. The source is
// the [emit.Package] entity whose name matches target.Package; its
// DocLines apply to every file in that package. Returns nil when
// no matching package carries DocLines — [renderDocs] renders the
// empty slice as the empty string, so the absent package doc
// introduces no whitespace.
func packageDocsFor(ctx *plugin.BackendContext, target emit.Target) []string {
	if pkg, ok := ctx.Store.Emit().Packages().ByQName(target.Package); ok {
		return pkg.DocLines
	}
	return nil
}

// renderFile produces the raw rendered body for one Target. The
// composition follows the canonical layout items 2 through 8:
//
//   - Pre-render pass: every entry in [emit.File.Imports] and
//     [emit.File.ImportsSlot] registers with the per-Target
//     [writer.ImportSet] so plugin-supplied imports flow through
//     the authoritative deduper alongside template-driven `imp`
//     calls.
//   - Render the layout-item-5 "top" slot (File.Top), the
//     layout-item-6 free-floating decls (each non-File entity), the
//     layout-item-7 init block (File.Init composed into a single
//     `func init() { … }`), and the layout-item-8 "bottom" slot
//     (File.Bottom), each through the kind-template dispatcher.
//   - Compose the body as
//     "<package-doc>" + "package <Name>" + import block + top +
//     decls + init + bottom.
//
// Returns [ErrEmptyTarget] when the Target carries no decls and
// every File slot is empty — the caller skips silently rather than
// producing an empty sink write.
//
// The returned bytes are unformatted — [finaliseBody] runs them
// through [go/format.Source] and the goimports library pass. The
// tracked-imports slice carries every recorded import so the
// finalisation pass can flag any path goimports added beyond what
// the templates declared.
func renderFile(
	state *renderState,
	target emit.Target,
	entities []emit.Node,
	packageDocs []string,
) ([]byte, []writer.Import, error) {
	file := fileFor(entities, target)
	if err := state.preRenderImports(file); err != nil {
		return nil, nil, err
	}

	decls := declEntities(entities)
	var declsBuf bytes.Buffer
	for _, n := range decls {
		rendered, err := state.render(n)
		if err != nil {
			return nil, nil, err
		}
		declsBuf.WriteString(rendered)
		declsBuf.WriteString("\n\n")
	}

	top, initBlock, bottom, err := state.renderFileSlots(file)
	if err != nil {
		return nil, nil, err
	}

	if len(decls) == 0 && top == "" && initBlock == "" && bottom == "" {
		return nil, nil, ErrEmptyTarget
	}

	tracked := state.imports.Imports()
	var fileBuf bytes.Buffer
	fileBuf.WriteString(renderDocs(packageDocs))
	fmt.Fprintf(&fileBuf, "package %s\n", target.Package)
	writeImportBlock(&fileBuf, tracked)
	fileBuf.WriteByte('\n')
	if top != "" {
		fileBuf.WriteString(top)
	}
	fileBuf.Write(declsBuf.Bytes())
	if initBlock != "" {
		fileBuf.WriteString(initBlock)
		fileBuf.WriteByte('\n')
	}
	if bottom != "" {
		fileBuf.WriteString(bottom)
	}

	return fileBuf.Bytes(), tracked, nil
}

// writeImportBlock emits the canonical `import ( … )` block for
// every import in imports. Empty slices emit nothing. Aliases
// matching the default-derived alias for their path are omitted
// from the line; explicit aliases (or collision-resolved variants
// like "context2") render as `<alias> "<path>"`. The goimports
// post-pass regroups stdlib vs external imports per Go convention.
func writeImportBlock(buf *bytes.Buffer, imports []writer.Import) {
	if len(imports) == 0 {
		return
	}
	buf.WriteString("\nimport (\n")
	for _, imp := range imports {
		buf.WriteByte('\t')
		if imp.Alias != writer.DefaultAlias(imp.Path) {
			buf.WriteString(imp.Alias)
			buf.WriteByte(' ')
		}
		buf.WriteString(strconv.Quote(imp.Path))
		buf.WriteByte('\n')
	}
	buf.WriteString(")\n")
}
