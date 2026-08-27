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
						eventually.ParamSettle:  "Flush",
						eventually.ParamSync:    "InSync",
						eventually.ParamObserve: "Items",
					}),
				},
			},
		}
		fns := []*sdk.Function{
			host,
			{Name: "Flush", Package: "x"},
			{Name: "InSync", Package: "x"},
			{Name: "Items", Package: "x"},
		}
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
		// The third part of "settle, then observe". Settle and sync
		// both describe reaching quiescence; without this one the
		// sentence had no observation.
		keyParamObserve := shape.MixinParamKey(eventually.Name, eventually.ParamObserve)
		if got, _ := keyParamObserve.Get(host.Meta()); got != "x.Items" {
			t.Errorf("observe = %q, want %q", got, "x.Items")
		}
	})

	t.Run("the bare form still classifies", func(t *testing.T) {
		t.Parallel()
		// Every partner is optional: a publisher whose effect arrives
		// eventually is what this names whether or not the author
		// points at one, and requiring a key would retire the
		// classification for every subject already carrying it.
		host := &sdk.Function{
			Name: "Put", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(eventually.Name, map[string]string{}),
				},
			},
		}
		pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{host}}
		for _, d := range mixintest.RunWithValidator(t, eventually.Mixin(), pkg) {
			if d.Severity == sdk.SeverityError {
				t.Fatalf("bare eventually was refused: %s", d.Message)
			}
		}
	})
}
