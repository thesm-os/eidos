// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package poisonable_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/poisonable"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, poisonable.Mixin(), poisonable.Name, poisonable.Params)
	})

	t.Run("every declared partner resolves to a qualified name", func(t *testing.T) {
		t.Parallel()
		// A law selecting this mixin calls the partners, so a stamp
		// left as a bare name gives the binding nothing to call — and a
		// law that never calls reports every implementation correct.
		host := &sdk.Function{
			Name: "Err", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(poisonable.Name, map[string]string{
						poisonable.ParamInduce: "Poison",
					}),
				},
			},
		}
		fns := []*sdk.Function{host, {Name: "Poison", Package: "x"}}
		mixintest.RunWithResolver(t, poisonable.Mixin(), &sdk.Package{
			Name: "x", Path: "x", Functions: fns,
		})

		keyParamInduce := shape.MixinParamKey(poisonable.Name, poisonable.ParamInduce)
		if got, _ := keyParamInduce.Get(host.Meta()); got != "x.Poison" {
			t.Errorf("induce = %q, want %q", got, "x.Poison")
		}
	})
}
