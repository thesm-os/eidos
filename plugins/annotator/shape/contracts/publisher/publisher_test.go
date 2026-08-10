// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package publisher_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/publisher"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t,
		publisher.Contract(),
		publisher.Name, publisher.Roles)
}

func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	pub := &sdk.Function{
		Name: "Publish", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(publisher.Name, "publish", map[string]string{
					"subscribe": "Subscribe",
				}),
			},
		},
	}
	sub := &sdk.Function{Name: "Subscribe", Package: "x"}
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{pub, sub},
	}
	diags := contracttest.RunPipeline(t, publisher.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, pub.Meta(), publisher.Name, "publish")
	contracttest.AssertPartner(t, pub.Meta(), publisher.Name, "subscribe", "x.Subscribe")
	contracttest.AssertRole(t, sub.Meta(), publisher.Name, "subscribe")
	contracttest.AssertPartner(t, sub.Meta(), publisher.Name, "publish", "x.Publish")
}
