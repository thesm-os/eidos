// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package lifecycleafterclose_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/lifecycleafterclose"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, lifecycleafterclose.Mixin(), lifecycleafterclose.Name, lifecycleafterclose.Params)
	})

	t.Run("resolver rewrites the close param to a qualified name", func(t *testing.T) {
		t.Parallel()
		// The point of declaring the param: an undeclared key stamps as
		// a bare name with no package and no owner, which a generator
		// cannot resolve to a callable — and the partner is the whole
		// content of this mixin.
		host := &sdk.Function{
			Name: "Read", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(lifecycleafterclose.Name, map[string]string{
						lifecycleafterclose.ParamClose: "Close",
					}),
				},
			},
		}
		partner := &sdk.Function{Name: "Close", Package: "x"}
		mixintest.RunWithResolver(t, lifecycleafterclose.Mixin(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host, partner},
		})

		got, _ := shape.MixinParamKey(lifecycleafterclose.Name, lifecycleafterclose.ParamClose).Get(host.Meta())
		if got != "x.Close" {
			t.Fatalf("close param = %q, want %q", got, "x.Close")
		}
	})
}
