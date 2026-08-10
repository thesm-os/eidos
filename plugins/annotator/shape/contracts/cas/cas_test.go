// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cas_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/cas"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t, cas.Contract(), cas.Name, cas.Roles)
}

func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Save", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(cas.Name, "writer", map[string]string{
					"version": "Version",
				}),
			},
		},
	}
	pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
	diags := contracttest.RunPipeline(t, cas.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, fn.Meta(), cas.Name, "writer")
	// The `version` KV lands under the param namespace, not the
	// partner namespace — the resolver does not touch it.
	got, ok := shape.ContractParamKey(cas.Name, "version").Get(fn.Meta())
	if !ok || got != "Version" {
		t.Fatalf("param.version = %q (present=%v); want %q", got, ok, "Version")
	}
	if _, ok := shape.ContractPartnerKey(cas.Name, "version").Get(fn.Meta()); ok {
		t.Fatalf("param value leaked into partner namespace")
	}
}
