// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package injectionsafe_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/injectionsafe"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, injectionsafe.Mixin(), injectionsafe.Name, injectionsafe.Params)
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
				mixintest.HostDirective(injectionsafe.Name, map[string]string{injectionsafe.ParamRead: "Load"}),
			},
		},
	}
	fns := []*sdk.Function{
		host,
		{Name: "Load", Package: "x"},
	}
	mixintest.RunWithResolver(t, injectionsafe.Mixin(), &sdk.Package{
		Name: "x", Path: "x", Functions: fns,
	})
	if got, _ := shape.MixinParamKey(injectionsafe.Name, injectionsafe.ParamRead).Get(host.Meta()); got != "x.Load" {
		t.Errorf("read = %q, want x.Load", got)
	}
}
