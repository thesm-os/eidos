// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pagination_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/pagination"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t, pagination.Contract(), pagination.Name, pagination.Roles)
}

func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "List", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(pagination.Name, "reader", map[string]string{
					"cursor": "Cursor",
				}),
			},
		},
	}
	pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
	diags := contracttest.RunPipeline(t, pagination.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, fn.Meta(), pagination.Name, "reader")
	got, ok := shape.ContractParamKey(pagination.Name, "cursor").Get(fn.Meta())
	if !ok || got != "Cursor" {
		t.Fatalf("param.cursor = %q (present=%v); want %q", got, ok, "Cursor")
	}
}
