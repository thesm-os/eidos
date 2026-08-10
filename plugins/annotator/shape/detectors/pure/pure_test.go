// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pure_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/pure"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = sdk.EnsureKey("frontend", sdk.StringParser)

// TestDetector_Identity pins the constructor invariants.
func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := pure.Detector()
	if det.Name != pure.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, pure.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

// TestDetector_MatchesPure covers the accepted signatures —
// arbitrary positional params, single non-error return, no
// context, no error.
func TestDetector_MatchesPure(t *testing.T) {
	t.Parallel()

	t.Run("no params, single return", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Now", Package: "x",
			Returns: sdk.AnonReturns(&sdk.TypeRef{Name: "Time", Package: "time"}),
		}
		runDetectFunc(t, fn)
		assertShape(t, fn.Meta(), pure.Name, "time.Time")
	})

	t.Run("two params, single return", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Add", Package: "x",
			Params: []*sdk.Param{
				{Name: "a", Type: &sdk.TypeRef{Name: "int"}},
				{Name: "b", Type: &sdk.TypeRef{Name: "int"}},
			},
			Returns: sdk.AnonReturns(&sdk.TypeRef{Name: "int"}),
		}
		runDetectFunc(t, fn)
		assertShape(t, fn.Meta(), pure.Name, "int")
	})

	t.Run("variadic params, single return", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Sum", Package: "x",
			Params: []*sdk.Param{
				{Name: "xs", Type: &sdk.TypeRef{Name: "int"}, Variadic: true},
			},
			Returns: sdk.AnonReturns(&sdk.TypeRef{Name: "int"}),
		}
		runDetectFunc(t, fn)
		assertShape(t, fn.Meta(), pure.Name, "int")
	})
}

// TestDetector_RejectsImpure pins the boundaries: any
// context-aware, error-returning, or multi-return signature must
// not detect.
func TestDetector_RejectsImpure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		fn   *sdk.Function
	}{
		{
			name: "has context parameter",
			fn: &sdk.Function{
				Name: "Now", Package: "x",
				Params: []*sdk.Param{
					{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}},
				},
				Returns: sdk.AnonReturns(&sdk.TypeRef{Name: "Time", Package: "time"}),
			},
		},
		{
			name: "has error return (Aggregator / Reader territory)",
			fn: &sdk.Function{
				Name: "Sum", Package: "x",
				Params: []*sdk.Param{
					{Name: "xs", Type: &sdk.TypeRef{Name: "int"}, Variadic: true},
				},
				Returns: sdk.AnonReturns(
					&sdk.TypeRef{Name: "int"},
					&sdk.TypeRef{Name: "error"},
				),
			},
		},
		{
			name: "two non-error returns (MultiAggregator territory)",
			fn: &sdk.Function{
				Name: "Pair", Package: "x",
				Returns: sdk.AnonReturns(
					&sdk.TypeRef{Name: "int"},
					&sdk.TypeRef{Name: "string"},
				),
			},
		},
		{
			name: "no returns (VoidLifecycle / Mutator territory)",
			fn: &sdk.Function{
				Name: "Side", Package: "x",
				Params: []*sdk.Param{
					{Name: "v", Type: &sdk.TypeRef{Name: "Article", Package: "x"}},
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runDetectFunc(t, tc.fn)
			if shape.IsStamped(tc.fn.Meta()) {
				t.Fatalf("expected no stamp for %s; got shape=%q", tc.name, shape.Get(tc.fn.Meta()))
			}
		})
	}
}

// runDetectFunc wires fn into a single-function package and runs
// the pure detector through the umbrella shape plugin.
func runDetectFunc(t *testing.T, fn *sdk.Function) {
	t.Helper()
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{fn},
	}
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")

	p := shape.New().Detectors(pure.Detector())
	ctx := &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   diag.New(),
	}
	if err := p.Annotate(ctx); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
}

// assertShape fails when the structural-shape meta keys on bag
// don't match the supplied want values.
func assertShape(t *testing.T, bag *sdk.Bag, wantName, wantValue string) {
	t.Helper()
	if got := shape.Get(bag); got != wantName {
		t.Fatalf("shape = %q, want %q", got, wantName)
	}
	got, _ := shape.MetaValueType.Get(bag)
	if got != wantValue {
		t.Fatalf("shape.value_type = %q, want %q", got, wantValue)
	}
}
