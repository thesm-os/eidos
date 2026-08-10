// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package multiaggregator_test

import (
	"testing"

	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/multiaggregator"
	"go.thesmos.sh/eidos/sdk"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := multiaggregator.Detector()
	if det.Name != multiaggregator.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, multiaggregator.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_MatchesTwoValues(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Pair", Package: "x",
		Params: []*sdk.Param{dt.Param("ctx", dt.Ctx())},
		Returns: sdk.AnonReturns(
			dt.Named("int"),
			dt.Qualified("x", "Article"),
			dt.Err(),
		),
	}
	bag := dt.RunFn(t, multiaggregator.Detector(), fn)
	dt.AssertShape(t, bag, multiaggregator.Name, "", "int")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *sdk.Function
	}{
		{"single value (Aggregator territory)", &sdk.Function{
			Name: "Count", Package: "x",
			Returns: sdk.AnonReturns(dt.Named("int"), dt.Err()),
		}},
		{"has non-ctx param (MultiReader territory)", &sdk.Function{
			Name: "Pair", Package: "x",
			Params: []*sdk.Param{dt.Param("id", dt.Named("string"))},
			Returns: sdk.AnonReturns(
				dt.Named("int"),
				dt.Named("int"),
				dt.Err(),
			),
		}},
		{"no error return", &sdk.Function{
			Name: "Pair", Package: "x",
			Returns: sdk.AnonReturns(dt.Named("int"), dt.Named("int")),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, multiaggregator.Detector(), tc.fn))
		})
	}
}
