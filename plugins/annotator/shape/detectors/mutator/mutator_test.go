// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package mutator_test

import (
	"testing"

	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/mutator"
	"go.thesmos.sh/eidos/sdk"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := mutator.Detector()
	if det.Name != mutator.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, mutator.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_MatchesValueByValue(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Set", Package: "x",
		Params: []*sdk.Param{
			dt.Param("ctx", dt.Ctx()),
			dt.Param("v", dt.Qualified("x", "Article")),
		},
	}
	bag := dt.RunFn(t, mutator.Detector(), fn)
	dt.AssertShape(t, bag, mutator.Name, "", "x.Article")
}

func TestDetector_MatchesPointerValue(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Set", Package: "x",
		Params: []*sdk.Param{
			dt.Param("v", dt.Pointer(dt.Qualified("x", "Article"))),
		},
	}
	bag := dt.RunFn(t, mutator.Detector(), fn)
	dt.AssertShape(t, bag, mutator.Name, "", "x.Article")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *sdk.Function
	}{
		{"has error return (Writer territory)", &sdk.Function{
			Name: "Save", Package: "x",
			Params:  []*sdk.Param{dt.Param("v", dt.Qualified("x", "Article"))},
			Returns: sdk.AnonReturns(dt.Err()),
		}},
		{"no params (Lifecycle territory)", &sdk.Function{
			Name: "Tick", Package: "x",
			Params: []*sdk.Param{dt.Param("ctx", dt.Ctx())},
		}},
		{"two non-ctx params (CompositeWriter territory)", &sdk.Function{
			Name: "X", Package: "x",
			Params: []*sdk.Param{
				dt.Param("k", dt.Named("string")),
				dt.Param("v", dt.Qualified("x", "Article")),
			},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, mutator.Detector(), tc.fn))
		})
	}
}
