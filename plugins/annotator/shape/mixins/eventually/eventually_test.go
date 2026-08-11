// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package eventually_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/eventually"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, eventually.Mixin(), eventually.Name, eventually.Params)
	})

	t.Run("every declared partner resolves to a qualified name", func(t *testing.T) {
		t.Parallel()
		// A law selecting this mixin calls the partners, so a stamp
		// left as a bare name gives the binding nothing to call — and a
		// law that never calls reports every implementation correct.
		host := &sdk.Function{
			Name: "Put", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(eventually.Name, map[string]string{
						eventually.ParamSettle: "Flush",
						eventually.ParamSync:   "InSync",
					}),
				},
			},
		}
		fns := []*sdk.Function{host, {Name: "Flush", Package: "x"}, {Name: "InSync", Package: "x"}}
		mixintest.RunWithResolver(t, eventually.Mixin(), &sdk.Package{
			Name: "x", Path: "x", Functions: fns,
		})

		keyParamSettle := shape.MixinParamKey(eventually.Name, eventually.ParamSettle)
		if got, _ := keyParamSettle.Get(host.Meta()); got != "x.Flush" {
			t.Errorf("settle = %q, want %q", got, "x.Flush")
		}
		keyParamSync := shape.MixinParamKey(eventually.Name, eventually.ParamSync)
		if got, _ := keyParamSync.Get(host.Meta()); got != "x.InSync" {
			t.Errorf("sync = %q, want %q", got, "x.InSync")
		}
	})
}
