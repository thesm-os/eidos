// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package readerwithbool_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/readerwithbool"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := readerwithbool.Detector()
	if det.Name != readerwithbool.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, readerwithbool.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_Matches(t *testing.T) {
	t.Parallel()
	fn := &node.Function{
		Name: "Find", Package: "x",
		Params: []*node.Param{
			dt.Param("ctx", dt.Ctx()),
			dt.Param("id", dt.Named("string")),
		},
		Returns: node.AnonReturns(
			dt.Qualified("x", "Article"),
			dt.Named("bool"),
		),
	}
	bag := dt.RunFn(t, readerwithbool.Detector(), fn)
	dt.AssertShape(t, bag, readerwithbool.Name, "string", "x.Article")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *node.Function
	}{
		{"second return is not bool (MultiReader territory)", &node.Function{
			Name: "Find", Package: "x",
			Params: []*node.Param{dt.Param("id", dt.Named("string"))},
			Returns: node.AnonReturns(
				dt.Qualified("x", "Article"),
				dt.Qualified("x", "Meta"),
			),
		}},
		{"three returns (Lookup territory)", &node.Function{
			Name: "Find", Package: "x",
			Params: []*node.Param{dt.Param("id", dt.Named("string"))},
			Returns: node.AnonReturns(
				dt.Qualified("x", "Article"),
				dt.Qualified("x", "Meta"),
				dt.Named("bool"),
			),
		}},
		{"no params (Aggregator territory)", &node.Function{
			Name: "Find", Package: "x",
			Returns: node.AnonReturns(
				dt.Qualified("x", "Article"),
				dt.Named("bool"),
			),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, readerwithbool.Detector(), tc.fn))
		})
	}
}
