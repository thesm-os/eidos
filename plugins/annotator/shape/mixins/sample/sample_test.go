// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sample_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/sample"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, sample.Mixin(), sample.Name, sample.Params)
	})

	t.Run("resolver rewrites the builder param to a qualified name", func(t *testing.T) {
		t.Parallel()
		// The point of declaring the param: an undeclared key stamps as
		// a bare name with no package and no owner, which a generator
		// cannot resolve to a callable — and the partner is the whole
		// content of this mixin.
		host := &sdk.Function{
			Name: "Accept", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(sample.Name, map[string]string{
						sample.ParamBuilder: "NewFixture",
					}),
				},
			},
		}
		partner := &sdk.Function{Name: "NewFixture", Package: "x"}
		mixintest.RunWithResolver(t, sample.Mixin(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host, partner},
		})

		got, _ := shape.MixinParamKey(sample.Name, sample.ParamBuilder).Get(host.Meta())
		if got != "x.NewFixture" {
			t.Fatalf("builder param = %q, want %q", got, "x.NewFixture")
		}
	})
}
