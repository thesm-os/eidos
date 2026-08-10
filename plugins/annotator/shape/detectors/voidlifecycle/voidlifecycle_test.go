// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package voidlifecycle_test

import (
	"testing"

	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/voidlifecycle"
	"go.thesmos.sh/eidos/sdk"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := voidlifecycle.Detector()
	if det.Name != voidlifecycle.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, voidlifecycle.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_MatchesVoidVoid(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{Name: "Side", Package: "x"}
	bag := dt.RunFn(t, voidlifecycle.Detector(), fn)
	dt.AssertShape(t, bag, voidlifecycle.Name, "", "")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *sdk.Function
	}{
		{"has params", &sdk.Function{
			Name: "X", Package: "x",
			Params: []*sdk.Param{dt.Param("a", dt.Named("string"))},
		}},
		{"has returns", &sdk.Function{
			Name: "X", Package: "x",
			Returns: sdk.AnonReturns(dt.Err()),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, voidlifecycle.Detector(), tc.fn))
		})
	}
}
