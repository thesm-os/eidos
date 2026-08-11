// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package windowed_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/windowed"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, windowed.Mixin(), windowed.Name, windowed.Params)
	})

	t.Run("every declared partner resolves to a qualified name", func(t *testing.T) {
		t.Parallel()
		// A law selecting this mixin calls the partners, so a stamp
		// left as a bare name gives the binding nothing to call — and a
		// law that never calls reports every implementation correct.
		host := &sdk.Function{
			Name: "Rate", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(windowed.Name, map[string]string{
						windowed.ParamIncr:  "Record",
						windowed.ParamCount: "CountIn",
					}),
				},
			},
		}
		fns := []*sdk.Function{host, {Name: "Record", Package: "x"}, {Name: "CountIn", Package: "x"}}
		mixintest.RunWithResolver(t, windowed.Mixin(), &sdk.Package{
			Name: "x", Path: "x", Functions: fns,
		})

		keyParamIncr := shape.MixinParamKey(windowed.Name, windowed.ParamIncr)
		if got, _ := keyParamIncr.Get(host.Meta()); got != "x.Record" {
			t.Errorf("incr = %q, want %q", got, "x.Record")
		}
		keyParamCount := shape.MixinParamKey(windowed.Name, windowed.ParamCount)
		if got, _ := keyParamCount.Get(host.Meta()); got != "x.CountIn" {
			t.Errorf("count = %q, want %q", got, "x.CountIn")
		}
	})
}
