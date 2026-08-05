// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package readernoerror_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	dt "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/internal/detectortest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/readernoerror"
)

func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := readernoerror.Detector()
	if det.Name != readernoerror.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, readernoerror.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

func TestDetector_Matches(t *testing.T) {
	t.Parallel()

	t.Run("(ctx, K) V", func(t *testing.T) {
		t.Parallel()
		fn := &node.Function{
			Name: "Get", Package: "x",
			Params: []*node.Param{
				dt.Param("ctx", dt.Ctx()),
				dt.Param("id", dt.Named("string")),
			},
			Returns: node.AnonReturns(dt.Qualified("x", "Article")),
		}
		bag := dt.RunFn(t, readernoerror.Detector(), fn)
		dt.AssertShape(t, bag, readernoerror.Name, "string", "x.Article")
	})

	t.Run("(K) V", func(t *testing.T) {
		t.Parallel()
		fn := &node.Function{
			Name: "Get", Package: "x",
			Params:  []*node.Param{dt.Param("id", dt.Named("string"))},
			Returns: node.AnonReturns(dt.Qualified("x", "Article")),
		}
		bag := dt.RunFn(t, readernoerror.Detector(), fn)
		dt.AssertShape(t, bag, readernoerror.Name, "string", "x.Article")
	})
}

func TestDetector_Rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fn   *node.Function
	}{
		{"has error return (Reader territory)", &node.Function{
			Name: "Get", Package: "x",
			Params: []*node.Param{dt.Param("id", dt.Named("string"))},
			Returns: node.AnonReturns(
				dt.Qualified("x", "Article"),
				dt.Err(),
			),
		}},
		{"no params (Aggregator territory)", &node.Function{
			Name: "Count", Package: "x",
			Returns: node.AnonReturns(dt.Named("int")),
		}},
		{"void return (Mutator territory)", &node.Function{
			Name: "Set", Package: "x",
			Params: []*node.Param{dt.Param("v", dt.Qualified("x", "Article"))},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dt.AssertUnstamped(t, dt.RunFn(t, readernoerror.Detector(), tc.fn))
		})
	}
}
