// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"slices"
	"strings"
	"text/template"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/plugin"
)

// unrenderableKinds returns every emit kind in the graph that no
// template can render, paired with the plugin that contributed it.
//
// # Why this runs before rendering rather than during
//
// [ErrTemplateMissing] already fires from the render site, but by
// then the run is midway through a target: it reports one kind, on
// one file, and stops. A plugin that shipped no template at all — a
// misspelled `define`, a template tree rooted one directory too
// high, an emit kind renamed on one side only — then surfaces as a
// single confusing failure about whichever declaration happened to
// sort first, with every other affected kind invisible until the
// first is fixed and the run repeated.
//
// Checked up front, the whole set is reported at once, each entry
// naming the plugin that has to fix it. The render-site error stays
// where it is: it is the guard for a kind reached by a path this
// walk does not cover.
func unrenderableKinds(ctx *plugin.BackendContext, tmpl *template.Template) []missingKind {
	seen := map[string]missingKind{}
	for _, target := range ctx.Store.Emit().ByTarget().Keys() {
		for _, root := range ctx.Store.Emit().ByTarget().Get(target) {
			collectKinds(root, tmpl, seen)
		}
	}
	out := make([]missingKind, 0, len(seen))
	for _, m := range seen {
		out = append(out, m)
	}
	// Deterministic, because the map above is not and this list is
	// rendered into diagnostics a golden may pin.
	slices.SortFunc(out, func(a, b missingKind) int { return strings.Compare(a.kind, b.kind) })
	return out
}

// missingKind names one emit kind with no template, and who shipped
// it.
type missingKind struct {
	kind   string
	setBy  string
	sample emit.Node
}

// collectKinds walks one emit tree, recording each kind the template
// set cannot render.
//
// Recorded once per kind rather than once per node: a plugin that
// forgot a template did so for every value of that kind, and a
// diagnostic per declaration would bury the one fact under a hundred
// copies of it.
func collectKinds(root emit.Node, tmpl *template.Template, seen map[string]missingKind) {
	// The visitor returns itself rather than nil: [emit.Walk] treats a
	// nil result as "do not descend", so a visitor that answered nil
	// after recording would see the root and nothing beneath it —
	// which is every declaration a file holds.
	var visit emit.Visitor
	visit = emit.VisitorFunc(func(n emit.Node) emit.Visitor {
		kind := string(n.Kind())
		if !templateDispatched(kind) || tmpl.Lookup(kind) != nil {
			return visit
		}
		if _, dup := seen[kind]; !dup {
			seen[kind] = missingKind{kind: kind, setBy: n.SetBy(), sample: n}
		}
		return visit
	})
	emit.Walk(root, visit)
}

// coreKindPrefix namespaces the emit model's own kinds.
const coreKindPrefix = "emit."

// templateDispatched reports whether a kind is one the backend
// resolves a template for.
//
// Only plugin-defined kinds are. The core `emit.` namespace is
// rendered two ways — the declaration kinds by their own templates,
// and expressions, statements and type references by the dedicated
// `renderExpr` / `renderStmt` / `renderType` funcmap helpers, which
// have no template and never will. Walking the graph and demanding a
// template per kind would report every statement in every body.
//
// Namespace rather than an exclusion list: a list would have to be
// revised whenever the model gains a node kind, and forgetting to
// would turn a new expression variant into an error on every run.
func templateDispatched(kind string) bool {
	return kind != "" && !strings.HasPrefix(kind, coreKindPrefix)
}

// reportUnrenderableKinds records a diagnostic per kind no template
// can render, and reports whether any were found.
//
// An error rather than a warning: the alternative is a rendered file
// silently missing whatever that kind contributed, which reaches the
// consumer's compiler as a reference to something nothing declares —
// blamed on the generator, several steps from the cause.
func reportUnrenderableKinds(ctx *plugin.BackendContext, ps *diag.PluginSink, tmpl *template.Template) bool {
	missing := unrenderableKinds(ctx, tmpl)
	for _, m := range missing {
		ps.Errorf(positionOf(m.sample),
			"no template renders emit kind %q, contributed by plugin %q; the backend "+
				"resolves a template by the kind's string value, so the plugin ships no "+
				"`{{ define %q }}` — check the template's define name and that its tree is "+
				"rooted at the directory the plugin declares",
			m.kind, attribution(m.setBy), m.kind)
	}
	return len(missing) > 0
}

// attribution names the contributing plugin, or says that nothing
// claimed the value.
//
// An unattributed emit value is one built without a provenance
// context; naming that plainly is more use than an empty pair of
// quotes in the middle of a sentence.
func attribution(setBy string) string {
	if setBy == "" {
		return "<unattributed>"
	}
	return setBy
}

// positionOf returns the source position a diagnostic about n should
// point at, zero when the value carries none.
func positionOf(n emit.Node) position.Pos {
	if n == nil {
		return position.Pos{}
	}
	if origin := n.Origin(); origin != nil {
		return origin.Pos()
	}
	return position.Pos{}
}
