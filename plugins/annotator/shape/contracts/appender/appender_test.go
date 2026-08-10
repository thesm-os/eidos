// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package appender_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/appender"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t,
		appender.Contract(),
		appender.Name, appender.Roles)
}

func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Append", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(appender.Name, "fn", nil),
			},
		},
	}
	pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
	diags := contracttest.RunPipeline(t, appender.Contract(), pkg)

	contracttest.AssertRole(t, fn.Meta(), appender.Name, "fn")
	contracttest.AssertNoErrorDiag(t, diags)
}
