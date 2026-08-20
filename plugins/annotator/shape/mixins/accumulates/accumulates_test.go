// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package accumulates_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/accumulates"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, accumulates.Mixin(), accumulates.Name, nil)
}

func TestMixin_Attaches(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Append", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				mixintest.HostDirective(accumulates.Name, nil),
			},
		},
	}
	bag := mixintest.RunPipeline(t, accumulates.Mixin(), fn)
	mixintest.AssertAttached(t, bag, accumulates.Name)
}
