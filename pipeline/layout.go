// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline

import (
	"cmp"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// runLayout resolves [emit.Target] on every routable emit entity in
// the store and materialises pending origin-anchored slot
// contributions onto the resulting files. The phase runs after the
// generator phase and before [store.EmitView.Freeze]; it is the
// single place where output routing decisions are made.
//
// The phase walks every routable emit kind through an explicit
// per-kind dispatch, composes an [emit.Target] per decl from the
// precedence layers — framework default, plugin filename suffix,
// resolved layout policy, per-source routing directives, CLI
// overrides — then drains the pending origin-anchored slot list
// and routes each tuple through the same composition. The
// one-file-one-package invariant is enforced once both passes
// complete. The byTarget index is rebuilt unconditionally so the
// backend's file grouping observes the resolved routing — even
// when the phase recorded routing errors, the rebuild excludes
// the offending Targets and renders the non-conflicting subset.
//
// Two flavours of per-source routing directive feed the same
// composition:
//
//   - the standalone `+gen:out <path>` directive (with optional
//     `plugin=<name>` scope and `pkg=<name>` override); and
//   - per-directive `out=` / `pkg=` keys on any plugin's own
//     directive — auto-recognised because the pipeline records
//     directive ownership at Build time (see [Pipeline.directiveOwners]).
//     Per-directive keys produce unscoped specs so companion plugins
//     emitting against the same origin inherit the override naturally.
//
// At the framework-default precedence layer the resolved package
// also picks up Go's external-test convention automatically: when
// the composed filename ends in `_test.go` and the package value
// was not pinned by a higher precedence layer (and isn't already
// suffixed `_test`), the layout appends `_test` to both Package
// and ImportPath. See [coreDirectives] for the full directive
// surface.
//
// Routing errors surface as Error diagnostics attributed to the
// pipeline's "pipeline.layout" channel. The run continues; the
// offending decls drop from the byTarget index so the backend
// skips them but remain in their per-kind buckets so debugging
// tools can still resolve them.
func (p *Pipeline) runLayout(s *store.Store) {
	ps := p.diag.For("pipeline.layout")
	// Layout runs plugin-authored code: dispatchOutputPackages calls
	// SetOutputPackages on every implementing emit value, and
	// emit.Walk traverses plugin-defined kinds to find them. Without
	// the guard a panic in either escapes the "plugin code that
	// panics is contained" contract and kills the process.
	//
	// The guard is phase-scoped rather than per-plugin: unlike the
	// frontend and generator loops, Layout has no boundary at which
	// the next plugin can independently resume — a half-routed emit
	// graph is not a state later phases can consume. Containing the
	// panic as a diagnostic and abandoning the phase is the useful
	// behaviour; continuing would be worse than stopping.
	defer diag.RecoverAs(ps, position.Pos{})
	v := s.Emit()
	outputs := p.collectPluginOutputs()

	loc := map[originOutputs]map[string]string{}
	routeDecls(s, v, ps, outputs, p, loc)
	materialiseOriginSlots(s, v, ps, outputs, p, loc)
	// Dispatch before the conflict pass: a plugin's reference to its
	// own output describes where that output was routed, and the
	// conflict pass clears Targets rather than re-routing them.
	// Running after would hand implementors paths for entities the
	// run is in the middle of dropping.
	dispatchOutputPackages(v, loc)
	enforceOneFileOnePackage(v, ps)
	v.RebuildByTarget()
}

// routeDecls dispatches per emit kind and stamps the resolved
// Target onto each routable decl. The dispatch table is the
// complete list of framework-defined routable kinds; adding a new
// kind requires registering it here explicitly. [emit.Alias] is
// special-cased because it stores its file routing in `.File`
// rather than `.Target` (the alias's `.Target` field holds the
// aliased type Ref).
//
// Each decl carries an [emit.BaseEmit.OutputTag] identifying which
// of its owning plugin's declared [plugin.Output] entries it
// belongs to. [composeOrZero] reads the tag to pick the matching
// suffix; the tag flows through the precedence pipeline so
// per-output routing overrides apply per resolved Target.
func routeDecls(
	s *store.Store,
	v *store.EmitView,
	ps *diag.PluginSink,
	outputs map[string][]plugin.Output,
	p *Pipeline,
	loc map[originOutputs]map[string]string,
) {
	v.Structs().Range(func(e *emit.Struct) bool {
		e.Target = composeOrZero(
			s,
			p,
			ps,
			outputs,
			e.SetBy(),
			e.OutputTag(),
			e.Origin(),
			e.Package,
			"struct",
			e.QName(),
			loc,
		)
		return true
	})
	v.Interfaces().Range(func(e *emit.Interface) bool {
		e.Target = composeOrZero(
			s,
			p,
			ps,
			outputs,
			e.SetBy(),
			e.OutputTag(),
			e.Origin(),
			e.Package,
			"interface",
			e.QName(),
			loc,
		)
		return true
	})
	v.Functions().Range(func(e *emit.Function) bool {
		e.Target = composeOrZero(
			s,
			p,
			ps,
			outputs,
			e.SetBy(),
			e.OutputTag(),
			e.Origin(),
			e.Package,
			"function",
			e.QName(),
			loc,
		)
		return true
	})
	v.Variables().Range(func(e *emit.Variable) bool {
		e.Target = composeOrZero(
			s,
			p,
			ps,
			outputs,
			e.SetBy(),
			e.OutputTag(),
			e.Origin(),
			e.Package,
			"variable",
			e.QName(),
			loc,
		)
		return true
	})
	v.Constants().Range(func(e *emit.Constant) bool {
		e.Target = composeOrZero(
			s,
			p,
			ps,
			outputs,
			e.SetBy(),
			e.OutputTag(),
			e.Origin(),
			e.Package,
			"constant",
			e.QName(),
			loc,
		)
		return true
	})
	v.Enums().Range(func(e *emit.Enum) bool {
		e.Target = composeOrZero(
			s,
			p,
			ps,
			outputs,
			e.SetBy(),
			e.OutputTag(),
			e.Origin(),
			e.Package,
			"enum",
			e.QName(),
			loc,
		)
		return true
	})
	// Top-level methods route via Package.Methods rather than the
	// methods bucket — that bucket also holds nested methods (which
	// inherit their owner's Target and must not be routed
	// independently). Iterating each package's Methods slice
	// naturally narrows the set to the top-level entries.
	v.Packages().Range(func(pkg *emit.Package) bool {
		for _, m := range pkg.Methods {
			m.Target = composeOrZero(
				s,
				p,
				ps,
				outputs,
				m.SetBy(),
				m.OutputTag(),
				m.Origin(),
				pkg.Path,
				"method",
				m.QName(),
				loc,
			)
		}
		return true
	})
	v.Aliases().Range(func(e *emit.Alias) bool {
		e.File = composeOrZero(
			s,
			p,
			ps,
			outputs,
			e.SetBy(),
			e.OutputTag(),
			e.Origin(),
			e.Package,
			"alias",
			e.QName(),
			loc,
		)
		return true
	})
}

// composeOrZero is the per-kind dispatch's stamp helper: it routes
// a single decl through the precedence pipeline, records the
// resolved [manifest.ResolvedLayout] on the pipeline for manifest
// composition, and returns the composed Target — or the zero
// Target when the decl cannot be routed. Routing failures
// (synthetic Origin, missing [plugin.FilenameProvider], unknown
// SetBy attribution, unknown [emit.BaseEmit.OutputTag], or empty
// OutputTag against a plugin that declares no default output)
// surface an Error diagnostic via ps before the helper returns
// the zero Target. The zero return value drops the decl from the
// byTarget rebuild and from the manifest; the per-kind bucket
// still retains it so debugging tools can still resolve it.
func composeOrZero(
	s *store.Store,
	p *Pipeline,
	ps *diag.PluginSink,
	outputs map[string][]plugin.Output,
	setBy, outputTag string,
	origin node.Node,
	emitPkgPath string,
	kind, qname string,
	loc map[originOutputs]map[string]string,
) emit.Target {
	if origin == nil {
		ps.Errorf(position.Pos{},
			"synthetic %s %q has no Origin; cannot route", kind, qname)
		return emit.Target{}
	}
	suffix, ok := resolveSuffix(p, ps, outputs, setBy, outputTag, kind, qname)
	if !ok {
		return emit.Target{}
	}
	multiOutput := len(outputs[setBy]) > 1
	t, rl, ok := composeTarget(
		s,
		p,
		ps,
		setBy,
		outputTag,
		suffix,
		multiOutput,
		origin,
		emitPkgPath,
		kind,
		qname,
	)
	if !ok {
		return emit.Target{}
	}
	p.recordResolvedLayout(t, rl)
	recordOutputPath(loc, origin, setBy, outputTag, t.ImportPath)
	return t
}

// resolveSuffix looks up the suffix from the plugin's declared
// outputs by matching outputTag against [plugin.Output.Tag]. The
// rules:
//
//   - The plugin's outputs slice is absent / nil / empty →
//     [ErrMissingFilenameProvider] (the plugin declared no
//     routable output for the active backend).
//   - outputTag is empty and the slice contains an empty-tag
//     entry → that entry's Suffix.
//   - outputTag is empty and the slice contains no empty-tag
//     entry → [ErrNoDefaultOutput] (the plugin's "every decl
//     must carry an explicit tag" intent).
//   - outputTag is non-empty and matches an entry's Tag → that
//     entry's Suffix.
//   - outputTag is non-empty with no matching entry →
//     [ErrUnknownOutputTag].
//
// Each failure surfaces an Error diagnostic via ps; the caller
// drops the decl on a false return.
func resolveSuffix(
	p *Pipeline,
	ps *diag.PluginSink,
	outputs map[string][]plugin.Output,
	setBy, outputTag, kind, qname string,
) (string, bool) {
	slice, ok := outputs[setBy]
	if !ok || len(slice) == 0 {
		p.reportLayoutErr(ps, fmt.Errorf("%w: %s %q emitted by %q",
			ErrMissingFilenameProvider, kind, qname, setBy))
		return "", false
	}
	for _, o := range slice {
		if o.Tag == outputTag {
			return o.Suffix, true
		}
	}
	if outputTag == "" {
		p.reportLayoutErr(ps, fmt.Errorf("%w: %s %q emitted by %q; declared tags: %v",
			ErrNoDefaultOutput, kind, qname, setBy, declaredTags(slice)))
		return "", false
	}
	p.reportLayoutErr(ps, fmt.Errorf("%w: %s %q emitted by %q has OutputTag %q; declared tags: %v",
		ErrUnknownOutputTag, kind, qname, setBy, outputTag, declaredTags(slice)))
	return "", false
}

// declaredTags returns the [plugin.Output.Tag] values from slice
// in declaration order. Used by routing-error diagnostics to show
// the plugin's actual contract so authors can locate the typo or
// stray `pkg.File(tag)` call quickly.
func declaredTags(slice []plugin.Output) []string {
	tags := make([]string, len(slice))
	for i, o := range slice {
		tags[i] = o.Tag
	}
	return tags
}

// composeTarget runs the precedence pipeline for one (plugin,
// origin) pair. Layer order, lowest priority first:
//
//  1. Framework default: alongside-source layout — Dir from
//     filepath.Dir(origin.Pos().File); Package + ImportPath from
//     the emit-side package the plugin emitted the decl into (when
//     known) or from the origin's source package (slot tuples,
//     synthetic decls).
//  2. Plugin filename suffix: Filename = <src-basename><suffix>.
//  3. Resolved layout policy: switches to centralised when policy
//     selects it; under alongside-source a non-empty policy.Package
//     overrides Package without changing Dir.
//  4. Per-source `+gen:out` directive on origin: overrides Filename.
//  5. CLI -o: overrides Filename for every routed decl in scope.
//  6. CLI -p (folded into policy.Package): wins for Package
//     unconditionally when set.
//
// emitPkgPath identifies the upstream [emit.Package] the decl
// lives in (the path key the store indexed it under). Empty for
// origin-anchored slot contributions, which have no upstream
// emit.Package; alongside-source routing then falls back to the
// origin's source package for Package / ImportPath.
//
// composeTarget is the single helper both decl routing and pending
// slot-tuple routing call so the two paths cannot drift. The
// returned [manifest.ResolvedLayout] records the per-field
// precedence-layer attribution for the manifest's observability
// block.
func composeTarget(
	s *store.Store,
	p *Pipeline,
	ps *diag.PluginSink,
	pluginName, outputTag, suffix string,
	multiOutput bool,
	origin node.Node,
	emitPkgPath string,
	kind, qname string,
) (emit.Target, manifest.ResolvedLayout, bool) {
	policy := p.LayoutPolicyForTag(pluginName, outputTag)
	srcPkg := originSourcePackage(s, origin)
	emitPkg := emitPackageByPath(s, emitPkgPath)
	srcDir, basename := originSourceDirBasename(origin, p.sourceRoot)

	var t emit.Target
	rl := manifest.ResolvedLayout{
		Layout:       policy.Layout,
		ResolvedFrom: map[string]manifest.Layer{},
	}

	// Filename: framework basename + plugin-declared suffix. The
	// suffix is guaranteed non-empty here because composeOrZero /
	// materialiseOriginSlots filter empty entries out of the
	// FilenameProvider lookup table; reaching this point with an
	// empty suffix is a framework bug.
	t.Filename = basename + suffix
	rl.ResolvedFrom["filename"] = manifest.LayerPluginSuffix
	rl.ResolvedFrom["layout"] = policy.LayoutFrom

	// Dir + Package + ImportPath per resolved policy. The switch
	// is exhaustive: an unrecognised Layout value is a framework
	// bug — Builder.resolveLayoutPolicy is the only producer and
	// normalises to one of the two valid values.
	switch policy.Layout {
	case LayoutCentralised:
		t.Dir = policy.Dir
		rl.ResolvedFrom["dir"] = policy.DirFrom
		if t.Dir == "" {
			// Dir defaults from Package when unset. The
			// attribution follows the Package source since that
			// is what supplied the directory's final value.
			t.Dir = policy.Package
			rl.ResolvedFrom["dir"] = policy.PackageFrom
		}
		t.Package = policy.Package
		rl.ResolvedFrom["package"] = policy.PackageFrom
		// Centralised ImportPath is not derivable without a module
		// root; left empty so the renderer's same-package elision
		// stays inert under centralised layout until config carries
		// the value forward.
	case LayoutAlongsideSource:
		t.Dir = srcDir
		rl.ResolvedFrom["dir"] = manifest.LayerFramework
		// Package / ImportPath: the upstream emit.Package the plugin
		// chose wins over the origin's source package — this lets
		// generators land output in sibling packages (mockgen's
		// `<srcPkg>_test`, custom mocks/<pkg>, etc.) without the
		// framework hardcoding language-specific conventions. Slot
		// contributions and synthetic decls have no upstream
		// emit.Package and fall back to the source package so
		// per-file slots still resolve into the source's directory.
		switch {
		case emitPkg != nil && emitPkg.Name != "":
			// Plugin's emit.Package has an opinion — honour it.
			t.Package = emitPkg.Name
			t.ImportPath = emitPkg.Path
		case srcPkg != nil:
			// Empty emit.Package.Name signals "no opinion from the
			// plugin" — fall through to the origin's source package.
			// This is the path plugins take when they construct their
			// builder with Package("", ""), leaving placement to the
			// routing layer.
			t.Package = srcPkg.Name
			t.ImportPath = srcPkg.Path
		}
		rl.ResolvedFrom["package"] = manifest.LayerFramework
		// A non-empty policy.Package under alongside-source pins
		// the package without changing the directory. Attribution
		// follows the layer that supplied the Package value.
		//
		// ImportPath has to follow Package: the plugin-chosen
		// emitPkg.Path (`<src>_test`, etc.) is no longer accurate
		// once the package identity is overridden. The policy
		// override doesn't know the canonical import path for the
		// new package, so ImportPath collapses to the source
		// package's path — every decl that lands in this routed
		// Target (regardless of which plugin's emit.Package it
		// came from) shares the same ImportPath, keeping the
		// recordingSink's Target-keyed map single-entry per
		// rendered file.
		if policy.Package != "" {
			t.Package = policy.Package
			rl.ResolvedFrom["package"] = policy.PackageFrom
			if srcPkg != nil {
				t.ImportPath = srcPkg.Path
			} else {
				t.ImportPath = ""
			}
		}
	default:
		p.diag.Internalf(position.Pos{},
			"pipeline.layout: unknown layout %q for plugin %q; cannot route",
			policy.Layout, pluginName)
		return emit.Target{}, manifest.ResolvedLayout{}, false
	}

	// +gen:out directive on origin overrides per-decl Target
	// fields. Plugin-scoped directives (`plugin=<name>`) apply to
	// the matching plugin only; unscoped directives apply to every
	// plugin targeting the origin. A non-empty pkg= arg pins
	// Target.Package + Target.ImportPath through the directive
	// precedence layer; the value-with-directory form stacks a
	// relative path onto Target.Dir.
	if spec, ok := selectOutDirective(outDirectivesFor(p, origin), pluginName, outputTag, ps, origin); ok {
		dir, filename := splitOutDirectivePath(spec.Path)
		// Unscoped filename-pinning overrides against a multi-output
		// plugin would force every output to share one filename —
		// silently collapsing the per-output distinction. The check
		// fires once per offending decl; authors scope the override
		// with `tag=<tag>` or relax to a directory-only path.
		if multiOutput && filename != "" && spec.Tag == "" {
			p.reportLayoutErr(ps, fmt.Errorf("%w: %s %q emitted by %q with directive path %q",
				ErrUnscopedMultiOutputOverride, kind, qname, pluginName, spec.Path))
			return emit.Target{}, manifest.ResolvedLayout{}, false
		}
		if filename != "" {
			t.Filename = filename
			rl.ResolvedFrom["filename"] = manifest.LayerDirective
		}
		if dir != "" {
			t.Dir = filepath.Join(t.Dir, dir)
			rl.ResolvedFrom["dir"] = manifest.LayerDirective
			// Derive Package + ImportPath from the resolved Dir's
			// basename when the directive carries a dir but no
			// explicit `pkg=` override. Attribution stays at
			// LayerFramework — the dir change came from the user but
			// the pkg name is the framework's automatic choice, so
			// downstream automatic adjustments (e.g. the _test.go
			// shift) still apply.
			if spec.Package == "" {
				t.Package = filepath.Base(t.Dir)
				if srcPkg != nil {
					t.ImportPath = path.Join(srcPkg.Path, filepath.ToSlash(dir))
				}
			}
		}
		if spec.Package != "" {
			t.Package = spec.Package
			rl.ResolvedFrom["package"] = manifest.LayerDirective
			// ImportPath is a path, not a package name. Assigning
			// spec.Package here put the clause name (`storetest`) where
			// an import path (`example.com/x/testkit`) belongs, so the
			// renderer's same-package elision compared against a value
			// that could never match and every self-reference rendered
			// qualified against a package it no longer lived in.
			//
			// The path follows the resolved Dir, matching the
			// dir-derived branch above; without a source package to
			// anchor against there is nothing to join, and an empty
			// ImportPath correctly leaves elision inert.
			switch {
			case srcPkg == nil:
				t.ImportPath = ""
			case dir != "":
				t.ImportPath = path.Join(srcPkg.Path, filepath.ToSlash(dir))
			default:
				t.ImportPath = srcPkg.Path
			}
		}
	}

	// CLI -o overrides Filename for every decl in scope. A path
	// with a directory component stacks the relative path onto
	// Target.Dir — symmetrical to the directive's path-aware form
	// — so `go:generate eidos run -target Foo -o test/foo.go`
	// produces `<origin-dir>/test/foo.go` per scope.
	//
	// Per-plugin and per-(plugin, tag) overrides win over the
	// legacy unscoped form; specificity prefers (plugin, tag) over
	// (plugin, "") via [Pipeline.PluginOutputFilename].
	cli, hasCLI := p.PluginOutputFilename(pluginName, outputTag)
	if !hasCLI {
		cli = p.OutputFilename()
	}
	if cli != "" {
		dir, filename := splitOutDirectivePath(cli)
		if filename != "" {
			t.Filename = filename
			rl.ResolvedFrom["filename"] = manifest.LayerCLI
		}
		if dir != "" {
			t.Dir = filepath.Join(t.Dir, dir)
			rl.ResolvedFrom["dir"] = manifest.LayerCLI
		}
	}

	// _test.go shift: when the resolved filename ends in `_test.go`
	// and the resolved package isn't already suffixed `_test`,
	// append `_test` to Package and ImportPath. Applies Go's
	// external test-package convention to every `_test.go` file the
	// framework routes.
	//
	// The shift deliberately ignores which precedence layer supplied
	// Package. `pkg=storetest` on a source directive answers "which
	// package does this generator's output belong to", not "suppress
	// Go's test-package convention" — honouring it literally on a
	// `_test.go` file lands the test *inside* the package it is
	// meant to exercise from outside, and silently converts an
	// external test into an internal one. A caller who genuinely
	// wants the internal form writes `pkg=storetest_test`, which the
	// suffix guard below leaves untouched.
	//
	// The same guard covers a plugin emitting into a `<pkg>_test`
	// emit.Package, which has already opted into the convention.
	if strings.HasSuffix(t.Filename, "_test.go") &&
		t.Package != "" &&
		!strings.HasSuffix(t.Package, "_test") {

		t.Package += "_test"
		if t.ImportPath != "" {
			t.ImportPath += "_test"
		}
	}

	rl.Package = t.Package
	rl.Dir = t.Dir
	rl.Filename = t.Filename
	return t, rl, true
}

// materialiseOriginSlots drains the pending origin-anchored slot
// list, composes a Target for each tuple's origin via the same
// helper that decl routing uses, and appends each item to the
// matching named slot on the resolved [emit.File]. Iteration order
// is the registration order on the pending list — the EmitView
// preserves insertion order on append, and the generator phase
// iterates source nodes in store insertion order, so two runs
// over the same input produce the same materialisation order.
//
// A tuple whose contributing plugin lacks a filename suffix
// surfaces [ErrMissingFilenameProvider]; the tuple is dropped
// and the run continues.
func materialiseOriginSlots(
	s *store.Store,
	v *store.EmitView,
	ps *diag.PluginSink,
	outputs map[string][]plugin.Output,
	p *Pipeline,
	loc map[originOutputs]map[string]string,
) {
	pending := v.PendingOriginSlots()
	for _, tup := range pending {
		setBy := tup.Prov.SetBy
		// Origin-anchored slot contributions resolve through the
		// OutputTag stamped on the queued item — empty for the
		// contributor's primary output, non-empty for a tagged
		// secondary output. The item is a full [emit.Node] whose
		// [emit.BaseEmit.OutputTagName] flows through the standard
		// per-output dispatch the same way decl-level OutputTag
		// values do.
		outputTag := tup.Item.OutputTag()
		suffix, ok := resolveSuffix(p, ps, outputs, setBy, outputTag, "slot "+tup.SlotName, "")
		if !ok {
			continue
		}
		// Slot tuples have no upstream emit.Package — the
		// contribution becomes part of a file Layout creates
		// during materialisation. Pass empty emitPkgPath so
		// composeTarget falls back to source-package routing.
		target, rl, ok := composeTarget(
			s,
			p,
			ps,
			setBy,
			outputTag,
			suffix,
			len(outputs[setBy]) > 1,
			tup.Origin,
			"",
			"slot "+tup.SlotName,
			"",
		)
		if !ok {
			continue
		}
		p.recordResolvedLayout(target, rl)
		recordOutputPath(loc, tup.Origin, setBy, outputTag, target.ImportPath)
		// FileFor only errors when the view is frozen; Layout runs
		// pre-freeze so the lookup-or-create cannot fail here.
		file, _ := v.FileFor(target)
		if err := file.Slot(tup.SlotName).Append(tup.Item, tup.Prov); err != nil {
			ps.Errorf(position.Pos{},
				"slot %q: append failed on %s: %v",
				tup.SlotName, target.JoinPath(), err)
		}
	}
}

// originOutputs identifies the set of outputs one plugin produced
// for one source node — the tuple Layout already routes on, since
// every Target in a run is composed from an origin, a contributing
// plugin, and an output tag. Keying on it makes [emit.OutputPackageSetter]
// dispatch a lookup rather than an inference.
//
// node.Node values are interfaces over pointers and so comparable,
// which is what lets the tuple be a map key.
type originOutputs struct {
	origin node.Node
	plugin string
}

// recordOutputPath notes that the output identified by (origin,
// plugin, tag) resolved to importPath.
//
// A nil origin is ignored — synthetic entities carry no origin, so
// nothing can key a lookup against them. An empty importPath is
// recorded deliberately: centralised routing resolves a Target
// without a derivable import path, and "routed, path unknown" is a
// state [emit.OutputPackageSetter] implementors are documented to observe
// separately from "not routed at all".
func recordOutputPath(
	loc map[originOutputs]map[string]string,
	origin node.Node,
	plugin, tag, importPath string,
) {
	if loc == nil || origin == nil {
		return
	}
	key := originOutputs{origin: origin, plugin: plugin}
	byTag, ok := loc[key]
	if !ok {
		byTag = map[string]string{}
		loc[key] = byTag
	}
	byTag[tag] = importPath
}

// dispatchOutputPackages hands every [emit.OutputPackageSetter] value in the
// store the import paths its own plugin's outputs resolved to.
//
// Discovery walks from both roots: packages, which reach decls and
// their nested slots, and files, which Layout itself creates during
// slot materialisation and which need not be attached to a package.
// [emit.Walk] visits every slot item, so a plugin-defined emit kind
// sitting in a file slot is reached even though the walk does not
// descend into it — being visited suffices, because OutputAware is
// implemented on the value itself.
//
// Each value is called at most once. Reaching one from both roots is
// ordinary, and the seen-set is what keeps the "at most once" half
// of the OutputAware contract true.
//
// A value whose (origin, plugin) pair routed nothing is skipped
// rather than called with an empty map, so an implementor can treat
// any call as carrying at least one usable answer.
func dispatchOutputPackages(v *store.EmitView, loc map[originOutputs]map[string]string) {
	seen := map[emit.Node]struct{}{}
	var walker emit.Visitor
	walker = emit.VisitorFunc(func(n emit.Node) emit.Visitor {
		aware, ok := n.(emit.OutputPackageSetter)
		if !ok {
			return walker
		}
		if _, done := seen[n]; done {
			return walker
		}
		seen[n] = struct{}{}
		if byTag := loc[originOutputs{origin: aware.Origin(), plugin: aware.SetBy()}]; len(byTag) > 0 {
			aware.SetOutputPackages(byTag)
		}
		return walker
	})
	v.Packages().Range(func(p *emit.Package) bool {
		emit.Walk(p, walker)
		return true
	})
	v.Files().Range(func(f *emit.File) bool {
		emit.Walk(f, walker)
		return true
	})
}

// reportLayoutErr emits err as an Error diagnostic and retains it
// for [Pipeline.Run] to join into its return value.
//
// Both halves matter and neither replaces the other: the diagnostic
// is what a human reads, and the retained value is what
// errors.Is matches. Reporting through the sink alone flattens the
// sentinel to text, which is the defect this exists to close.
func (p *Pipeline) reportLayoutErr(ps *diag.PluginSink, err error) {
	ps.Errorf(position.Pos{}, "%s", err.Error())
	p.recordLayoutErr(err)
}

// enforceOneFileOnePackage walks every routable emit entity, groups
// by (Dir, Filename), and surfaces an Error diagnostic for any
// group whose members disagree on Package. The offending decls have
// their Target cleared so the byTarget rebuild below excludes them;
// the manifest sink correspondingly omits the conflicted Target.
// The run continues for non-conflicting Targets.
func enforceOneFileOnePackage(v *store.EmitView, ps *diag.PluginSink) {
	groups := map[fileKey]map[string][]string{} // file → package → qnames
	visit := func(t emit.Target, qname string) {
		if t.Dir == "" || t.Filename == "" {
			return
		}
		key := fileKey{dir: t.Dir, filename: t.Filename}
		pkgs, ok := groups[key]
		if !ok {
			pkgs = map[string][]string{}
			groups[key] = pkgs
		}
		pkgs[t.Package] = append(pkgs[t.Package], qname)
	}
	v.Structs().Range(func(e *emit.Struct) bool { visit(e.Target, e.QName()); return true })
	v.Interfaces().Range(func(e *emit.Interface) bool { visit(e.Target, e.QName()); return true })
	v.Functions().Range(func(e *emit.Function) bool { visit(e.Target, e.QName()); return true })
	v.Variables().Range(func(e *emit.Variable) bool { visit(e.Target, e.QName()); return true })
	v.Constants().Range(func(e *emit.Constant) bool { visit(e.Target, e.QName()); return true })
	v.Enums().Range(func(e *emit.Enum) bool { visit(e.Target, e.QName()); return true })
	v.Aliases().Range(func(e *emit.Alias) bool { visit(e.File, e.QName()); return true })

	// Report every group first, then clear once. Clearing inside the
	// loop re-walked all seven buckets per conflicting group and
	// joined a path string for every decl it touched — 7k bucket
	// walks and up to k·n concatenations, which on a repo-wide
	// package mismatch is quadratic, since k scales with n.
	conflicted := map[fileKey]struct{}{}
	for key, pkgs := range groups {
		if len(pkgs) < 2 {
			continue
		}
		ps.Errorf(position.Pos{},
			"one-file-one-package violation at %s: conflicting package declarations %s",
			key.path(), formatPackageConflict(pkgs))
		conflicted[key] = struct{}{}
	}
	if len(conflicted) > 0 {
		clearConflictedTargets(v, conflicted)
	}
}

// fileKey identifies the output file a decl routes to.
//
// A comparable struct rather than the joined "dir/filename" string
// the grouping used to build: the concatenation escaped into the map
// key on every visited decl, and the clearing pass then rebuilt it
// per decl per group.
//
// It also distinguishes Dir="a/b", Filename="c" from Dir="a",
// Filename="b/c", which the joined form reported as a conflict.
// splitOutDirectivePath never puts a separator in a filename, so that
// pairing is unreachable — but it was a false positive and it is gone.
type fileKey struct {
	dir      string
	filename string
}

// path renders the key the way the joined string used to read, for
// the diagnostic. Kept as a method so the message cannot drift from
// the identity it names.
func (k fileKey) path() string { return k.dir + "/" + k.filename }

// formatPackageConflict renders the conflicting-package set in a
// deterministic, parser-friendly form for the diagnostic message:
// `pkgA={qname1,qname2}; pkgB={qname3}`. Package names are sorted
// lexicographically so the diagnostic stays byte-stable across
// runs even though the visit order through the buckets is not
// guaranteed to be deterministic. The qnames within each entry
// are left in insertion order since emit-view ranges yield decls
// in insertion order already.
func formatPackageConflict(pkgs map[string][]string) string {
	type entry struct {
		pkg    string
		qnames []string
	}
	out := make([]entry, 0, len(pkgs))
	for pkg, qnames := range pkgs {
		out = append(out, entry{pkg: pkg, qnames: qnames})
	}
	slices.SortFunc(out, func(a, b entry) int { return cmp.Compare(a.pkg, b.pkg) })
	parts := make([]string, 0, len(out))
	for _, e := range out {
		parts = append(parts, fmt.Sprintf("%s={%s}", e.pkg, strings.Join(e.qnames, ",")))
	}
	return strings.Join(parts, "; ")
}

// clearConflictedTargets zeroes every routable decl routed to a file
// in conflicted. The cleared decls drop from the byTarget rebuild and
// from the manifest; they remain in their per-kind buckets so
// debugging tooling can still inspect them.
//
// Takes the whole conflicting set so the seven buckets are walked
// once for the run rather than once per conflicting group. Clearing
// is idempotent and order-independent — zeroing a Target empties its
// Dir and Filename, and the grouping pass skips those — so a batched
// clear produces the identical cleared set a per-group one did.
func clearConflictedTargets(v *store.EmitView, conflicted map[fileKey]struct{}) {
	bad := func(t emit.Target) bool {
		_, ok := conflicted[fileKey{dir: t.Dir, filename: t.Filename}]
		return ok
	}
	v.Structs().Range(func(e *emit.Struct) bool {
		if bad(e.Target) {
			e.Target = emit.Target{}
		}
		return true
	})
	v.Interfaces().Range(func(e *emit.Interface) bool {
		if bad(e.Target) {
			e.Target = emit.Target{}
		}
		return true
	})
	v.Functions().Range(func(e *emit.Function) bool {
		if bad(e.Target) {
			e.Target = emit.Target{}
		}
		return true
	})
	v.Variables().Range(func(e *emit.Variable) bool {
		if bad(e.Target) {
			e.Target = emit.Target{}
		}
		return true
	})
	v.Constants().Range(func(e *emit.Constant) bool {
		if bad(e.Target) {
			e.Target = emit.Target{}
		}
		return true
	})
	v.Enums().Range(func(e *emit.Enum) bool {
		if bad(e.Target) {
			e.Target = emit.Target{}
		}
		return true
	})
	// Aliases route on File, not Target. A membership test written
	// once over e.Target would compile and silently stop clearing
	// them.
	v.Aliases().Range(func(e *emit.Alias) bool {
		if bad(e.File) {
			e.File = emit.Target{}
		}
		return true
	})
}

// collectPluginOutputs builds the plugin-name → declared-Outputs
// lookup the Layout phase consults to dispatch each decl to its
// matching [plugin.Output.Suffix]. The map contains one entry per
// registered generator that implements [plugin.FilenameProvider]
// AND returns at least one Output for the active backend's
// language — generators that don't implement the capability OR
// that return an empty slice are both absent from the map. The
// two cases are equivalent at the routing layer: both signal "I
// don't declare routable output for this language", and either
// kind of attribution failure surfaces [ErrMissingFilenameProvider]
// when a decl emitted by such a plugin reaches the Layout phase.
//
// The map stores the entire Outputs slice (rather than just the
// primary Suffix) so [resolveSuffix] can dispatch on each decl's
// [emit.BaseEmit.OutputTag]: empty tag → empty-Tag Output;
// non-empty tag → matching Output.Tag. The framework's Build-time
// validation enforces the slice's well-formedness before Layout
// runs, so resolveSuffix can assume non-empty Suffix values and
// at-most-one empty-Tag entry.
func (p *Pipeline) collectPluginOutputs() map[string][]plugin.Output {
	out := map[string][]plugin.Output{}
	lang := p.backendLanguage()
	for _, gen := range p.generators {
		fp, ok := any(gen).(plugin.FilenameProvider)
		if !ok {
			continue
		}
		outputs := fp.Outputs(lang)
		if len(outputs) == 0 {
			continue
		}
		out[gen.Name()] = outputs
	}
	return out
}

// backendLanguage returns the configured backend's language —
// the value passed to [plugin.FilenameProvider.Outputs]
// (and to [plugin.TemplateProvider.Templates]) so plugins narrow
// their per-language surfaces to the active backend. Returns
// the empty string when no backend is configured; under that
// (no-backend) construction the Layout phase will not run anyway.
func (p *Pipeline) backendLanguage() string {
	if p.backend == nil {
		return ""
	}
	return p.backend.Language()
}

// originSourcePackage walks origin's owning-package chain and
// returns the matching [*node.Package] from the node store. The
// lookup is a direct ByQName so scope predicates do not filter it
// out. Returns nil when origin has no resolvable package (a
// synthetic Origin built without a frontend, or a Node kind whose
// Owner chain never reaches a packaged ancestor).
//
// The helper is the single place that knows how to extract a
// packaged ancestor from a [node.Node] of any kind. Both decl
// routing and pending-slot-tuple routing call into it so the two
// paths cannot drift.
func originSourcePackage(s *store.Store, origin node.Node) *node.Package {
	path := originPackagePath(origin)
	if path == "" {
		return nil
	}
	pkg, _ := s.Nodes().Packages().ByQName(path)
	return pkg
}

// originPackagePath returns the import path of origin's owning
// source package. Top-level kinds (Struct, Interface, Function,
// Variable, Constant, Enum, Alias) read their Package field
// directly. File terminates at its Owner Package. EnumVariant
// terminates at its Owner Enum. Method and Field walk their Owner
// chain — typically one hop to the enclosing Struct or Interface
// — to reach a packaged ancestor. Returns the empty string for
// kinds (Param, TypeParam, Import, Package itself) whose
// Origin-as-routable use case has no documented routing semantics;
// adding an arm here requires the same kind to surface through
// [store.Reader] and at least one plugin's emit graph.
func originPackagePath(n node.Node) string {
	cur := n
	for cur != nil {
		switch v := cur.(type) {
		case *node.Struct:
			return v.Package
		case *node.Interface:
			return v.Package
		case *node.Function:
			return v.Package
		case *node.Variable:
			return v.Package
		case *node.Constant:
			return v.Package
		case *node.Enum:
			return v.Package
		case *node.Alias:
			return v.Package
		case *node.File:
			if v.Owner != nil {
				return v.Owner.Path
			}
			return ""
		case *node.Method:
			cur = v.Owner
			continue
		case *node.Field:
			cur = v.Owner
			continue
		case *node.EnumVariant:
			if v.Owner == nil {
				return ""
			}
			return v.Owner.Package
		}
		return ""
	}
	return ""
}

// originSourceDirBasename returns the directory and basename
// derived from origin's source position. The basename strips the
// `.go` extension when present so the Layout phase can append the
// plugin's filename suffix verbatim. Returns empty strings when the
// origin's source position carries no file — synthetic origins fall
// back to that case and the resulting Target needs an alternative
// filename source (CLI -o, +gen:out) to be routable.
//
// When sourceRoot is non-empty and the origin's source file lives
// under it, the returned directory is normalised relative to the
// root so [emit.Target.Dir] stays portable across machines — the
// manifest's stored Target is the canonical record consumed by
// `eidos prune` and `eidos check`, and an absolute path in there
// breaks both. Files outside sourceRoot fall back to the absolute
// path so attribution remains correct even when a fixture sits
// outside the working directory.
func originSourceDirBasename(origin node.Node, sourceRoot string) (dir, basename string) {
	pos := origin.Pos()
	if pos.File == "" {
		return "", ""
	}
	base := filepath.Base(pos.File)
	if ext := filepath.Ext(base); ext != "" {
		base = base[:len(base)-len(ext)]
	}
	d := filepath.Dir(pos.File)
	if sourceRoot != "" && filepath.IsAbs(d) {
		if rel, err := filepath.Rel(sourceRoot, d); err == nil && !strings.HasPrefix(rel, "..") {
			d = rel
		}
	}
	if d == "." {
		d = ""
	}
	return d, base
}

// emitPackageByPath returns the [emit.Package] indexed under path
// in the store's emit view, or nil when no package is registered
// under that path. Layout calls this to read the upstream
// emit.Package's Name (which carries the plugin's intended
// package identity for the rendered file) without coupling to the
// store's package-bucket internals.
func emitPackageByPath(s *store.Store, path string) *emit.Package {
	if path == "" {
		return nil
	}
	pkg, _ := s.Emit().Packages().ByQName(path)
	return pkg
}

// outDirectiveSpec captures the parsed shape of a single `+gen:out`
// directive: the positional path (filename or relative-to-origin
// dir + filename), the optional `plugin=<name>` scope filter, the
// optional `tag=<output-tag>` per-output filter, and the optional
// `pkg=<name>` package override. The Layout phase resolves the
// directive's positional value into Target.Dir and
// Target.Filename via [splitOutDirectivePath] and applies the
// package override against Target.Package + Target.ImportPath.
//
// Filter composition rules ([selectOutDirective] applies them):
//
//   - `PluginName` empty + `Tag` empty: applies to every plugin's
//     every output (the original unscoped form).
//   - `PluginName` set: applies only to that plugin.
//   - `Tag` set: applies only to outputs whose
//     [emit.BaseEmit.OutputTag] matches.
//   - Both set: intersection — that plugin's output with the
//     matching tag.
type outDirectiveSpec struct {
	Path       string
	PluginName string
	Tag        string
	Package    string
}

// outDirectivesFor returns every routing-bearing directive attached
// to n in source order, parsed into the [outDirectiveSpec] shape.
// Two flavours contribute:
//
//   - The standalone `+gen:out <path>` directive (with optional
//     `plugin=<name>` scope and `pkg=<name>` override). Skipped
//     silently when its positional path is absent.
//   - Any directive owned by a plugin (per
//     [Pipeline.directiveOwners]) that carries `out=` and / or
//     `pkg=` keys. The resulting spec is implicitly scoped to the
//     directive's owning plugin — semantically identical to a
//     `+gen:out plugin=<owner>` directive, but anchored at the
//     directive that actually triggers the emission so users don't
//     have to repeat the plugin name.
func outDirectivesFor(p *Pipeline, n node.Node) []outDirectiveSpec {
	var specs []outDirectiveSpec
	for _, d := range n.Directives() {
		if d.Name == OutDirective {
			if len(d.Args) == 0 {
				continue
			}
			specs = append(specs, outDirectiveSpec{
				Path:       d.Args[0],
				PluginName: d.Value("plugin"),
				Tag:        d.Value("tag"),
				Package:    d.Value("pkg"),
			})
			continue
		}
		// Per-directive routing keys: honoured when the directive
		// is owned by a registered plugin and carries at least one
		// routing key. Every key is scoped to the directive's owner
		// — `//+gen:stub out=testkit/` routes stubgen's output and
		// nothing else.
		//
		// Scoping is what lets two generators on one node travel to
		// different places, which is the ordinary case the moment a
		// type carries more than one directive:
		//
		//	//+gen:mock out=mocks/ pkg=demomocks
		//	//+gen:stub out=testkit/ pkg=demotest
		//
		// These keys were once recorded unscoped, so a companion
		// plugin discovering another's emit could inherit its
		// routing without restating it. The trade was bad: the pair
		// above silently routed the stub into `mocks/` because the
		// mock directive happened to be written first. A companion
		// that genuinely needs to follow another plugin uses the
		// standalone `+gen:out plugin=<name>` form, which says so.
		owner, owned := p.directiveOwners[d.Name]
		if !owned {
			continue
		}
		out := d.Value("out")
		pkg := d.Value("pkg")
		tag := d.Value("tag")
		if out == "" && pkg == "" && tag == "" {
			continue
		}
		spec := outDirectiveSpec{
			Path:    out,
			Tag:     tag,
			Package: pkg,
		}
		spec.PluginName = owner
		specs = append(specs, spec)
	}
	return specs
}

// selectOutDirective picks the most specific [outDirectiveSpec] in
// specs that applies to (pluginName, outputTag). Filter rules: a
// spec's `PluginName` must be empty or equal pluginName, AND its
// `Tag` must be empty or equal outputTag. Specificity rule among
// matching specs: more filters set wins (both PluginName + Tag >
// PluginName only > Tag only > unscoped). Returns (zero, false)
// when no directive applies.
//
// Two matching specs at the same specificity are ambiguous: the
// source says twice, differently, where one output goes, and
// nothing in the precedence rules breaks the tie. Rather than let
// declaration order decide silently, the ambiguity is reported
// through ps and the first spec is used, so the run still produces
// the output the diagnostic refers to. Identical duplicates are not
// ambiguous — they resolve to the same place, so there is nothing
// to report.
func selectOutDirective(
	specs []outDirectiveSpec,
	pluginName, outputTag string,
	ps *diag.PluginSink,
	origin node.Node,
) (outDirectiveSpec, bool) {
	var (
		best      outDirectiveSpec
		bestScore int
		haveBest  bool
		conflict  bool
	)
	for _, s := range specs {
		if s.PluginName != "" && s.PluginName != pluginName {
			continue
		}
		if s.Tag != "" && s.Tag != outputTag {
			continue
		}
		score := 0
		if s.PluginName != "" {
			score++
		}
		if s.Tag != "" {
			score++
		}
		switch {
		case !haveBest || score > bestScore:
			best = s
			bestScore = score
			haveBest = true
			conflict = false
		case score == bestScore && s != best:
			conflict = true
		}
	}
	if conflict && ps != nil {
		ps.Errorf(originPos(origin),
			"conflicting routing for %q: two directives of equal scope both "+
				"apply and neither takes precedence; scope one with "+
				"`+gen:out plugin=<name>` or remove it",
			pluginName)
	}
	if haveBest {
		return best, true
	}
	return outDirectiveSpec{}, false
}

// originPos returns origin's source position, or the zero position
// for a nil origin — a synthetic entity carries no source location
// to anchor a diagnostic to.
func originPos(origin node.Node) position.Pos {
	if origin == nil {
		return position.Pos{}
	}
	return origin.Pos()
}

// splitOutDirectivePath splits a directive's positional value into
// the relative directory under the origin's source directory and
// the rendered filename. Values without a directory component
// route the file into the origin's own directory (the legacy
// `+gen:out filename.go` behaviour); values with one or more
// directory segments stack on top of the origin's directory so
// `+gen:out test/handler.go` next to `foo/iface.go` lands the
// file at `foo/test/handler.go`. Absolute paths in the directive
// value are interpreted relative to the origin's source directory
// (the leading separator is stripped) — the directive cannot
// escape the source tree.
func splitOutDirectivePath(value string) (dir, filename string) {
	value = filepath.ToSlash(value)

	// A trailing separator is the directive's only way to say
	// "directory, keep the composed filename" (`+gen:out build/`).
	// path.Clean discards it, so the signal is captured before the
	// clamp and restored after.
	dirOnly := strings.HasSuffix(value, "/")

	// Rooting before cleaning is what makes the containment contract
	// true rather than merely documented. Cleaning `value` in place
	// would leave a leading `..` intact, and composeTarget applies
	// filepath.Join(originDir, dir) — Join cleans, so each surviving
	// `..` cancels a component of the origin's own directory and the
	// output lands outside the package, or with enough segments
	// outside the module. Against a synthetic root the same segments
	// cancel harmlessly, and the leading-separator strip that used to
	// be a special case falls out of the same operation.
	value = strings.TrimPrefix(path.Clean("/"+value), "/")
	if dirOnly && value != "" {
		value += "/"
	}

	dir, filename = path.Split(value)
	dir = strings.TrimRight(dir, "/")
	return dir, filename
}
