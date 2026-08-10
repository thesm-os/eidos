// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package multiargwriter_test

import (
	"testing"

	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/multiargwriter"
	"go.thesmos.sh/eidos/sdk"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := multiargwriter.Detector()
	if det.Name != multiargwriter.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, multiargwriter.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_MatchesThreeArgs(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Record", Package: "x",
		Params: []*sdk.Param{
			dt.Param("ctx", dt.Ctx()),
			dt.Param("a", dt.Named("string")),
			dt.Param("b", dt.Named("string")),
			dt.Param("c", dt.Named("string")),
		},
		Returns: sdk.AnonReturns(dt.Err()),
	}
	bag := dt.RunFn(t, multiargwriter.Detector(), fn)
	dt.AssertShape(t, bag, multiargwriter.Name, "", "")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *sdk.Function
	}{
		{"two args (CompositeWriter territory)", &sdk.Function{
			Name: "Set", Package: "x",
			Params: []*sdk.Param{
				dt.Param("k", dt.Named("string")),
				dt.Param("v", dt.Qualified("x", "Article")),
			},
			Returns: sdk.AnonReturns(dt.Err()),
		}},
		{"no error return", &sdk.Function{
			Name: "Record", Package: "x",
			Params: []*sdk.Param{
				dt.Param("a", dt.Named("string")),
				dt.Param("b", dt.Named("string")),
				dt.Param("c", dt.Named("string")),
			},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, multiargwriter.Detector(), tc.fn))
		})
	}
}
