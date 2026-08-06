// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline

import (
	"strconv"
	"testing"

	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/priority"
)

// benchTopoSizes are the bucket populations BenchmarkTopoSortBucket
// sweeps. 1 exposes the fixed cost of the three maps Kahn's
// algorithm allocates before it does any work; 1000 is far above any
// realistic bucket, and that is the point — the frontier insert is
// linear per element, so a quadratic term only becomes legible once
// the frontier is wide enough for the memmove to dominate the map
// operations.
var benchTopoSizes = []int{1, 10, 100, 1000}

// BenchmarkTopoSortBucket measures ordering one priority bucket:
// building the Provides index, building the in-degree and dependents
// maps, seeding and sorting the ready frontier, and draining it.
//
// The cost worth watching is the frontier, not the graph walk.
// topoSortBucket keeps `ready` sorted alphabetically so the emitted
// order is deterministic, and it maintains that ordering with
// [insertSorted] — a binary search plus a memmove that is O(len(ready))
// per insert. A realistic capability graph is wide and shallow (most
// plugins depend on nothing), so the frontier starts near the full
// bucket size and every dependent released later inserts into a
// slice of roughly that width. That is a latent O(n²) term, and the
// sweep is the assertion: per-op time must grow by roughly the same
// factor as n. A steeper slope says the frontier maintenance, not
// the graph, is what the resolver is paying for.
//
// The plugin slice is built once per size above the timed region.
// topoSortBucket does not mutate its input — it allocates its own
// output — so one fixture serves every iteration.
func BenchmarkTopoSortBucket(b *testing.B) {
	b.ReportAllocs()

	for _, n := range benchTopoSizes {
		plugins := newTopoBenchBucket(n)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			var ordered int
			for b.Loop() {
				out, err := topoSortBucket(plugins)
				if err != nil {
					b.Fatalf("topoSortBucket: %v", err)
				}
				ordered += len(out)
			}
			// A bucket that dropped plugins would surface as ErrCycle
			// above, but a bucket the loop never actually sorted would
			// not. The accumulator makes the result load-bearing and
			// the check makes the accumulator load-bearing.
			if ordered == 0 {
				b.Fatalf("accumulator stayed zero: the timed loop did not run")
			}
		})
	}
}

// newTopoBenchBucket builds n capability-declaring plugins shaped
// like a real priority bucket rather than like a worst case.
//
// Every plugin provides exactly one capability, which is what plugins
// in this framework actually do — a capability name is a promise
// about metadata the plugin stamps. Dependencies are sparse and
// backwards-pointing: roughly a third of the plugins require their
// immediate predecessor (the short chains that form when a validator
// follows a shape annotator), and every fifth additionally requires a
// plugin from the first half of the bucket (the fan-in that forms
// when several plugins consume one foundational capability).
//
// The shape matters to what the benchmark can see. Only backwards
// edges are emitted, so the graph is acyclic by construction and
// ErrCycle cannot mask a timing regression. Keeping most plugins
// dependency-free is what leaves the ready frontier wide, which is
// the condition under which [insertSorted]'s linear insert is
// visible at all; a pure chain would hide it behind a
// single-element frontier.
func newTopoBenchBucket(n int) []*benchCapPlugin {
	out := make([]*benchCapPlugin, n)
	for i := range n {
		p := &benchCapPlugin{
			// Zero-padded so alphabetical order — the tie-break the
			// resolver applies — matches numeric order, keeping the
			// produced sequence readable when a failure has to be
			// debugged.
			name:     "plugin" + zeroPad(i, n),
			provides: []string{"cap." + zeroPad(i, n)},
		}
		if i > 0 && i%3 == 0 {
			p.requires = append(p.requires, "cap."+zeroPad(i-1, n))
		}
		if i > 1 && i%5 == 0 {
			p.requires = append(p.requires, "cap."+zeroPad(i/2, n))
		}
		out[i] = p
	}
	return out
}

// zeroPad renders i left-padded with zeroes to the width of n-1, so
// the generated names sort alphabetically in numeric order.
func zeroPad(i, n int) string {
	width := len(strconv.Itoa(n - 1))
	s := strconv.Itoa(i)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// benchCapPlugin is a plugin that declares capabilities and nothing
// else. topoSortBucket reaches it only through [plugin.Plugin] and
// [plugin.CapabilityProvider], so no role method is needed.
type benchCapPlugin struct {
	name     string
	provides []string
	requires []string
}

func (p *benchCapPlugin) Name() string              { return p.name }
func (*benchCapPlugin) Priority() priority.Priority { return priority.Default }
func (p *benchCapPlugin) Provides() []string        { return p.provides }
func (p *benchCapPlugin) Requires() []string        { return p.requires }

// benchCapPlugin has to satisfy plugin.Plugin for the generic
// instantiation above; the assertion documents the coupling so a
// change to the constraint fails here rather than at the call site.
var _ plugin.Plugin = (*benchCapPlugin)(nil)
