// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package leakfree_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/leakfree"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, leakfree.Mixin(), leakfree.Name, leakfree.Params)
	})

	t.Run("every declared partner resolves to a qualified name", func(t *testing.T) {
		t.Parallel()
		// A law selecting this mixin calls the partners, so a stamp
		// left as a bare name gives the binding nothing to call — and a
		// law that never calls reports every implementation correct.
		host := &sdk.Function{
			Name: "Use", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(leakfree.Name, map[string]string{
						leakfree.ParamOpen:  "Acquire",
						leakfree.ParamClose: "Release",
					}),
				},
			},
		}
		fns := []*sdk.Function{host, {Name: "Acquire", Package: "x"}, {Name: "Release", Package: "x"}}
		mixintest.RunWithResolver(t, leakfree.Mixin(), &sdk.Package{
			Name: "x", Path: "x", Functions: fns,
		})

		keyParamOpen := shape.MixinParamKey(leakfree.Name, leakfree.ParamOpen)
		if got, _ := keyParamOpen.Get(host.Meta()); got != "x.Acquire" {
			t.Errorf("open = %q, want %q", got, "x.Acquire")
		}
		keyParamClose := shape.MixinParamKey(leakfree.Name, leakfree.ParamClose)
		if got, _ := keyParamClose.Get(host.Meta()); got != "x.Release" {
			t.Errorf("close = %q, want %q", got, "x.Release")
		}
	})
}
