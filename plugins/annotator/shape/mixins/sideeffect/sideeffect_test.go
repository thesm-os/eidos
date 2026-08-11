// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sideeffect_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/sideeffect"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, sideeffect.Mixin(), sideeffect.Name, sideeffect.Params)
	})

	t.Run("resolver rewrites the observe param to a qualified name", func(t *testing.T) {
		t.Parallel()
		// The point of declaring the param: an undeclared key stamps as
		// a bare name with no package and no owner, which a generator
		// cannot resolve to a callable — and the partner is the whole
		// content of this mixin.
		host := &sdk.Function{
			Name: "Publish", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(sideeffect.Name, map[string]string{
						sideeffect.ParamObserve: "Count",
					}),
				},
			},
		}
		partner := &sdk.Function{Name: "Count", Package: "x"}
		mixintest.RunWithResolver(t, sideeffect.Mixin(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host, partner},
		})

		got, _ := shape.MixinParamKey(sideeffect.Name, sideeffect.ParamObserve).Get(host.Meta())
		if got != "x.Count" {
			t.Fatalf("observe param = %q, want %q", got, "x.Count")
		}
	})
}
