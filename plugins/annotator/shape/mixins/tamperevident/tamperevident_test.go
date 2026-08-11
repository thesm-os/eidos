// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tamperevident_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/tamperevident"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, tamperevident.Mixin(), tamperevident.Name, tamperevident.Params)
	})

	t.Run("every declared partner resolves to a qualified name", func(t *testing.T) {
		t.Parallel()
		// A law selecting this mixin calls the partners, so a stamp
		// left as a bare name gives the binding nothing to call — and a
		// law that never calls reports every implementation correct.
		host := &sdk.Function{
			Name: "Append", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(tamperevident.Name, map[string]string{
						tamperevident.ParamTamper: "Corrupt",
						tamperevident.ParamVerify: "Check",
					}),
				},
			},
		}
		fns := []*sdk.Function{host, {Name: "Corrupt", Package: "x"}, {Name: "Check", Package: "x"}}
		mixintest.RunWithResolver(t, tamperevident.Mixin(), &sdk.Package{
			Name: "x", Path: "x", Functions: fns,
		})

		keyParamTamper := shape.MixinParamKey(tamperevident.Name, tamperevident.ParamTamper)
		if got, _ := keyParamTamper.Get(host.Meta()); got != "x.Corrupt" {
			t.Errorf("tamper = %q, want %q", got, "x.Corrupt")
		}
		keyParamVerify := shape.MixinParamKey(tamperevident.Name, tamperevident.ParamVerify)
		if got, _ := keyParamVerify.Get(host.Meta()); got != "x.Check" {
			t.Errorf("verify = %q, want %q", got, "x.Check")
		}
	})
}
