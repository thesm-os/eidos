// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package compositewriter_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/compositewriter"
	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := compositewriter.Detector()
	if det.Name != compositewriter.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, compositewriter.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_Matches(t *testing.T) {
	t.Parallel()
	fn := &node.Function{
		Name: "Set", Package: "x",
		Params: []*node.Param{
			dt.Param("ctx", dt.Ctx()),
			dt.Param("k", dt.Named("string")),
			dt.Param("v", dt.Qualified("x", "Article")),
		},
		Returns: node.AnonReturns(dt.Err()),
	}
	bag := dt.RunFn(t, compositewriter.Detector(), fn)
	dt.AssertShape(t, bag, compositewriter.Name, "string", "x.Article")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *node.Function
	}{
		{"one non-ctx param (Writer territory)", &node.Function{
			Name: "Save", Package: "x",
			Params:  []*node.Param{dt.Param("v", dt.Qualified("x", "Article"))},
			Returns: node.AnonReturns(dt.Err()),
		}},
		{"three non-ctx params (MultiArgWriter territory)", &node.Function{
			Name: "Save", Package: "x",
			Params: []*node.Param{
				dt.Param("a", dt.Named("string")),
				dt.Param("b", dt.Named("string")),
				dt.Param("c", dt.Named("string")),
			},
			Returns: node.AnonReturns(dt.Err()),
		}},
		{"no error return", &node.Function{
			Name: "Set", Package: "x",
			Params: []*node.Param{
				dt.Param("k", dt.Named("string")),
				dt.Param("v", dt.Qualified("x", "Article")),
			},
		}},
		{"with-result variant", &node.Function{
			Name: "Set", Package: "x",
			Params: []*node.Param{
				dt.Param("k", dt.Named("string")),
				dt.Param("v", dt.Qualified("x", "Article")),
			},
			Returns: node.AnonReturns(
				dt.Qualified("x", "Result"),
				dt.Err(),
			),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, compositewriter.Detector(), tc.fn))
		})
	}
}
