// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package mutator_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/mutator"
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
	fn := &node.Function{
		Name: "Set", Package: "x",
		Params: []*node.Param{
			dt.Param("ctx", dt.Ctx()),
			dt.Param("v", dt.Qualified("x", "Article")),
		},
	}
	bag := dt.RunFn(t, mutator.Detector(), fn)
	dt.AssertShape(t, bag, mutator.Name, "", "x.Article")
}

func TestDetector_MatchesPointerValue(t *testing.T) {
	t.Parallel()
	fn := &node.Function{
		Name: "Set", Package: "x",
		Params: []*node.Param{
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
		fn   *node.Function
	}{
		{"has error return (Writer territory)", &node.Function{
			Name: "Save", Package: "x",
			Params:  []*node.Param{dt.Param("v", dt.Qualified("x", "Article"))},
			Returns: node.AnonReturns(dt.Err()),
		}},
		{"no params (Lifecycle territory)", &node.Function{
			Name: "Tick", Package: "x",
			Params: []*node.Param{dt.Param("ctx", dt.Ctx())},
		}},
		{"two non-ctx params (CompositeWriter territory)", &node.Function{
			Name: "X", Package: "x",
			Params: []*node.Param{
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
