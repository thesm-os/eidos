// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package crdtmerge_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/crdtmerge"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, crdtmerge.Mixin(), crdtmerge.Name, crdtmerge.Params)
}

// TestMixin_ObserverResolves pins the partner params: the claim is
// about state a check has to look at, and a bare name is not
// something a generated check can call.
func TestMixin_ObserverResolves(t *testing.T) {
	t.Parallel()
	host := &sdk.Function{
		Name: "Host", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				mixintest.HostDirective(
					crdtmerge.Name,
					map[string]string{crdtmerge.ParamWrite: "Add", crdtmerge.ParamRead: "Items"},
				),
			},
		},
	}
	fns := []*sdk.Function{
		host,
		{Name: "Add", Package: "x"},
		{Name: "Items", Package: "x"},
	}
	mixintest.RunWithResolver(t, crdtmerge.Mixin(), &sdk.Package{
		Name: "x", Path: "x", Functions: fns,
	})
	if got, _ := shape.MixinParamKey(crdtmerge.Name, crdtmerge.ParamWrite).Get(host.Meta()); got != "x.Add" {
		t.Errorf("write = %q, want x.Add", got)
	}
	if got, _ := shape.MixinParamKey(crdtmerge.Name, crdtmerge.ParamRead).Get(host.Meta()); got != "x.Items" {
		t.Errorf("read = %q, want x.Items", got)
	}
}
