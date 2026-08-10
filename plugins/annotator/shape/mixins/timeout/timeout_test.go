// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package timeout_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/timeout"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, timeout.Mixin(), timeout.Name, timeout.Params)
	})

	t.Run("pipeline stamps duration param", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Charge", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(timeout.Name, map[string]string{
						"duration": "5s",
					}),
				},
			},
		}
		bag := mixintest.RunPipeline(t, timeout.Mixin(), fn)
		mixintest.AssertAttached(t, bag, timeout.Name)
		mixintest.AssertParam(t, bag, timeout.Name, "duration", "5s")
		// Belt and braces: confirm the value really sits under
		// the per-mixin param key the contract documents.
		if got, _ := shape.MixinParamKey(timeout.Name, "duration").Get(bag); got != "5s" {
			t.Fatalf("MixinParamKey resolved %q, want %q", got, "5s")
		}
	})
}
