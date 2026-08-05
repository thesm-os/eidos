// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package poisonaccessor_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/poisonaccessor"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := poisonaccessor.Detector()
	if det.Name != poisonaccessor.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, poisonaccessor.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_MatchesPoison(t *testing.T) {
	t.Parallel()
	fn := &node.Function{
		Name: "Check", Package: "x",
		Returns: node.AnonReturns(dt.Err()),
	}
	bag := dt.RunFn(t, poisonaccessor.Detector(), fn)
	dt.AssertShape(t, bag, poisonaccessor.Name, "", "")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *node.Function
	}{
		{"has params (Lifecycle territory)", &node.Function{
			Name: "Check", Package: "x",
			Params:  []*node.Param{dt.Param("ctx", dt.Ctx())},
			Returns: node.AnonReturns(dt.Err()),
		}},
		{"non-error single return (Predicate territory)", &node.Function{
			Name: "Check", Package: "x",
			Returns: node.AnonReturns(dt.Named("bool")),
		}},
		{"void return (VoidLifecycle territory)", &node.Function{
			Name: "Check", Package: "x",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, poisonaccessor.Detector(), tc.fn))
		})
	}
}
