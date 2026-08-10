// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package streamconsumer_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/streamconsumer"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// These mirror the singletons the detector resolves by name. Being
// the same registry entries, they are how a test stamps the fact
// the Go frontend would stamp in a real run.
//
//nolint:gochecknoglobals // cross-package registry-singleton keys
var (
	frontendMarker  = sdk.EnsureKey("frontend", sdk.StringParser)
	metaIsInterface = sdk.EnsureKey("go.isInterface", sdk.BoolParser)
)

// TestDetector_Identity pins the constructor invariants.
func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := streamconsumer.Detector()
	if det.Name != streamconsumer.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, streamconsumer.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

// TestDetector_MatchesStreamConsumers covers the forms the docstring
// promises detect, across both routes to interface-ness.
func TestDetector_MatchesStreamConsumers(t *testing.T) {
	t.Parallel()

	t.Run("named interface carrying the frontend stamp", func(t *testing.T) {
		t.Parallel()
		fn := consumerFunc("Load", stampedInterface("io", "Reader"))
		runDetectFunc(t, fn)
		assertStream(t, fn.Meta(), "io.Reader", "int")
	})

	t.Run("inline interface needs no stamp", func(t *testing.T) {
		t.Parallel()
		fn := consumerFunc("Load", &sdk.TypeRef{TypeKind: sdk.TypeRefAnonInterface})
		runDetectFunc(t, fn)
		if got := shape.Get(fn.Meta()); got != streamconsumer.Name {
			t.Fatalf("shape = %q, want %q", got, streamconsumer.Name)
		}
	})
}

// TestDetector_StampsStreamTypeNotKeyType pins the distinction the
// shape exists for: recording the stream as a key is the false
// claim it removes.
func TestDetector_StampsStreamTypeNotKeyType(t *testing.T) {
	t.Parallel()

	fn := consumerFunc("Load", stampedInterface("io", "Reader"))
	runDetectFunc(t, fn)

	if got, ok := shape.MetaKeyType.Get(fn.Meta()); ok && got != "" {
		t.Fatalf("shape.key_type = %q; a consumed stream is not a key", got)
	}
	if got, _ := streamconsumer.MetaStreamType.Get(fn.Meta()); got != "io.Reader" {
		t.Fatalf("shape.stream_type = %q, want io.Reader", got)
	}
}

// TestDetector_RejectsNonConsumers covers the negative space, so
// neighbouring shapes keep their signatures.
func TestDetector_RejectsNonConsumers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fn   *sdk.Function
	}{
		{
			// Requiring ctx keeps the detector out of constructor-
			// and helper-shaped code, which shares this signature.
			name: "no context parameter",
			fn: &sdk.Function{
				Name: "Load", Package: "x",
				Params:  []*sdk.Param{{Name: "r", Type: stampedInterface("io", "Reader")}},
				Returns: sdk.AnonReturns(&sdk.TypeRef{Name: "int"}, &sdk.TypeRef{Name: "error"}),
			},
		},
		{
			name: "unstamped named ref is not known to be an interface",
			fn:   consumerFunc("Load", &sdk.TypeRef{Name: "Duration", Package: "time"}),
		},
		{
			name: "plain value parameter",
			fn:   consumerFunc("Get", &sdk.TypeRef{Name: "string"}),
		},
		{
			// A constraint is an interface; `K comparable` is a key.
			name: "type parameter whose constraint is an interface",
			fn:   consumerFunc("Get", stampedTypeParam()),
		},
		{
			name: "no error return",
			fn: &sdk.Function{
				Name: "Load", Package: "x",
				Params: []*sdk.Param{
					{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}},
					{Name: "r", Type: stampedInterface("io", "Reader")},
				},
				Returns: sdk.AnonReturns(&sdk.TypeRef{Name: "int"}),
			},
		},
		{
			name: "two values alongside the error",
			fn: &sdk.Function{
				Name: "Load", Package: "x",
				Params: []*sdk.Param{
					{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}},
					{Name: "r", Type: stampedInterface("io", "Reader")},
				},
				Returns: sdk.AnonReturns(
					&sdk.TypeRef{Name: "int"},
					&sdk.TypeRef{Name: "int"},
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

// stampedInterface returns a named ref carrying the frontend's
// interface-ness fact, as the Go frontend would produce it.
func stampedInterface(pkg, name string) *sdk.TypeRef {
	ref := &sdk.TypeRef{Name: name, Package: pkg}
	metaIsInterface.Set(ref.EnsureMeta(), true, "test")
	return ref
}

// stampedTypeParam returns a type-parameter ref carrying the stamp
// it would never receive in practice, so the test proves the
// detector's own exclusion rather than the frontend's.
func stampedTypeParam() *sdk.TypeRef {
	ref := &sdk.TypeRef{TypeKind: sdk.TypeRefTypeParam, Name: "K"}
	metaIsInterface.Set(ref.EnsureMeta(), true, "test")
	return ref
}

// consumerFunc builds the canonical consumer signature around the
// supplied stream parameter type.
func consumerFunc(name string, stream *sdk.TypeRef) *sdk.Function {
	return &sdk.Function{
		Name: name, Package: "x",
		Params: []*sdk.Param{
			{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}},
			{Name: "r", Type: stream},
		},
		Returns: sdk.AnonReturns(&sdk.TypeRef{Name: "int"}, &sdk.TypeRef{Name: "error"}),
	}
}

// runDetectFunc wires fn into a single-function package and runs the
// detector through the umbrella shape plugin.
func runDetectFunc(t *testing.T, fn *sdk.Function) {
	t.Helper()
	pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")

	p := shape.New().Detectors(streamconsumer.Detector())
	if err := p.Annotate(&sdk.AnnotatorContext{
		Store: s, Reader: store.NewReader(s), Diag: diag.New(),
	}); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
}

// assertStream fails unless bag carries the streamconsumer stamp
// with the expected stream and value types.
func assertStream(t *testing.T, bag *sdk.Bag, wantStream, wantValue string) {
	t.Helper()
	if got := shape.Get(bag); got != streamconsumer.Name {
		t.Fatalf("shape = %q, want %q", got, streamconsumer.Name)
	}
	if got, _ := streamconsumer.MetaStreamType.Get(bag); got != wantStream {
		t.Fatalf("shape.stream_type = %q, want %q", got, wantStream)
	}
	if got, _ := shape.MetaValueType.Get(bag); got != wantValue {
		t.Fatalf("shape.value_type = %q, want %q", got, wantValue)
	}
}
