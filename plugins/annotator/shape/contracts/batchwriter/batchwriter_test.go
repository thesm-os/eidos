// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package batchwriter_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/batchwriter"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		contracttest.AssertIdentity(t, batchwriter.Contract(), batchwriter.Name, batchwriter.Roles)
	})

	t.Run("pipeline stamps mode param", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "WriteBatch", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(batchwriter.Name, "writer", map[string]string{
						"mode": "all-or-nothing",
					}),
				},
			},
		}
		pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
		diags := contracttest.RunPipeline(t, batchwriter.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)

		contracttest.AssertRole(t, fn.Meta(), batchwriter.Name, "writer")
		got, _ := shape.ContractParamKey(batchwriter.Name, "mode").Get(fn.Meta())
		if got != "all-or-nothing" {
			t.Fatalf("mode param = %q, want %q", got, "all-or-nothing")
		}
	})
}
