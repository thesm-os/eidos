// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package streamreader_test

import (
	"testing"

	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/streamreader"
	"go.thesmos.sh/eidos/sdk"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := streamreader.Detector()
	if det.Name != streamreader.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, streamreader.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_MatchesSeq(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "All", Package: "x",
		Params:  []*sdk.Param{dt.Param("ctx", dt.Ctx())},
		Returns: sdk.AnonReturns(dt.IterSeq(dt.Qualified("x", "Article"))),
	}
	bag := dt.RunFn(t, streamreader.Detector(), fn)
	dt.AssertShape(t, bag, streamreader.Name, "", "x.Article")
}

func TestDetector_MatchesSeqWithKey(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "AllWithFilter", Package: "x",
		Params: []*sdk.Param{
			dt.Param("ctx", dt.Ctx()),
			dt.Param("category", dt.Named("string")),
		},
		Returns: sdk.AnonReturns(dt.IterSeq(dt.Qualified("x", "Article"))),
	}
	bag := dt.RunFn(t, streamreader.Detector(), fn)
	dt.AssertShape(t, bag, streamreader.Name, "string", "x.Article")
}

func TestDetector_MatchesSeq2(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "All", Package: "x",
		Params: []*sdk.Param{dt.Param("ctx", dt.Ctx())},
		Returns: sdk.AnonReturns(
			dt.IterSeq2(dt.Qualified("x", "Article"), dt.Err()),
		),
	}
	bag := dt.RunFn(t, streamreader.Detector(), fn)
	dt.AssertShape(t, bag, streamreader.Name, "", "x.Article")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *sdk.Function
	}{
		{"non-iter return (Reader / Aggregator territory)", &sdk.Function{
			Name: "Get", Package: "x",
			Params:  []*sdk.Param{dt.Param("ctx", dt.Ctx())},
			Returns: sdk.AnonReturns(dt.Qualified("x", "Article")),
		}},
		{"multiple returns", &sdk.Function{
			Name: "All", Package: "x",
			Returns: sdk.AnonReturns(
				dt.IterSeq(dt.Qualified("x", "Article")),
				dt.Err(),
			),
		}},
		{"too many input keys", &sdk.Function{
			Name: "All", Package: "x",
			Params: []*sdk.Param{
				dt.Param("a", dt.Named("string")),
				dt.Param("b", dt.Named("string")),
			},
			Returns: sdk.AnonReturns(dt.IterSeq(dt.Qualified("x", "Article"))),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, streamreader.Detector(), tc.fn))
		})
	}
}
