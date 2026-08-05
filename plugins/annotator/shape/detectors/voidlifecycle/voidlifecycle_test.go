// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package voidlifecycle_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/voidlifecycle"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := voidlifecycle.Detector()
	if det.Name != voidlifecycle.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, voidlifecycle.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_MatchesVoidVoid(t *testing.T) {
	t.Parallel()
	fn := &node.Function{Name: "Side", Package: "x"}
	bag := dt.RunFn(t, voidlifecycle.Detector(), fn)
	dt.AssertShape(t, bag, voidlifecycle.Name, "", "")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *node.Function
	}{
		{"has params", &node.Function{
			Name: "X", Package: "x",
			Params: []*node.Param{dt.Param("a", dt.Named("string"))},
		}},
		{"has returns", &node.Function{
			Name: "X", Package: "x",
			Returns: node.AnonReturns(dt.Err()),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, voidlifecycle.Detector(), tc.fn))
		})
	}
}
