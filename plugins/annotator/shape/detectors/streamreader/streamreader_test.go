// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package streamreader_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/streamreader"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := streamreader.Detector()
	if det.Name != streamreader.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, streamreader.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_MatchesSeq(t *testing.T) {
	t.Parallel()
	fn := &node.Function{
		Name: "All", Package: "x",
		Params:  []*node.Param{dt.Param("ctx", dt.Ctx())},
		Returns: node.AnonReturns(dt.IterSeq(dt.Qualified("x", "Article"))),
	}
	bag := dt.RunFn(t, streamreader.Detector(), fn)
	dt.AssertShape(t, bag, streamreader.Name, "", "x.Article")
}

func TestDetector_MatchesSeqWithKey(t *testing.T) {
	t.Parallel()
	fn := &node.Function{
		Name: "AllWithFilter", Package: "x",
		Params: []*node.Param{
			dt.Param("ctx", dt.Ctx()),
			dt.Param("category", dt.Named("string")),
		},
		Returns: node.AnonReturns(dt.IterSeq(dt.Qualified("x", "Article"))),
	}
	bag := dt.RunFn(t, streamreader.Detector(), fn)
	dt.AssertShape(t, bag, streamreader.Name, "string", "x.Article")
}

func TestDetector_MatchesSeq2(t *testing.T) {
	t.Parallel()
	fn := &node.Function{
		Name: "All", Package: "x",
		Params: []*node.Param{dt.Param("ctx", dt.Ctx())},
		Returns: node.AnonReturns(
			dt.IterSeq2(dt.Qualified("x", "Article"), dt.Err()),
		),
	}
	bag := dt.RunFn(t, streamreader.Detector(), fn)
	dt.AssertShape(t, bag, streamreader.Name, "", "x.Article")
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *node.Function
	}{
		{"non-iter return (Reader / Aggregator territory)", &node.Function{
			Name: "Get", Package: "x",
			Params:  []*node.Param{dt.Param("ctx", dt.Ctx())},
			Returns: node.AnonReturns(dt.Qualified("x", "Article")),
		}},
		{"multiple returns", &node.Function{
			Name: "All", Package: "x",
			Returns: node.AnonReturns(
				dt.IterSeq(dt.Qualified("x", "Article")),
				dt.Err(),
			),
		}},
		{"too many input keys", &node.Function{
			Name: "All", Package: "x",
			Params: []*node.Param{
				dt.Param("a", dt.Named("string")),
				dt.Param("b", dt.Named("string")),
			},
			Returns: node.AnonReturns(dt.IterSeq(dt.Qualified("x", "Article"))),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, streamreader.Detector(), tc.fn))
		})
	}
}
