// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package batchreader_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/batchreader"
	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := batchreader.Detector()
	if det.Name != batchreader.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, batchreader.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_Matches(t *testing.T) {
	t.Parallel()
	fn := &node.Function{
		Name: "GetAll", Package: "x",
		Params: []*node.Param{
			dt.Param("ctx", dt.Ctx()),
			dt.Variadic("ids", dt.Named("string")),
		},
		Returns: node.AnonReturns(
			dt.Slice(dt.Qualified("x", "Article")),
			dt.Err(),
		),
	}
	bag := dt.RunFn(t, batchreader.Detector(), fn)
	dt.AssertShape(t, bag, batchreader.Name, "string", "x.Article")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *node.Function
	}{
		{"non-variadic key (Reader territory)", &node.Function{
			Name: "GetAll", Package: "x",
			Params: []*node.Param{dt.Param("id", dt.Named("string"))},
			Returns: node.AnonReturns(
				dt.Slice(dt.Qualified("x", "Article")),
				dt.Err(),
			),
		}},
		{"non-slice value (Reader territory)", &node.Function{
			Name: "Get", Package: "x",
			Params: []*node.Param{dt.Variadic("ids", dt.Named("string"))},
			Returns: node.AnonReturns(
				dt.Qualified("x", "Article"),
				dt.Err(),
			),
		}},
		{"no error return", &node.Function{
			Name: "GetAll", Package: "x",
			Params: []*node.Param{dt.Variadic("ids", dt.Named("string"))},
			Returns: node.AnonReturns(
				dt.Slice(dt.Qualified("x", "Article")),
			),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, batchreader.Detector(), tc.fn))
		})
	}
}
