// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package watcher_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/watcher"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t,
		watcher.Contract(),
		watcher.Name, watcher.Roles)
}

func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	watch := &sdk.Function{
		Name: "Watch", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(watcher.Name, "watch", map[string]string{
					"trigger": "Notify",
				}),
			},
		},
	}
	trigger := &sdk.Function{Name: "Notify", Package: "x"}
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{watch, trigger},
	}
	diags := contracttest.RunPipeline(t, watcher.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, watch.Meta(), watcher.Name, "watch")
	contracttest.AssertPartner(t, watch.Meta(), watcher.Name, "trigger", "x.Notify")
	contracttest.AssertRole(t, trigger.Meta(), watcher.Name, "trigger")
	contracttest.AssertPartner(t, trigger.Meta(), watcher.Name, "watch", "x.Watch")
}

// TestContract_HandleMembers covers the third resolver scope.
//
// Next and Stop are declared on the subscription Watch answers, not on
// the interface Watch belongs to, so neither the callable scope nor
// the var scope reaches them. The law this contract selects has to
// call both, and a stamp left as a bare name gives the binding nothing
// to call.
func TestContract_HandleMembers(t *testing.T) {
	t.Parallel()

	build := func(loadHandle bool) (*sdk.Method, *sdk.Package) {
		watch := &sdk.Method{
			Name: "Watch",
			Params: []*sdk.Param{
				{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}},
			},
			Returns: sdk.AnonReturns(
				&sdk.TypeRef{
					TypeKind: sdk.TypeRefPointer,
					Elem:     &sdk.TypeRef{Name: "Subscription", Package: "x"},
				},
				&sdk.TypeRef{Name: "error"},
			),
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(watcher.Name, "watch", map[string]string{
						watcher.ParamNext: "Next",
						watcher.ParamStop: "Stop",
					}),
				},
			},
		}
		iface := &sdk.Interface{Name: "Store", Package: "x", Methods: []*sdk.Method{watch}}
		watch.Owner = iface
		pkg := &sdk.Package{Name: "x", Path: "x", Interfaces: []*sdk.Interface{iface}}
		if loadHandle {
			sub := &sdk.Struct{
				Name: "Subscription", Package: "x",
				Methods: []*sdk.Method{{Name: "Next"}, {Name: "Stop"}},
			}
			for _, m := range sub.Methods {
				m.Owner = sub
			}
			pkg.Structs = []*sdk.Struct{sub}
		}
		return watch, pkg
	}

	t.Run("a handle method resolves against the answered type", func(t *testing.T) {
		t.Parallel()
		watch, pkg := build(true)
		diags := contracttest.RunPipeline(t, watcher.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)

		next, _ := shape.ContractParamKey(watcher.Name, watcher.ParamNext).Get(watch.Meta())
		stop, _ := shape.ContractParamKey(watcher.Name, watcher.ParamStop).Get(watch.Meta())
		if next != "x.Subscription.Next" || stop != "x.Subscription.Stop" {
			t.Fatalf("next/stop = %q/%q, want x.Subscription.Next/x.Subscription.Stop", next, stop)
		}
	})

	t.Run("an unloaded handle type stamps unvalidated", func(t *testing.T) {
		t.Parallel()
		// The one place a diagnostic's presence depends on the run's
		// patterns. Reporting here would fail a correct directive
		// whenever the handle lives in a package outside the patterns,
		// so silence is deliberate — and not a pass.
		_, pkg := build(false)
		diags := contracttest.RunPipeline(t, watcher.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)
	})
}
