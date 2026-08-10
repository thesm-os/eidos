// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package full assembles the shape annotator with every classification
// this repository ships.
//
// The three per-axis aggregators — [detectors.All], [contracts.All] and
// [mixins.All] — have always existed, and nothing combined them, so a
// consumer wanting the whole vocabulary spelled the chain out. That is
// three chances to omit an axis, and omitting one is silent: the
// classifications it would have stamped are simply absent, which reads
// downstream as "this method has no shape" rather than as a
// registration mistake.
//
// It lives beside the axes rather than in `shape` itself because each
// of them imports `shape` to name the types it returns. A union in
// `shape` would be an import cycle, so the union goes below them.
//
// Taking everything is a decision, not a default. A pipeline that
// registers a classification it never reads pays the walk over every
// callable for it — cheap, but not free — and stamps meta a consumer
// may be surprised to find. Reach for the per-axis aggregators, or the
// individual detectors, when the set is known and small.
package full

import (
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins"
	"go.thesmos.sh/eidos/sdk"
)

// New returns a shape plugin carrying every detector, contract and
// mixin in the repository.
//
// Defined as exactly the three [detectors.All], [contracts.All] and
// [mixins.All] calls, with no curation of its own. Curating here would
// make "full" mean something a reader has to look up, and would drift
// the moment an axis gained a member — which is the failure this
// exists to remove, reintroduced one level up.
func New() *shape.Plugin {
	return shape.New().
		Detectors(detectors.All()...).
		Contracts(contracts.All()...).
		Mixins(mixins.All()...)
}

// Annotators returns the fully-loaded plugin's three registrations, in
// pipeline order — the one-liner for a consumer who wants the whole
// vocabulary and does not want to think about either decomposition:
//
//	for _, a := range full.Annotators() {
//	    pipe.WithAnnotator(a)
//	}
//
// Equivalent to `full.New().Annotators()`, and named here so both
// mistakes a consumer can make — a missing axis and a missing
// companion plugin — are closed by the same call.
func Annotators() []sdk.Annotator { return New().Annotators() }
