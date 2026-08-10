// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package predicate_test

import (
	"testing"

	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/predicate"
	"go.thesmos.sh/eidos/sdk"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := predicate.Detector()
	if det.Name != predicate.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, predicate.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_MatchesPredicate(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Ready", Package: "x",
		Returns: sdk.AnonReturns(dt.Named("bool")),
	}
	bag := dt.RunFn(t, predicate.Detector(), fn)
	dt.AssertShape(t, bag, predicate.Name, "", "")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *sdk.Function
	}{
		{"void return", &sdk.Function{Name: "X", Package: "x"}},
		{"error return", &sdk.Function{
			Name: "X", Package: "x",
			Returns: sdk.AnonReturns(dt.Err()),
		}},
		{"qualified bool is not a builtin", &sdk.Function{
			Name: "X", Package: "x",
			Returns: sdk.AnonReturns(dt.Qualified("x", "bool")),
		}},
		{"has params", &sdk.Function{
			Name: "X", Package: "x",
			Params:  []*sdk.Param{dt.Param("a", dt.Named("string"))},
			Returns: sdk.AnonReturns(dt.Named("bool")),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, predicate.Detector(), tc.fn))
		})
	}
}
