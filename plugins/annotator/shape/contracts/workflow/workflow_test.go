// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package workflow_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/workflow"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		contracttest.AssertIdentity(t, workflow.Contract(), workflow.Name, workflow.Roles)
	})

	t.Run("pipeline stamps transitions param", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Run", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(workflow.Name, "fn", map[string]string{
						"transitions": "start->step1->done",
					}),
				},
			},
		}
		pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
		diags := contracttest.RunPipeline(t, workflow.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)

		got, _ := shape.ContractParamKey(workflow.Name, "transitions").Get(fn.Meta())
		if got != "start->step1->done" {
			t.Fatalf("transitions = %q, want %q", got, "start->step1->done")
		}
	})
}
