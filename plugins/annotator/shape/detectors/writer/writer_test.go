// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package writer_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/writer"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = sdk.EnsureKey("frontend", sdk.StringParser)

// TestDetector_Identity pins the constructor invariants.
func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := writer.Detector()
	if det.Name != writer.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, writer.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

// TestDetector_MatchesWriterSignatures covers every signature
// variant the docstring promises detects as a writer: exactly one
// non-context parameter and error as the only return.
func TestDetector_MatchesWriterSignatures(t *testing.T) {
	t.Parallel()

	t.Run("error-only return with leading context", func(t *testing.T) {
		t.Parallel()
		fn := writerFunc("Save", true, false)
		runDetectFunc(t, fn)
		assertShape(t, fn.Meta(), writer.Name, "x.Article")
	})

	t.Run("error-only return without context", func(t *testing.T) {
		t.Parallel()
		fn := writerFunc("Save", false, false)
		runDetectFunc(t, fn)
		assertShape(t, fn.Meta(), writer.Name, "x.Article")
	})

	t.Run("struct method", func(t *testing.T) {
		t.Parallel()
		m := writerMethod("Save", true)
		s := &sdk.Struct{
			Name: "Repo", Package: "x",
			Methods: []*sdk.Method{m},
		}
		runDetect(t, &sdk.Package{
			Name: "x", Path: "x",
			Structs: []*sdk.Struct{s},
		})
		assertShape(t, m.Meta(), writer.Name, "x.Article")
	})
}

// TestDetector_RejectsNonWriter pins the boundaries against the
// writer's neighbours in the shape catalog.
func TestDetector_RejectsNonWriter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		fn   *sdk.Function
	}{
		{
			name: "no error return (not a writer)",
			fn: &sdk.Function{
				Name: "Save", Package: "x",
				Params: []*sdk.Param{
					{Name: "a", Type: &sdk.TypeRef{Name: "Article", Package: "x"}},
				},
				Returns: sdk.AnonReturns(&sdk.TypeRef{Name: "Article", Package: "x"}),
			},
		},
		{
			// This variant used to detect as a writer, which made
			// every signature `reader` recognises a strict subset of
			// this detector's. Running first at the higher priority,
			// writer claimed them all and reader never won a
			// dispatch — while stamping the callable's parameter as
			// the written value. A write returning a receipt is
			// genuinely neither shape and wants a detector of its
			// own; until then it falls through to reader, which at
			// least records both types.
			name: "a value alongside the error (receipt-returning write)",
			fn:   writerFunc("Save", true, true),
		},
		{
			name: "two non-ctx params (CompositeWriter territory)",
			fn: &sdk.Function{
				Name: "Save", Package: "x",
				Params: []*sdk.Param{
					{Name: "k", Type: &sdk.TypeRef{Name: "string"}},
					{Name: "v", Type: &sdk.TypeRef{Name: "Article", Package: "x"}},
				},
				Returns: sdk.AnonReturns(&sdk.TypeRef{Name: "error"}),
			},
		},
		{
			name: "lifecycle signature (no non-ctx params)",
			fn: &sdk.Function{
				Name: "Start", Package: "x",
				Params: []*sdk.Param{
					{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}},
				},
				Returns: sdk.AnonReturns(&sdk.TypeRef{Name: "error"}),
			},
		},
		{
			name: "three returns including error (MultiReader territory)",
			fn: &sdk.Function{
				Name: "Save", Package: "x",
				Params: []*sdk.Param{
					{Name: "v", Type: &sdk.TypeRef{Name: "Article", Package: "x"}},
				},
				Returns: sdk.AnonReturns(
					&sdk.TypeRef{Name: "R1", Package: "x"},
					&sdk.TypeRef{Name: "R2", Package: "x"},
					&sdk.TypeRef{Name: "error"},
				),
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

// writerFunc builds a free [sdk.Function] matching the canonical
// writer signature. withResult enables the (R, error) return
// variant; withCtx prepends a leading context parameter.
func writerFunc(name string, withCtx, withResult bool) *sdk.Function {
	params := []*sdk.Param{
		{Name: "v", Type: &sdk.TypeRef{Name: "Article", Package: "x"}},
	}
	if withCtx {
		params = append([]*sdk.Param{
			{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}},
		}, params...)
	}
	returns := []*sdk.TypeRef{{Name: "error"}}
	if withResult {
		returns = []*sdk.TypeRef{
			{Name: "Result", Package: "x"},
			{Name: "error"},
		}
	}
	return &sdk.Function{
		Name: name, Package: "x",
		Params:  params,
		Returns: sdk.AnonReturns(returns...),
	}
}

// writerMethod builds a [sdk.Method] matching the canonical
// writer signature.
func writerMethod(name string, withCtx bool) *sdk.Method {
	fn := writerFunc(name, withCtx, false)
	return &sdk.Method{
		Name: fn.Name, Params: fn.Params, Returns: fn.Returns,
	}
}

// runDetectFunc wires fn into a single-function package and runs
// the writer detector through the umbrella shape plugin.
func runDetectFunc(t *testing.T, fn *sdk.Function) {
	t.Helper()
	runDetect(t, &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{fn},
	})
}

// runDetect adds pkg to a fresh store, stamps the Go frontend
// marker on the package, and runs the umbrella shape plugin
// configured with this package's detector.
func runDetect(t *testing.T, pkg *sdk.Package) {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")

	p := shape.New().Detectors(writer.Detector())
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
// don't match the supplied want values. Empty wantValue means
// "value must be absent".
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
