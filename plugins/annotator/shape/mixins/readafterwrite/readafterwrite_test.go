// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package readafterwrite_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/readafterwrite"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t,
		readafterwrite.Mixin(),
		readafterwrite.Name, readafterwrite.Params)
}

// TestMixin_PipelineStamping exercises the umbrella plugin
// stamping a real `+gen:mixin readafterwrite write=Save`
// directive — the mixin must be attached and the `write`
// parameter must reach its per-mixin meta key.
func TestMixin_PipelineStamping(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Find", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				mixintest.HostDirective(readafterwrite.Name, map[string]string{
					"write": "Save",
				}),
			},
		},
	}
	bag := mixintest.RunPipeline(t, readafterwrite.Mixin(), fn)
	mixintest.AssertAttached(t, bag, readafterwrite.Name)
	mixintest.AssertParam(t, bag, readafterwrite.Name, "write", "Save")
}
