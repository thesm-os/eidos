// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package lease_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/lease"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t, lease.Contract(), lease.Name, lease.Roles)
}

func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	acquire := &sdk.Function{
		Name: "Acquire", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(lease.Name, "acquire", map[string]string{
					"release": "Release",
				}),
			},
		},
	}
	release := &sdk.Function{Name: "Release", Package: "x"}
	pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{acquire, release}}
	diags := contracttest.RunPipeline(t, lease.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, acquire.Meta(), lease.Name, "acquire")
	contracttest.AssertPartner(t, acquire.Meta(), lease.Name, "release", "x.Release")
	contracttest.AssertRole(t, release.Meta(), lease.Name, "release")
}
