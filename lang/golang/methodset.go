// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// Reporter is where a diagnostic goes, over whatever sink a caller
// holds.
//
// The same arrangement [Resolver] uses and for the same reason: this
// package cannot depend on the pipeline, and a caller that already
// holds a sink should not write an adapter to hand it over.
// `ctx.Diag` satisfies it structurally, so a plugin passes what it was
// given.
//
// Two methods rather than the whole sink, because two are what
// [ReportMethodSet] raises. A wider port would make every fake a
// caller writes answer for severities nothing here emits.
type Reporter interface {
	// Warnf reports something the author may not need to act on.
	Warnf(pos position.Pos, format string, args ...any)

	// Errorf reports something no run of any width resolves.
	Errorf(pos position.Pos, format string, args ...any)
}

// Consequence is the clause a generator's method-set diagnostics end
// on: what that generator's output loses when an embed contributes
// nothing.
//
// Carried as data because it is the only thing that differs between
// callers, and it differs for a reason a reader can act on. A double
// missing a method cannot stand in for the interface it doubles at
// all, so nothing is emitted; a harness over part of a contract still
// runs and merely passes an implementation that fails the rest. Both
// are refusals, and a reader deserves the one that applies to what
// they asked for rather than a sentence written for somebody else's
// generator.
type Consequence struct {
	// Partial ends the cycle diagnostic, which is a warning. The walk
	// broke the cycle only after the interface it points back at had
	// contributed, so the set is short of nothing.
	Partial string

	// Incomplete ends the diagnostics that stop generation.
	Incomplete string
}

// ReportMethodSet reports every embed that contributed nothing to a
// resolved method set, and returns whether what came back is usable.
//
// Resolution itself belongs to the model — the embed walk, the
// duplicate rule, the cycle guard and the attribution of a method to
// the embed it arrived through are facts about a method set, and
// `store.Reader.MethodSet` answers them. What lives here is what a Go
// generator does with an incomplete one, which is the half each caller
// had written for itself.
//
// # Why the result matters
//
// Reading `Methods` alone reads what the source typed rather than what
// the interface has, and a double short an embedded method does not
// satisfy the interface it doubles. Reporting that and generating
// anyway is worse than either alternative, because the pipeline
// carries every phase to completion: the backend still renders, the
// short file lands on disk, and the non-zero exit is the only sign it
// is wrong. A caller is expected to emit nothing when this is false.
//
// # Severity
//
// Split on whether a wider run would fix it, because the two ask
// different things of the author.
//
// An unloaded embed is a warning: the same source under a wider
// pattern resolves, and the author has written nothing wrong. It still
// stops generation, since the method set is genuinely short.
//
// A cycle is a warning that stops nothing. The walk broke out of it
// only after the interface it points back at had contributed, so the
// set is complete and the note exists to explain a shape a reader may
// not have known was there.
//
// Anything else — an embed that is not an interface, a parameterised
// one the model cannot substitute through — is an error against the
// declaration, because no run of any width resolves it.
//
// The embed is named the way the source spelled it, through [Display]:
// `io.Closer` rather than the bare `Closer` the reference carries, so
// the message names something the author can find in their own file.
func ReportMethodSet(
	r Reporter, set node.MethodSetResult, iface *node.Interface, plugin string, why Consequence,
) bool {
	if r == nil || iface == nil {
		// Nothing to report through, and nothing that could have gone
		// wrong: an absent sink is a caller mistake this package cannot
		// diagnose, and reporting the set as unusable would stop a
		// generator for a fault that is not in its source.
		return true
	}
	complete := true
	for _, issue := range set.Issues {
		written := Display(issue.Embed.Type)
		switch issue.Reason {
		case node.ReasonCyclic:
			r.Warnf(issue.Embed.Pos(),
				"%s: interface %q embeds %q through a cycle; the walk broke out of it, so %s",
				plugin, iface.QName(), written, why.Partial)
		case node.ReasonUnresolved:
			complete = false
			r.Warnf(issue.Embed.Pos(),
				"%s: interface %q embeds %q, which this run did not load, so its method set "+
					"cannot be completed; nothing is generated, because %s",
				plugin, iface.QName(), written, why.Incomplete)
		default:
			complete = false
			r.Errorf(issue.Embed.Pos(),
				"%s: interface %q embeds %q, which %s; nothing is generated, because %s",
				plugin, iface.QName(), written, issue.Reason, why.Incomplete)
		}
	}
	return complete
}
