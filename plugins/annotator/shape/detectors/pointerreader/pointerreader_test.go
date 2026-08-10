// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pointerreader_test

import (
	"testing"

	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/pointerreader"
	"go.thesmos.sh/eidos/sdk"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := pointerreader.Detector()
	if det.Name != pointerreader.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, pointerreader.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_Matches(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Get", Package: "x",
		Params: []*sdk.Param{
			dt.Param("ctx", dt.Ctx()),
			dt.Param("id", dt.Named("string")),
		},
		Returns: sdk.AnonReturns(dt.Pointer(dt.Qualified("x", "Article"))),
	}
	bag := dt.RunFn(t, pointerreader.Detector(), fn)
	dt.AssertShape(t, bag, pointerreader.Name, "string", "x.Article")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *sdk.Function
	}{
		{"non-pointer return (ReaderNoError territory)", &sdk.Function{
			Name: "Get", Package: "x",
			Params:  []*sdk.Param{dt.Param("id", dt.Named("string"))},
			Returns: sdk.AnonReturns(dt.Qualified("x", "Article")),
		}},
		{"has error return (Reader territory)", &sdk.Function{
			Name: "Get", Package: "x",
			Params: []*sdk.Param{dt.Param("id", dt.Named("string"))},
			Returns: sdk.AnonReturns(
				dt.Pointer(dt.Qualified("x", "Article")),
				dt.Err(),
			),
		}},
		{"no params (Aggregator territory)", &sdk.Function{
			Name: "First", Package: "x",
			Returns: sdk.AnonReturns(dt.Pointer(dt.Qualified("x", "Article"))),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, pointerreader.Detector(), tc.fn))
		})
	}
}
