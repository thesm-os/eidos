// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package bounded_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/bounded"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, bounded.Mixin(), bounded.Name, bounded.Params)
	})

	t.Run("pipeline stamps limit param", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Send", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(bounded.Name, map[string]string{
						"limit": "100",
					}),
				},
			},
		}
		bag := mixintest.RunPipeline(t, bounded.Mixin(), fn)
		mixintest.AssertAttached(t, bag, bounded.Name)
		mixintest.AssertParam(t, bag, bounded.Name, "limit", "100")
	})
}
