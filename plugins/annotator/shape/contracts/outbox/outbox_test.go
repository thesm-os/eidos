// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package outbox_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/outbox"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t,
		outbox.Contract(),
		outbox.Name, outbox.Roles)
}

func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	app := &sdk.Function{
		Name: "Append", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(outbox.Name, "append", map[string]string{
					"subscribe": "Subscribe",
				}),
			},
		},
	}
	sub := &sdk.Function{Name: "Subscribe", Package: "x"}
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{app, sub},
	}
	diags := contracttest.RunPipeline(t, outbox.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, app.Meta(), outbox.Name, "append")
	contracttest.AssertPartner(t, app.Meta(), outbox.Name, "subscribe", "x.Subscribe")
	contracttest.AssertRole(t, sub.Meta(), outbox.Name, "subscribe")
	contracttest.AssertPartner(t, sub.Meta(), outbox.Name, "append", "x.Append")
}
