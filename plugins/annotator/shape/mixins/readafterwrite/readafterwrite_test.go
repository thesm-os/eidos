// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package readafterwrite_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
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

// TestMixin_ResolverRewritesWrite pins the behaviour the shape package
// documented in three places and the mixin did not implement.
//
// `SiblingParams` names readafterwrite as its worked example
// (`mixin.go:44`), `Validate`'s docblock offers "every readafterwrite's
// write partner resolves to a known callable" as the invariant to check
// (`mixin.go:55`), and the directive syntax block spells
// `readafterwrite write=Save` (`mixin.go:103`) — while the mixin
// declared only Params, so the rewrite never ran and `Save` stayed a
// bare name with no package and no owner.
func TestMixin_ResolverRewritesWrite(t *testing.T) {
	t.Parallel()

	host := &sdk.Function{
		Name: "Find", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				mixintest.HostDirective(readafterwrite.Name, map[string]string{
					readafterwrite.ParamWrite: "Save",
				}),
			},
		},
	}
	save := &sdk.Function{Name: "Save", Package: "x"}
	mixintest.RunWithResolver(t, readafterwrite.Mixin(), &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{host, save},
	})

	got, _ := shape.MixinParamKey(readafterwrite.Name, readafterwrite.ParamWrite).Get(host.Meta())
	if got != "x.Save" {
		t.Fatalf("write param = %q, want %q", got, "x.Save")
	}
}
