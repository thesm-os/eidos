// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline

import (
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
)

// RecordResolvedLayoutForTest exposes recordResolvedLayout so a test
// can seed the per-run derived state a [Pipeline] accumulates and
// then observe that a subsequent [Pipeline.Run] clears it.
//
// The state is unexported because nothing outside the package has
// any business writing it; the leak it caused was invisible for the
// same reason.
// Writes the map directly rather than through the phase's recorder:
// the recorder is phase-scoped and publishes at the end of runLayout,
// so it cannot seed a Pipeline that is not running. The seeding tests
// care that an entry exists and that Run clears it, not what it was
// composed from.
func (p *Pipeline) RecordResolvedLayoutForTest(t emit.Target, rl manifest.ResolvedLayout) {
	p.resolvedLayoutsMu.Lock()
	defer p.resolvedLayoutsMu.Unlock()
	if p.resolvedLayouts == nil {
		p.resolvedLayouts = map[emit.Target]resolvedEntry{}
	}
	p.resolvedLayouts[t] = resolvedEntry{rl: rl}
}

// HasLayoutActivityForTest exposes hasLayoutActivity — the length
// check over resolvedLayouts that answered true forever once any run
// routed anything, arming a manifest guard on later runs that routed
// nothing.
func (p *Pipeline) HasLayoutActivityForTest() bool { return p.hasLayoutActivity() }
