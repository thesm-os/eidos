// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package predicate_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/predicate"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := predicate.Detector()
	if det.Name != predicate.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, predicate.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_MatchesPredicate(t *testing.T) {
	t.Parallel()
	fn := &node.Function{
		Name: "Ready", Package: "x",
		Returns: node.AnonReturns(dt.Named("bool")),
	}
	bag := dt.RunFn(t, predicate.Detector(), fn)
	dt.AssertShape(t, bag, predicate.Name, "", "")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *node.Function
	}{
		{"void return", &node.Function{Name: "X", Package: "x"}},
		{"error return", &node.Function{
			Name: "X", Package: "x",
			Returns: node.AnonReturns(dt.Err()),
		}},
		{"qualified bool is not a builtin", &node.Function{
			Name: "X", Package: "x",
			Returns: node.AnonReturns(dt.Qualified("x", "bool")),
		}},
		{"has params", &node.Function{
			Name: "X", Package: "x",
			Params:  []*node.Param{dt.Param("a", dt.Named("string"))},
			Returns: node.AnonReturns(dt.Named("bool")),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, predicate.Detector(), tc.fn))
		})
	}
}
