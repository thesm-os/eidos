// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package partition_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/partition"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, partition.Mixin(), partition.Name, partition.Params)
	})

	t.Run("resolver rewrites the read param to a qualified name", func(t *testing.T) {
		t.Parallel()
		// The point of declaring the param: an undeclared key stamps as
		// a bare name with no package and no owner, which a generator
		// cannot resolve to a callable — and the partner is the whole
		// content of this mixin.
		host := &sdk.Function{
			Name: "Put", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(partition.Name, map[string]string{
						partition.ParamRead: "Get",
					}),
				},
			},
		}
		partner := &sdk.Function{Name: "Get", Package: "x"}
		mixintest.RunWithResolver(t, partition.Mixin(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host, partner},
		})

		got, _ := shape.MixinParamKey(partition.Name, partition.ParamRead).Get(host.Meta())
		if got != "x.Get" {
			t.Fatalf("read param = %q, want %q", got, "x.Get")
		}
	})
}
