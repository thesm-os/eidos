// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package lookup_test

import (
	"testing"

	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/lookup"
	"go.thesmos.sh/eidos/sdk"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := lookup.Detector()
	if det.Name != lookup.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, lookup.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_Matches(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Lookup", Package: "x",
		Params: []*sdk.Param{
			dt.Param("ctx", dt.Ctx()),
			dt.Param("id", dt.Named("string")),
		},
		Returns: sdk.AnonReturns(
			dt.Qualified("x", "Article"),
			dt.Qualified("x", "Meta"),
			dt.Named("bool"),
		),
	}
	bag := dt.RunFn(t, lookup.Detector(), fn)
	dt.AssertShape(t, bag, lookup.Name, "string", "x.Article")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *sdk.Function
	}{
		{"two returns (ReaderWithBool territory)", &sdk.Function{
			Name: "Find", Package: "x",
			Params: []*sdk.Param{dt.Param("id", dt.Named("string"))},
			Returns: sdk.AnonReturns(
				dt.Qualified("x", "Article"),
				dt.Named("bool"),
			),
		}},
		{"third return is not bool (MultiReader territory)", &sdk.Function{
			Name: "Find", Package: "x",
			Params: []*sdk.Param{dt.Param("id", dt.Named("string"))},
			Returns: sdk.AnonReturns(
				dt.Qualified("x", "Article"),
				dt.Qualified("x", "Meta"),
				dt.Err(),
			),
		}},
		{"no key param", &sdk.Function{
			Name: "Find", Package: "x",
			Returns: sdk.AnonReturns(
				dt.Qualified("x", "Article"),
				dt.Qualified("x", "Meta"),
				dt.Named("bool"),
			),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, lookup.Detector(), tc.fn))
		})
	}
}
