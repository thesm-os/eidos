// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package orderafter_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/orderafter"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, orderafter.Mixin(), orderafter.Name, orderafter.Params)
	})

	t.Run("resolver rewrites fn param to qualified name", func(t *testing.T) {
		t.Parallel()
		host := &sdk.Function{
			Name: "DoWork", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(orderafter.Name, map[string]string{
						"fn": "Initialise",
					}),
				},
			},
		}
		initFn := &sdk.Function{Name: "Initialise", Package: "x"}
		mixintest.RunWithResolver(t, orderafter.Mixin(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host, initFn},
		})

		got, _ := shape.MixinParamKey(orderafter.Name, "fn").Get(host.Meta())
		if got != "x.Initialise" {
			t.Fatalf("fn param = %q, want %q", got, "x.Initialise")
		}
	})
}
