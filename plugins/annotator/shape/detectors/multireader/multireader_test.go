// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package multireader_test

import (
	"testing"

	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/multireader"
	"go.thesmos.sh/eidos/sdk"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := multireader.Detector()
	if det.Name != multireader.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, multireader.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_MatchesMultipleValues(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Get", Package: "x",
		Params: []*sdk.Param{
			dt.Param("ctx", dt.Ctx()),
			dt.Param("id", dt.Named("string")),
		},
		Returns: sdk.AnonReturns(
			dt.Qualified("x", "Article"),
			dt.Qualified("x", "Meta"),
			dt.Err(),
		),
	}
	bag := dt.RunFn(t, multireader.Detector(), fn)
	dt.AssertShape(t, bag, multireader.Name, "string", "x.Article")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *sdk.Function
	}{
		{"single value (Reader territory)", &sdk.Function{
			Name: "Get", Package: "x",
			Params: []*sdk.Param{dt.Param("id", dt.Named("string"))},
			Returns: sdk.AnonReturns(
				dt.Qualified("x", "Article"),
				dt.Err(),
			),
		}},
		{"no params (MultiAggregator territory)", &sdk.Function{
			Name: "Pair", Package: "x",
			Returns: sdk.AnonReturns(
				dt.Named("int"),
				dt.Named("int"),
				dt.Err(),
			),
		}},
		{"no error return (ReaderWithBool / Lookup territory)", &sdk.Function{
			Name: "Get", Package: "x",
			Params: []*sdk.Param{dt.Param("id", dt.Named("string"))},
			Returns: sdk.AnonReturns(
				dt.Qualified("x", "Article"),
				dt.Qualified("x", "Meta"),
			),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, multireader.Detector(), tc.fn))
		})
	}
}
