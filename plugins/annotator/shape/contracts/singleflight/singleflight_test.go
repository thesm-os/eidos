// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package singleflight_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/singleflight"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t,
		singleflight.Contract(),
		singleflight.Name, singleflight.Roles)
}

func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Fetch", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(singleflight.Name, "fn", nil),
			},
		},
	}
	pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
	diags := contracttest.RunPipeline(t, singleflight.Contract(), pkg)

	contracttest.AssertRole(t, fn.Meta(), singleflight.Name, "fn")
	contracttest.AssertNoErrorDiag(t, diags)
}
