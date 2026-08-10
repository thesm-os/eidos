// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cursor_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/cursor"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t,
		cursor.Contract(),
		cursor.Name, cursor.Roles)
}

func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	next := &sdk.Function{
		Name: "Next", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(cursor.Name, "next", map[string]string{
					"close": "Close",
				}),
			},
		},
	}
	closeFn := &sdk.Function{Name: "Close", Package: "x"}
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{next, closeFn},
	}
	diags := contracttest.RunPipeline(t, cursor.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, next.Meta(), cursor.Name, "next")
	contracttest.AssertPartner(t, next.Meta(), cursor.Name, "close", "x.Close")
	contracttest.AssertRole(t, closeFn.Meta(), cursor.Name, "close")
	contracttest.AssertPartner(t, closeFn.Meta(), cursor.Name, "next", "x.Next")
}
