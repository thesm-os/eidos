// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package multiaggregator_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/multiaggregator"
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
	fn := &node.Function{
		Name: "Pair", Package: "x",
		Params: []*node.Param{dt.Param("ctx", dt.Ctx())},
		Returns: node.AnonReturns(
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
		fn   *node.Function
	}{
		{"single value (Aggregator territory)", &node.Function{
			Name: "Count", Package: "x",
			Returns: node.AnonReturns(dt.Named("int"), dt.Err()),
		}},
		{"has non-ctx param (MultiReader territory)", &node.Function{
			Name: "Pair", Package: "x",
			Params: []*node.Param{dt.Param("id", dt.Named("string"))},
			Returns: node.AnonReturns(
				dt.Named("int"),
				dt.Named("int"),
				dt.Err(),
			),
		}},
		{"no error return", &node.Function{
			Name: "Pair", Package: "x",
			Returns: node.AnonReturns(dt.Named("int"), dt.Named("int")),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, multiaggregator.Detector(), tc.fn))
		})
	}
}
