// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package transaction_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/transaction"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t,
		transaction.Contract(),
		transaction.Name, transaction.Roles)
}

// TestContract_PipelineRoundTrip exercises the umbrella →
// resolver → validator sequence for a real `+gen:contract
// transaction` directive — proving the package's [Contract]
// value plugs into the framework correctly.
func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Charge", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(transaction.Name, "fn", nil),
			},
		},
	}
	pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
	diags := contracttest.RunPipeline(t, transaction.Contract(), pkg)

	contracttest.AssertRole(t, fn.Meta(), transaction.Name, "fn")
	contracttest.AssertNoErrorDiag(t, diags)
}
