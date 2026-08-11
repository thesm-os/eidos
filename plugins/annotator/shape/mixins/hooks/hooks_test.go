// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package hooks_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/hooks"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, hooks.Mixin(), hooks.Name, hooks.Params)
	})

	t.Run("resolver rewrites the register param to a qualified name", func(t *testing.T) {
		t.Parallel()
		// The point of declaring the param: an undeclared key stamps as
		// a bare name with no package and no owner, which a generator
		// cannot resolve to a callable — and the partner is the whole
		// content of this mixin.
		host := &sdk.Function{
			Name: "Run", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(hooks.Name, map[string]string{
						hooks.ParamRegister: "OnEvent",
					}),
				},
			},
		}
		partner := &sdk.Function{Name: "OnEvent", Package: "x"}
		mixintest.RunWithResolver(t, hooks.Mixin(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host, partner},
		})

		got, _ := shape.MixinParamKey(hooks.Name, hooks.ParamRegister).Get(host.Meta())
		if got != "x.OnEvent" {
			t.Fatalf("register param = %q, want %q", got, "x.OnEvent")
		}
	})
}
