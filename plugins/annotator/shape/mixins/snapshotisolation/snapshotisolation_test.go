// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package snapshotisolation_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/snapshotisolation"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(
		t,
		snapshotisolation.Mixin(),
		snapshotisolation.Name,
		snapshotisolation.Params,
	)
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
					snapshotisolation.Name,
					map[string]string{snapshotisolation.ParamRead: "Get"},
				),
			},
		},
	}
	fns := []*sdk.Function{
		host,
		{Name: "Get", Package: "x"},
	}
	mixintest.RunWithResolver(t, snapshotisolation.Mixin(), &sdk.Package{
		Name: "x", Path: "x", Functions: fns,
	})
	key := shape.MixinParamKey(snapshotisolation.Name, snapshotisolation.ParamRead)
	if got, _ := key.Get(host.Meta()); got != "x.Get" {
		t.Errorf("read = %q, want x.Get", got)
	}
}
