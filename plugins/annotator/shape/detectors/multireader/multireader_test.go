// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package multireader_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/multireader"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := multireader.Detector()
	if det.Name != multireader.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, multireader.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_MatchesMultipleValues(t *testing.T) {
	t.Parallel()
	fn := &node.Function{
		Name: "Get", Package: "x",
		Params: []*node.Param{
			dt.Param("ctx", dt.Ctx()),
			dt.Param("id", dt.Named("string")),
		},
		Returns: node.AnonReturns(
			dt.Qualified("x", "Article"),
			dt.Qualified("x", "Meta"),
			dt.Err(),
		),
	}
	bag := dt.RunFn(t, multireader.Detector(), fn)
	dt.AssertShape(t, bag, multireader.Name, "string", "x.Article")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *node.Function
	}{
		{"single value (Reader territory)", &node.Function{
			Name: "Get", Package: "x",
			Params: []*node.Param{dt.Param("id", dt.Named("string"))},
			Returns: node.AnonReturns(
				dt.Qualified("x", "Article"),
				dt.Err(),
			),
		}},
		{"no params (MultiAggregator territory)", &node.Function{
			Name: "Pair", Package: "x",
			Returns: node.AnonReturns(
				dt.Named("int"),
				dt.Named("int"),
				dt.Err(),
			),
		}},
		{"no error return (ReaderWithBool / Lookup territory)", &node.Function{
			Name: "Get", Package: "x",
			Params: []*node.Param{dt.Param("id", dt.Named("string"))},
			Returns: node.AnonReturns(
				dt.Qualified("x", "Article"),
				dt.Qualified("x", "Meta"),
			),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, multireader.Detector(), tc.fn))
		})
	}
}
