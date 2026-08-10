// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package watcher_test

import (
	"testing"

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
