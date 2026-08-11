// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package streamreflectsmutations_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/streamreflectsmutations"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t,
			streamreflectsmutations.Mixin(),
			streamreflectsmutations.Name,
			streamreflectsmutations.Params)
	})

	t.Run("resolver rewrites the mutate param to a qualified name", func(t *testing.T) {
		t.Parallel()
		// The point of declaring the param: an undeclared key stamps as
		// a bare name with no package and no owner, which a generator
		// cannot resolve to a callable — and the partner is the whole
		// content of this mixin.
		host := &sdk.Function{
			Name: "Stream", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(streamreflectsmutations.Name, map[string]string{
						streamreflectsmutations.ParamMutate: "Put",
					}),
				},
			},
		}
		partner := &sdk.Function{Name: "Put", Package: "x"}
		mixintest.RunWithResolver(t, streamreflectsmutations.Mixin(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host, partner},
		})

		key := shape.MixinParamKey(
			streamreflectsmutations.Name,
			streamreflectsmutations.ParamMutate,
		)
		got, _ := key.Get(host.Meta())
		if got != "x.Put" {
			t.Fatalf("mutate param = %q, want %q", got, "x.Put")
		}
	})
}
