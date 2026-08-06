// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"go.thesmos.sh/eidos/core/diag"
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
	selfAliases := collectSelfPackages(ctx.Store)

	keys := ctx.Store.Emit().ByTarget().Keys()
	results := make([]renderResult, len(keys))
	renderTargets(keys, results,
		func() *renderState {
			st := newRenderState(merged.tmpl, pluginOrder, merged.extensions, merged.overrides)
			st.bridgeImports = bridgeImports
			st.selfAliases = selfAliases
			return st
		},
		func(st *renderState, i int) renderResult {
			return renderTarget(ctx, st, keys[i])
		},
	)

	// The package union is the first thing this loop needs and the
	// last thing the render pass can supply: a qualifier is only
	// unresolved once every target that might declare it has
	// rendered.
	declared := unionTopLevel(keys, results)

	// Replay and write in key order. Rendering is what parallelises;
	// the observable sequence stays exactly what the sequential loop
	// produced, so Stdout ordering, diagnostic ordering and the
	// manifest are all unchanged by concurrency.
	for i, res := range results {
		for _, d := range res.diags {
			ctx.Diag.Append(d)
		}
		if res.skip {
			continue
		}
		for _, q := range unresolvedAfterPackage(res.candidates, declared[keyFor(keys[i])]) {
			ps.Warnf(position.Pos{},
				"%s: unresolved qualifier %q: no import binds it, the generated file will not compile",
				keys[i].JoinPath(), q)
		}
		if err := ctx.Sink.Write(keys[i], res.out); err != nil {
			return fmt.Errorf("%s: sink write %s: %w", Name, keys[i].JoinPath(), err)
		}
	}
	return nil
}

// renderResult carries one target's rendered bytes and the
// diagnostics its render produced, so both can be replayed in key
// order after a concurrent render pass.
type renderResult struct {
	// out is the finalised file content, valid only when skip is
	// false.
	out []byte

	// diags are the diagnostics this target's render emitted, held
	// rather than written straight through so their order does not
	// depend on which goroutine finished first.
	diags []diag.Diag

	// skip marks a target that produced no renderable content
	// ([ErrEmptyTarget]) or whose render failed. Either way it
	// reaches no sink; a failure has already recorded its
	// diagnostic.
	skip bool

	// candidates are the qualifiers this target's body names that
	// no import binds and the file itself does not declare, sorted.
	// Not yet a verdict — a sibling target in the same package may
	// declare the name, which [Backend.Render] subtracts before
	// reporting.
	candidates []string

	// topLevel are the package-scope names this target declares.
	// Held per result rather than merged during render because the
	// merge spans targets and the render pass is concurrent.
	topLevel map[string]struct{}
}

// renderTargets fills results by invoking render for every index,
// bounded to one worker per CPU.
//
// The work is embarrassingly parallel: each target owns a template
// clone, a funcmap of closures bound to its own render state, and a
// fresh [writer.ImportSet]. Nothing crosses between them, which is
// the property backend/golang's package doc has always claimed and
// this dispatches on.
//
// Bounded rather than one goroutine per target because a clone is
// not free — a thousand-target workspace would hold a thousand
// template trees at once for no throughput gain past the core count.
//
// Writes into results are by distinct index into a pre-sized slice,
// so no synchronisation is needed on the slice itself.
//
// newState is called once per worker, not once per target. A
// [renderState] costs a full template clone plus a funcmap of bound
// closures — together the largest allocation in the render path, at
// 27% of everything the backend allocates — and a worker handles its
// targets one at a time, so one state per worker is all the
// isolation the design needs. [renderTarget] resets the only
// per-target field on it.
func renderTargets(
	keys []emit.Target,
	results []renderResult,
	newState func() *renderState,
	render func(*renderState, int) renderResult,
) {
	if len(keys) == 0 {
		return
	}
	workers := min(runtime.GOMAXPROCS(0), len(keys))
	if workers <= 1 {
		st := newState()
		for i := range keys {
			results[i] = render(st, i)
		}
		return
	}
	idx := make(chan int)
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			st := newState()
			for i := range idx {
				results[i] = render(st, i)
			}
		}()
	}
	for i := range keys {
		idx <- i
	}
	close(idx)
	wg.Wait()
}

// renderTarget renders one target to finalised bytes, collecting its
// diagnostics into a private sink so the caller can replay them in
// key order.
//
// The private sink is what keeps diagnostics deterministic under
// concurrency: writing straight to ctx.Diag would order them by
// whichever goroutine finished first, and `-diag-format json` makes
// that observable.
func renderTarget(
	ctx *plugin.BackendContext,
	state *renderState,
	target emit.Target,
) (res renderResult) {
	local := diag.New()
	ps := local.For(Name)
	res = renderResult{skip: true}
	// Named return: the deferred collection must land on the value
	// the caller receives, not on a copy already taken.
	defer func() { res.diags = local.Diagnostics() }()

	// The import set is the only per-target state the render state
	// carries — the template clone, the bound funcmap, and the
	// bridge / self-package maps are per-worker or run-wide.
	// Replacing it here needs no rebinding, and leaks nothing from
	// the worker's previous target, for exactly one reason: no
	// funcmap entry may capture s.imports. Every entry is bound as
	// `s.<method>`, which resolves the field when the template
	// calls it. Binding `s.imports.<method>` instead would freeze
	// the pointer at funcmap-construction time and silently orphan
	// everything the templates write — see [renderState.imp], which
	// exists to keep that invariant true for the one entry whose
	// work is the import set's rather than the state's.
	//
	// Cleared in place rather than reallocated: the set is
	// per-worker and the previous target's contents are dead by the
	// time this runs. Note that this makes the identity stable,
	// which would make a captured-pointer funcmap binding
	// accidentally work — the reason the invariant above is stated
	// as a rule rather than left to the reset's shape.
	state.imports.Reset()

	entities := ctx.Store.Emit().ByTarget().Get(target)
	{
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
		body, tracked, refs, err := renderFile(state, target, entities, packageDocsFor(ctx, target))
		if err != nil {
			if errors.Is(err, ErrEmptyTarget) {
				return res
			}
			ps.Errorf(position.Pos{}, "%s: %v", target.JoinPath(), err)
			return res
		}
		// An import whose alias is also a local name survives the
		// prune on that local's own selectors. Naming it is the only
		// signal the run gives: the check declines to drop the
		// import, so the file reaches the compiler with an "imported
		// and not used" error and nothing pointing back here.
		for _, imp := range shadowedImports(tracked, refs) {
			ps.Warnf(position.Pos{}, "%s: import %q is unused but kept: %q is also declared in this file",
				target.JoinPath(), imp.Path, imp.Alias)
		}
		res.candidates = unresolvedCandidates(refs, tracked)
		res.topLevel = refs.topLevel
		body = finaliseBody(body, target, ps)
		res.out = composeFile(ctx, entities, body)
		res.skip = false
	}
	return res
}

// selfAlias is one output package whose declared name diverges from
// the alias its import path derives to, and therefore needs an
// explicit alias registered on any file that imports it.
type selfAlias struct {
	path string
	pkg  string
}

// collectSelfPackages returns the run's own output packages whose
// declared name diverges from their path-derived alias.
//
// The divergence test used to live in the per-target loop, which ran
// it for every (target, package) pair and continued on all of them:
// a run with T targets across P output packages did T×P alias
// derivations to perform, normally, zero registrations. That is the
// only quadratic in the render path, and it is invisible unless a
// benchmark varies package count independently of target count.
//
// Filtering here is safe because the test is target-independent — a
// package's name either matches its derived alias or it does not,
// whatever file is being rendered. The one genuinely per-target skip,
// a package not importing itself, stays in [renderState.applySelfAliases].
//
// The map is kept for deduplication and only then flattened.
// ByTarget().Keys() yields one entry per target, not per package, so
// building the slice directly would give a thousand entries for a
// thousand targets in one package — turning a constant-time scan
// into a linear one, which is worse than what it replaces.
//
// Targets sharing an import path agree on their package name in any
// run that passes the layout phase's one-file-one-package check, so
// last-write-wins is not observable.
func collectSelfPackages(s *store.Store) []selfAlias {
	seen := map[string]string{}
	for _, t := range s.Emit().ByTarget().Keys() {
		if t.ImportPath == "" || t.Package == "" {
			continue
		}
		seen[t.ImportPath] = t.Package
	}
	out := make([]selfAlias, 0, len(seen))
	for path, pkg := range seen {
		// The writer omits an alias that matches the derived one, so
		// registering an agreeing pair is a no-op. Dropping it here
		// keeps the per-target loop empty on every run without a
		// pkg= override.
		if writer.DefaultAlias(path) == pkg {
			continue
		}
		out = append(out, selfAlias{path: path, pkg: pkg})
	}
	// Sorted for a stable slice and a stable diff, not for
	// correctness: Alias keys by path and each path appears once, so
	// iteration order was never observable.
	slices.SortFunc(out, func(a, b selfAlias) int { return cmp.Compare(a.path, b.path) })
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
// tracked-imports slice carries every import that survived the
// prune, so the finalisation pass can flag any path goimports added
// beyond what the body actually references. refs carries the
// reference sets the prune walk produced, for the callers that need
// to reason about the file's names after it returns.
func renderFile(
	state *renderState,
	target emit.Target,
	entities []emit.Node,
	packageDocs []string,
) ([]byte, []writer.Import, fileRefs, error) {
	file := fileFor(entities, target)
	if err := state.preRenderImports(file); err != nil {
		return nil, nil, fileRefs{}, err
	}

	decls := declEntities(entities)
	var declsBuf bytes.Buffer
	for _, n := range decls {
		// Written through rather than via render's string: every
		// decl's spelling is appended here and immediately dropped.
		if err := state.renderInto(&declsBuf, n); err != nil {
			return nil, nil, fileRefs{}, err
		}
		declsBuf.WriteString("\n\n")
	}

	top, initBlock, bottom, err := state.renderFileSlots(file)
	if err != nil {
		return nil, nil, fileRefs{}, err
	}

	if len(decls) == 0 && top == "" && initBlock == "" && bottom == "" {
		return nil, nil, fileRefs{}, ErrEmptyTarget
	}

	// The body is assembled and walked before the import block is
	// written, because the walk's whole purpose is to decide what
	// goes in that block. Prefixing the package clause is what makes
	// the fragment parse as a file; the doc comment above it is left
	// out deliberately, so a `pkg.Symbol` mentioned in prose cannot
	// hold an import alive.
	var bodyBuf bytes.Buffer
	fmt.Fprintf(&bodyBuf, "package %s\n\n", target.Package)
	bodyBuf.WriteString(top)
	bodyBuf.Write(declsBuf.Bytes())
	if initBlock != "" {
		bodyBuf.WriteString(initBlock)
		bodyBuf.WriteByte('\n')
	}
	bodyBuf.WriteString(bottom)

	refs := collectRefs(bodyBuf.Bytes(), target.JoinPath())
	tracked := pruneImports(state.imports.Imports(), refs)

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

	return fileBuf.Bytes(), tracked, refs, nil
}

// writeImportBlock emits the canonical `import ( … )` block for
// every import in imports. Empty slices emit nothing. Aliases
// matching the default-derived alias for their path are omitted
// from the line; explicit aliases (or collision-resolved variants
// like "context2") render as `<alias> "<path>"`.
//
// imports has already been through [pruneImports], so every entry
// here is one the rendered body references or one no body text
// could reference (`_`, `.`).
//
// Entries are sorted by [importGroup] then by path, with a blank
// line between groups — the arrangement the goimports resolve pass
// used to impose. Sorting on path alone within a group needs no
// tiebreak: [writer.ImportSet] is keyed by path, so no two entries
// can share one.
//
// Only the grouping is load-bearing. [go/format.Source] runs after
// this and calls [go/ast.SortImports], which re-sorts each
// blank-line-delimited run by path — so a within-group order that
// disagreed would be corrected, while a misplaced blank line would
// not. Sorting here anyway keeps the emitted bytes already
// canonical, which matters for the fused parse-and-print that
// eventually removes the formatter from this path.
func writeImportBlock(buf *bytes.Buffer, imports []writer.Import) {
	if len(imports) == 0 {
		return
	}
	sorted := slices.Clone(imports)
	slices.SortFunc(sorted, func(a, b writer.Import) int {
		if g := cmp.Compare(importGroup(a.Path), importGroup(b.Path)); g != 0 {
			return g
		}
		return strings.Compare(a.Path, b.Path)
	})
	buf.WriteString("\nimport (\n")
	prev := importGroup(sorted[0].Path)
	for _, imp := range sorted {
		if g := importGroup(imp.Path); g != prev {
			buf.WriteByte('\n')
			prev = g
		}
		buf.WriteByte('\t')
		// Against the raw segment, not the derived alias: an alias
		// the writer had to sanitise or suffix is exactly the one
		// that must be stated, and comparing it against its own
		// derivation would always agree and never emit it.
		if writer.NeedsExplicitAlias(imp.Path, imp.Alias) {
			buf.WriteString(imp.Alias)
			buf.WriteByte(' ')
		}
		buf.WriteString(strconv.Quote(imp.Path))
		buf.WriteByte('\n')
	}
	buf.WriteString(")\n")
}

// importGroup reports the gofmt grouping bucket for path, mirroring
// `importGroup` in x/tools' internal/imports.
//
// Three of its four rules are reachable here. The local-prefix rule
// (group 3) is not: `imports.LocalPrefix` is a package-level global
// that nothing in this workspace assigns, so it is always empty and
// the rule always declines. The appengine rule is reachable despite
// looking like a relic — it is not gated on the local prefix, and a
// path beginning `appengine` would otherwise land in the standard
// library's bucket.
//
// This is a duplicated rule and can drift from a future x/tools.
// [TestImportGroup_MatchesResolver] is the mitigation: it compares
// this against the resolver's own arrangement over a path corpus,
// which is the only check that can see divergence — a golden file
// pins our output and would keep passing through it.
func importGroup(path string) int {
	if strings.HasPrefix(path, "appengine") {
		return 2
	}
	if first, _, _ := strings.Cut(path, "/"); strings.Contains(first, ".") {
		return 1
	}
	return 0
}
