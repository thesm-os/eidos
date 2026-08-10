// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package validates_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/validates"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, validates.Mixin(), validates.Name, validates.Params)
	})

	t.Run("resolver rewrites fn param to qualified name", func(t *testing.T) {
		t.Parallel()
		host := &sdk.Function{
			Name: "Save", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(validates.Name, map[string]string{
						"fn": "ValidateInput",
					}),
				},
			},
		}
		validator := &sdk.Function{Name: "ValidateInput", Package: "x"}
		mixintest.RunWithResolver(t, validates.Mixin(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host, validator},
		})

		got, _ := shape.MixinParamKey(validates.Name, "fn").Get(host.Meta())
		if got != "x.ValidateInput" {
			t.Fatalf("fn param = %q, want %q", got, "x.ValidateInput")
		}
	})
}
