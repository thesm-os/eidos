// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package scheduled_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/scheduled"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, scheduled.Mixin(), scheduled.Name, scheduled.Params)
	})

	t.Run("both partners resolve to qualified names", func(t *testing.T) {
		t.Parallel()
		// The law compares the fired count across a clock advance, so a
		// suite that cannot count firings reports every scheduler as
		// correct — including one that never fires.
		host := &sdk.Function{
			Name: "Timer", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(scheduled.Name, map[string]string{
						scheduled.ParamSchedule: "After",
						scheduled.ParamFired:    "FiredCount",
					}),
				},
			},
		}
		mixintest.RunWithResolver(t, scheduled.Mixin(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{
				host,
				{Name: "After", Package: "x"},
				{Name: "FiredCount", Package: "x"},
			},
		})

		sch, _ := shape.MixinParamKey(scheduled.Name, scheduled.ParamSchedule).Get(host.Meta())
		fired, _ := shape.MixinParamKey(scheduled.Name, scheduled.ParamFired).Get(host.Meta())
		if sch != "x.After" || fired != "x.FiredCount" {
			t.Fatalf("schedule/fired = %q/%q, want x.After/x.FiredCount", sch, fired)
		}
	})
}
