// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package monotonic_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/monotonic"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, monotonic.Mixin(), monotonic.Name, nil)
}

// TestMixin_PipelineStamping exercises the umbrella plugin
// stamping a real `+gen:mixin monotonic` directive — proving
// the package's [Mixin] value plugs into the framework
// correctly.
func TestMixin_PipelineStamping(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Tick", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				mixintest.HostDirective(monotonic.Name, nil),
			},
		},
	}
	bag := mixintest.RunPipeline(t, monotonic.Mixin(), fn)
	mixintest.AssertAttached(t, bag, monotonic.Name)
}
