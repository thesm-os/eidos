// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package upserter_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/upserter"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t, upserter.Contract(), upserter.Name, upserter.Roles)
}

func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	put := &sdk.Function{
		Name: "Put", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(upserter.Name, "writer", map[string]string{
					"reader": "GetByID",
				}),
			},
		},
	}
	get := &sdk.Function{Name: "GetByID", Package: "x"}
	pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{put, get}}
	diags := contracttest.RunPipeline(t, upserter.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, put.Meta(), upserter.Name, "writer")
	contracttest.AssertPartner(t, put.Meta(), upserter.Name, "reader", "x.GetByID")
	contracttest.AssertRole(t, get.Meta(), upserter.Name, "reader")
}
