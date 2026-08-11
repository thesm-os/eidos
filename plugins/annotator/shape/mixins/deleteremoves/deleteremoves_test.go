// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package deleteremoves_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/deleteremoves"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, deleteremoves.Mixin(), deleteremoves.Name, deleteremoves.Params)
	})

	t.Run("resolver rewrites the read param to a qualified name", func(t *testing.T) {
		t.Parallel()
		// The point of declaring the param: an undeclared key stamps as
		// a bare name with no package and no owner, which a generator
		// cannot resolve to a callable — and the partner is the whole
		// content of this mixin.
		host := &sdk.Function{
			Name: "Delete", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(deleteremoves.Name, map[string]string{
						deleteremoves.ParamRead: "Get",
					}),
				},
			},
		}
		partner := &sdk.Function{Name: "Get", Package: "x"}
		mixintest.RunWithResolver(t, deleteremoves.Mixin(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host, partner},
		})

		got, _ := shape.MixinParamKey(deleteremoves.Name, deleteremoves.ParamRead).Get(host.Meta())
		if got != "x.Get" {
			t.Fatalf("read param = %q, want %q", got, "x.Get")
		}
	})
}
