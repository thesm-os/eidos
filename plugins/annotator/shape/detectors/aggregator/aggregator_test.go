// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package aggregator_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/aggregator"
	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/sdk"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := aggregator.Detector()
	if det.Name != aggregator.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, aggregator.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_Matches(t *testing.T) {
	t.Parallel()

	t.Run("(ctx) (T, error)", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Count", Package: "x",
			Params: []*sdk.Param{dt.Param("ctx", dt.Ctx())},
			Returns: sdk.AnonReturns(
				dt.Named("int"),
				dt.Err(),
			),
		}
		bag := dt.RunFn(t, aggregator.Detector(), fn)
		dt.AssertShape(t, bag, aggregator.Name, "", "int")
	})

	t.Run("(ctx) T", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Count", Package: "x",
			Params:  []*sdk.Param{dt.Param("ctx", dt.Ctx())},
			Returns: sdk.AnonReturns(dt.Named("int")),
		}
		bag := dt.RunFn(t, aggregator.Detector(), fn)
		dt.AssertShape(t, bag, aggregator.Name, "", "int")
	})

	t.Run("() T", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Count", Package: "x",
			Returns: sdk.AnonReturns(dt.Named("int")),
		}
		bag := dt.RunFn(t, aggregator.Detector(), fn)
		dt.AssertShape(t, bag, aggregator.Name, "", "int")
	})
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *sdk.Function
	}{
		{"has non-ctx param (Reader / ReaderNoError territory)", &sdk.Function{
			Name: "Find", Package: "x",
			Params:  []*sdk.Param{dt.Param("id", dt.Named("string"))},
			Returns: sdk.AnonReturns(dt.Named("int")),
		}},
		{"two values + error (MultiAggregator territory)", &sdk.Function{
			Name: "Stats", Package: "x",
			Returns: sdk.AnonReturns(
				dt.Named("int"),
				dt.Named("int"),
				dt.Err(),
			),
		}},
		{"void return (Lifecycle / VoidLifecycle territory)", &sdk.Function{
			Name: "Tick", Package: "x",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, aggregator.Detector(), tc.fn))
		})
	}
}
