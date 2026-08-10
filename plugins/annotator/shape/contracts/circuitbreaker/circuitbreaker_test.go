// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package circuitbreaker_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/circuitbreaker"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		contracttest.AssertIdentity(t, circuitbreaker.Contract(), circuitbreaker.Name, circuitbreaker.Roles)
	})

	t.Run("pipeline stamps role", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Call", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(circuitbreaker.Name, "fn", nil),
				},
			},
		}
		pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
		diags := contracttest.RunPipeline(t, circuitbreaker.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)
		contracttest.AssertRole(t, fn.Meta(), circuitbreaker.Name, "fn")
	})
}
