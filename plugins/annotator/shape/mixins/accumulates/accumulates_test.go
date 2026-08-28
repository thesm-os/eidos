// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package accumulates_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/accumulates"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, accumulates.Mixin(), accumulates.Name, accumulates.Params)
}

func TestMixin_Attaches(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Append", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				mixintest.HostDirective(accumulates.Name, nil),
			},
		},
	}
	bag := mixintest.RunPipeline(t, accumulates.Mixin(), fn)
	mixintest.AssertAttached(t, bag, accumulates.Name)
}

// TestMixin_Observe covers the key that makes the claim checkable.
//
// "N calls have N observable effects" needs the observation, and the
// idempotence probe does not supply it: inverting that probe asserts
// the second call is refused, which is a different sentence — and a
// callable whose effect is out of band answers the same error twice
// whether it compounded or coalesced.
func TestMixin_Observe(t *testing.T) {
	t.Parallel()

	build := func(kv map[string]string) (*sdk.Method, *sdk.Package) {
		add := &sdk.Method{
			Name: "Add",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(accumulates.Name, kv),
				},
			},
		}
		total := &sdk.Method{Name: "Total"}
		counter := &sdk.Interface{
			Name: "Counter", Package: "x",
			Methods: []*sdk.Method{add, total},
		}
		add.Owner, total.Owner = counter, counter
		return add, &sdk.Package{
			Name: "x", Path: "x",
			Interfaces: []*sdk.Interface{counter},
		}
	}

	t.Run("the observation resolves through the callable scope", func(t *testing.T) {
		t.Parallel()
		// A sibling method, so the scope is the owner's method set —
		// and the stamp lands qualified, which is what a law needs to
		// call it.
		add, pkg := build(map[string]string{accumulates.ParamObserve: "Total"})
		mixintest.RunWithResolver(t, accumulates.Mixin(), pkg)

		got, _ := shape.MixinParamKey(accumulates.Name, accumulates.ParamObserve).Get(add.Meta())
		if got != "x.Counter.Total" {
			t.Fatalf("observe = %q, want x.Counter.Total", got)
		}
	})

	t.Run("an observation nothing declares is reported", func(t *testing.T) {
		t.Parallel()
		// KindCallable, so a typo is caught where the author is
		// rather than surfacing as a law reading a sibling that does
		// not exist.
		_, pkg := build(map[string]string{accumulates.ParamObserve: "Absent"})
		var found bool
		for _, d := range mixintest.RunWithValidator(t, accumulates.Mixin(), pkg) {
			if d.Severity == sdk.SeverityError {
				found = true
			}
		}
		if !found {
			t.Fatal("an unresolvable observation was accepted")
		}
	})

	t.Run("the bare form still classifies", func(t *testing.T) {
		t.Parallel()
		// Optional: a compounding callable without one is still what
		// the mixin names, and the counting law does not bind.
		_, pkg := build(map[string]string{})
		for _, d := range mixintest.RunWithValidator(t, accumulates.Mixin(), pkg) {
			if d.Severity == sdk.SeverityError {
				t.Fatalf("bare accumulates was refused: %s", d.Message)
			}
		}
	})
}
