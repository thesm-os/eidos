// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline

import (
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

// Directive validation, and why the pipeline owns it.
//
// A directive is a claim measured against two things: the graph a
// frontend produced, and the schemas this run registered. Frontends
// own the first and plugins the second, so the pipeline is the only
// part holding both — which is what [plugin.FrontendContext.Registry]
// was lending a frontend, and every defect this replaced followed from
// the loan.
//
// A frontend validating during conversion is skipped entirely by a
// warm cache, because conversion is what a cache hit exists to avoid:
// a malformed directive was reported on the cold run and accepted
// silently on every run after it. A frontend validating per node kind
// spells the call once per kind, so a kind it does not name goes
// unchecked and nothing says so. And a frontend that does not validate
// at all — which two of this repository's three did — is
// indistinguishable from source with nothing wrong in it.
//
// Walking the store here is none of those by construction. There is
// one traversal, it runs whatever produced the graph, and it takes
// each node's kind from the node rather than from a call site that
// could pair them wrongly.

// validateDirectives checks every directive in the loaded graph
// against the run's registry.
//
// Runs after the frontends and before the node view is frozen.
// [directive.Validate] folds a schema's positional defaults into the
// directive's arguments, and every reader downstream — annotators,
// the override pass, generators — expects the folded list. Validating
// later would hand the first of them the unfolded one.
func (p *Pipeline) validateDirectives(s *store.Store) {
	if p.registry == nil {
		return
	}
	// No phase-boundary log. This is a step inside the frontend phase
	// rather than a phase of its own — it runs no plugins, so it has
	// nothing to parallelise and no bucket to report — and logging it as
	// one would put a sixth entry in a list that names who ran.
	ps := p.diag.For("pipeline")
	// The walk reaches plugin-registered schemas but runs none of their
	// code; the containment is here for the same reason the override
	// pass has it — a panic in a shared traversal would abort the run
	// with no attribution at all.
	defer diag.RecoverAs(ps, position.Pos{})

	seen := make(map[node.Node]bool)
	var descend node.VisitorFunc
	descend = func(n node.Node) node.Visitor {
		// Guarded rather than assumed acyclic. The walk descends into
		// type references, and a self-referential type is ordinary
		// source — `type Node struct { Next *Node }` — so an unguarded
		// traversal does not terminate on input nobody would call
		// malformed.
		if n == nil || seen[n] {
			return nil
		}
		seen[n] = true
		if ds := n.Directives(); len(ds) > 0 {
			directive.Validate(ds, n.Kind(), p.registry, ps)
		}
		// Self-referencing, because [node.Walk] drives a node's children
		// with whatever its Visit returned: a visitor returning anything
		// else stops one level down.
		return descend
	}
	s.Nodes().Packages().Range(func(pkg *node.Package) bool {
		node.Walk(pkg, descend)
		return true
	})
}
