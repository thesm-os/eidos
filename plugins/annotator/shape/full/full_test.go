// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package full_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors"
	"go.thesmos.sh/eidos/plugins/annotator/shape/full"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins"
	"go.thesmos.sh/eidos/sdk"
)

// TestNew pins that "full" means every axis, in full.
//
// Asserted against the three aggregators rather than a hard-coded
// count, because a count is a number someone has to remember to
// update and this must not need updating: an axis gaining a member
// gains it here too, or the union is not one.
func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("carries every classification the repository ships", func(t *testing.T) {
		t.Parallel()
		p := full.New()
		// Registered counts are not exposed, so the property is checked
		// the way a consumer would notice it failing: the fully-loaded
		// plugin must classify at least as much as one built from any
		// single axis, and rebuilding it from all three must agree.
		want := shape.New().
			Detectors(detectors.All()...).
			Contracts(contracts.All()...).
			Mixins(mixins.All()...)
		if len(p.Directives()) != len(want.Directives()) {
			t.Errorf("Directives = %d, want %d — full.New must be exactly the three All() calls",
				len(p.Directives()), len(want.Directives()))
		}
	})

	t.Run("no axis is empty", func(t *testing.T) {
		t.Parallel()
		// Guards the omission this package exists to prevent. An empty
		// axis would make the union silently narrower than its name.
		if len(detectors.All()) == 0 || len(contracts.All()) == 0 || len(mixins.All()) == 0 {
			t.Fatalf("detectors=%d contracts=%d mixins=%d; an empty axis makes full.New a lie",
				len(detectors.All()), len(contracts.All()), len(mixins.All()))
		}
	})
}

// TestAnnotators pins the three-plugin registration, which is the
// other thing a consumer must get exactly right and cannot check.
func TestAnnotators(t *testing.T) {
	t.Parallel()

	t.Run("returns the umbrella, resolver and validator in order", func(t *testing.T) {
		t.Parallel()
		got := full.Annotators()
		if len(got) != 3 {
			t.Fatalf("Annotators = %d plugins, want 3", len(got))
		}
		// Order is the contract: each depends on its predecessor having
		// run. Checked by name so a reordering is a failure rather than
		// a coincidence of priorities.
		want := []string{shape.PluginName, shape.ResolverName, shape.ValidatorName}
		for i, w := range want {
			if got[i].Name() != w {
				t.Errorf("Annotators[%d] = %q, want %q", i, got[i].Name(), w)
			}
		}
	})

	t.Run("priorities run them in the returned order", func(t *testing.T) {
		t.Parallel()
		// The slice order documents the sequence; the priorities are
		// what actually enforce it. If the two disagreed, a consumer
		// registering them in slice order would still get the right
		// run order and never learn the slice was wrong.
		// Priority is an optional capability rather than part of the
		// Annotator interface, so it is read through the assertion the
		// pipeline itself uses.
		type prioritised interface{ Priority() sdk.Priority }
		got := full.Annotators()
		prev := sdk.Priority(0)
		for i, a := range got {
			p, ok := a.(prioritised)
			if !ok {
				t.Fatalf("Annotators[%d] (%s) declares no priority, so its position "+
					"in the run is whatever registration order happens to give it", i, a.Name())
			}
			if i > 0 && prev >= p.Priority() {
				t.Errorf("Annotators[%d] (%s, %v) does not run before [%d] (%s, %v)",
					i-1, got[i-1].Name(), prev, i, a.Name(), p.Priority())
			}
			prev = p.Priority()
		}
	})
}
