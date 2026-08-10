// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ifmatch_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/ifmatch"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		contracttest.AssertIdentity(t, ifmatch.Contract(), ifmatch.Name, ifmatch.Roles)
	})

	t.Run("pipeline stamps pred param", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Update", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(ifmatch.Name, "writer", map[string]string{
						"pred": "Version==Expected",
					}),
				},
			},
		}
		pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
		diags := contracttest.RunPipeline(t, ifmatch.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)

		got, _ := shape.ContractParamKey(ifmatch.Name, "pred").Get(fn.Meta())
		if got != "Version==Expected" {
			t.Fatalf("pred = %q, want %q", got, "Version==Expected")
		}
	})
}
