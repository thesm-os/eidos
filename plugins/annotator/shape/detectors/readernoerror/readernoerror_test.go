// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package readernoerror_test

import (
	"testing"

	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/readernoerror"
	"go.thesmos.sh/eidos/sdk"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := readernoerror.Detector()
	if det.Name != readernoerror.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, readernoerror.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_Matches(t *testing.T) {
	t.Parallel()

	t.Run("(ctx, K) V", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Get", Package: "x",
			Params: []*sdk.Param{
				dt.Param("ctx", dt.Ctx()),
				dt.Param("id", dt.Named("string")),
			},
			Returns: sdk.AnonReturns(dt.Qualified("x", "Article")),
		}
		bag := dt.RunFn(t, readernoerror.Detector(), fn)
		dt.AssertShape(t, bag, readernoerror.Name, "string", "x.Article")
	})

	t.Run("(K) V", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Get", Package: "x",
			Params:  []*sdk.Param{dt.Param("id", dt.Named("string"))},
			Returns: sdk.AnonReturns(dt.Qualified("x", "Article")),
		}
		bag := dt.RunFn(t, readernoerror.Detector(), fn)
		dt.AssertShape(t, bag, readernoerror.Name, "string", "x.Article")
	})
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *sdk.Function
	}{
		{"has error return (Reader territory)", &sdk.Function{
			Name: "Get", Package: "x",
			Params: []*sdk.Param{dt.Param("id", dt.Named("string"))},
			Returns: sdk.AnonReturns(
				dt.Qualified("x", "Article"),
				dt.Err(),
			),
		}},
		{"no params (Aggregator territory)", &sdk.Function{
			Name: "Count", Package: "x",
			Returns: sdk.AnonReturns(dt.Named("int")),
		}},
		{"void return (Mutator territory)", &sdk.Function{
			Name: "Set", Package: "x",
			Params: []*sdk.Param{dt.Param("v", dt.Qualified("x", "Article"))},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, readernoerror.Detector(), tc.fn))
		})
	}
}
