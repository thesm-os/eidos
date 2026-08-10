// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package wrappedvia_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/wrappedvia"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, wrappedvia.Mixin(), wrappedvia.Name, wrappedvia.Params)
	})

	t.Run("resolver rewrites fn param to qualified name", func(t *testing.T) {
		t.Parallel()
		host := &sdk.Function{
			Name: "Wrapped", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(wrappedvia.Name, map[string]string{
						"fn": "Delegate",
					}),
				},
			},
		}
		inner := &sdk.Function{Name: "Delegate", Package: "x"}
		mixintest.RunWithResolver(t, wrappedvia.Mixin(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host, inner},
		})

		got, _ := shape.MixinParamKey(wrappedvia.Name, "fn").Get(host.Meta())
		if got != "x.Delegate" {
			t.Fatalf("fn param = %q, want %q", got, "x.Delegate")
		}
	})
}
