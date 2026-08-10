// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package batchreader_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/batchreader"
	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/sdk"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := batchreader.Detector()
	if det.Name != batchreader.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, batchreader.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_Matches(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "GetAll", Package: "x",
		Params: []*sdk.Param{
			dt.Param("ctx", dt.Ctx()),
			dt.Variadic("ids", dt.Named("string")),
		},
		Returns: sdk.AnonReturns(
			dt.Slice(dt.Qualified("x", "Article")),
			dt.Err(),
		),
	}
	bag := dt.RunFn(t, batchreader.Detector(), fn)
	dt.AssertShape(t, bag, batchreader.Name, "string", "x.Article")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *sdk.Function
	}{
		{"non-variadic key (Reader territory)", &sdk.Function{
			Name: "GetAll", Package: "x",
			Params: []*sdk.Param{dt.Param("id", dt.Named("string"))},
			Returns: sdk.AnonReturns(
				dt.Slice(dt.Qualified("x", "Article")),
				dt.Err(),
			),
		}},
		{"non-slice value (Reader territory)", &sdk.Function{
			Name: "Get", Package: "x",
			Params: []*sdk.Param{dt.Variadic("ids", dt.Named("string"))},
			Returns: sdk.AnonReturns(
				dt.Qualified("x", "Article"),
				dt.Err(),
			),
		}},
		{"no error return", &sdk.Function{
			Name: "GetAll", Package: "x",
			Params: []*sdk.Param{dt.Variadic("ids", dt.Named("string"))},
			Returns: sdk.AnonReturns(
				dt.Slice(dt.Qualified("x", "Article")),
			),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, batchreader.Detector(), tc.fn))
		})
	}
}
