// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package lifecycle_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/lifecycle"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = meta.EnsureKey("frontend", meta.StringParser)

// TestDetector_Identity pins the constructor invariants.
func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := lifecycle.Detector()
	if det.Name != lifecycle.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, lifecycle.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

// TestDetector_MatchesLifecycle covers the only accepted
// signature variant: `(ctx) error`.
func TestDetector_MatchesLifecycle(t *testing.T) {
	t.Parallel()

	fn := lifecycleFunc("Start")
	runDetectFunc(t, fn)
	if got := shape.Get(fn.Meta()); got != lifecycle.Name {
		t.Fatalf("shape = %q, want %q", got, lifecycle.Name)
	}
	// Neither key nor value type stamped — lifecycle has neither.
	if v, ok := shape.MetaKeyType.Get(fn.Meta()); ok {
		t.Fatalf("shape.key_type unexpectedly stamped: %q", v)
	}
	if v, ok := shape.MetaValueType.Get(fn.Meta()); ok {
		t.Fatalf("shape.value_type unexpectedly stamped: %q", v)
	}
}

// TestDetector_MatchesMethod covers methods on structs and
// interfaces — both have the same signature acceptance.
func TestDetector_MatchesMethod(t *testing.T) {
	t.Parallel()

	t.Run("struct method", func(t *testing.T) {
		t.Parallel()
		m := lifecycleMethod("Start")
		s := &node.Struct{
			Name: "Service", Package: "x",
			Methods: []*node.Method{m},
		}
		runDetect(t, &node.Package{
			Name: "x", Path: "x",
			Structs: []*node.Struct{s},
		})
		if got := shape.Get(m.Meta()); got != lifecycle.Name {
			t.Fatalf("shape = %q, want %q", got, lifecycle.Name)
		}
	})

	t.Run("interface method", func(t *testing.T) {
		t.Parallel()
		m := lifecycleMethod("Start")
		i := &node.Interface{
			Name: "Service", Package: "x",
			Methods: []*node.Method{m},
		}
		runDetect(t, &node.Package{
			Name: "x", Path: "x",
			Interfaces: []*node.Interface{i},
		})
		if got := shape.Get(m.Meta()); got != lifecycle.Name {
			t.Fatalf("shape = %q, want %q", got, lifecycle.Name)
		}
	})
}

// TestDetector_RejectsNonLifecycle pins the boundaries: anything
// beyond the bare `(ctx) error` shape must not detect.
func TestDetector_RejectsNonLifecycle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		fn   *node.Function
	}{
		{
			name: "missing context (would be VoidLifecycle / Predicate / PoisonAccessor)",
			fn: &node.Function{
				Name: "Start", Package: "x",
				Returns: node.AnonReturns(&node.TypeRef{Name: "error"}),
			},
		},
		{
			name: "missing error (just `(ctx)` is void)",
			fn: &node.Function{
				Name: "Start", Package: "x",
				Params: []*node.Param{
					{Name: "ctx", Type: &node.TypeRef{Name: "Context", Package: "context"}},
				},
			},
		},
		{
			name: "extra param (Reader / Writer territory)",
			fn: &node.Function{
				Name: "Start", Package: "x",
				Params: []*node.Param{
					{Name: "ctx", Type: &node.TypeRef{Name: "Context", Package: "context"}},
					{Name: "x", Type: &node.TypeRef{Name: "string"}},
				},
				Returns: node.AnonReturns(&node.TypeRef{Name: "error"}),
			},
		},
		{
			name: "extra return (Reader territory)",
			fn: &node.Function{
				Name: "Start", Package: "x",
				Params: []*node.Param{
					{Name: "ctx", Type: &node.TypeRef{Name: "Context", Package: "context"}},
				},
				Returns: node.AnonReturns(
					&node.TypeRef{Name: "string"},
					&node.TypeRef{Name: "error"},
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

// lifecycleFunc builds a free [node.Function] with the canonical
// lifecycle signature.
func lifecycleFunc(name string) *node.Function {
	return &node.Function{
		Name: name, Package: "x",
		Params: []*node.Param{
			{Name: "ctx", Type: &node.TypeRef{Name: "Context", Package: "context"}},
		},
		Returns: node.AnonReturns(&node.TypeRef{Name: "error"}),
	}
}

// lifecycleMethod builds a [node.Method] with the canonical
// lifecycle signature.
func lifecycleMethod(name string) *node.Method {
	fn := lifecycleFunc(name)
	return &node.Method{
		Name: fn.Name, Params: fn.Params, Returns: fn.Returns,
	}
}

// runDetectFunc wires fn into a single-function package and runs
// the lifecycle detector through the umbrella shape plugin.
func runDetectFunc(t *testing.T, fn *node.Function) {
	t.Helper()
	runDetect(t, &node.Package{
		Name: "x", Path: "x",
		Functions: []*node.Function{fn},
	})
}

// runDetect adds pkg to a fresh store, stamps the Go frontend
// marker on the package, and runs the umbrella shape plugin
// configured with this package's detector.
func runDetect(t *testing.T, pkg *node.Package) {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")

	p := shape.New().Detectors(lifecycle.Detector())
	ctx := &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   diag.New(),
	}
	if err := p.Annotate(ctx); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
}
