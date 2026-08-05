// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package reader_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/reader"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// frontendMarker mirrors the umbrella plugin's package-level
// frontend lookup key so fixtures can stamp it on the test
// package's meta bag.
//
//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = meta.EnsureKey("frontend", meta.StringParser)

// TestDetector_Identity pins the constructor's invariants: the
// detector reports the package's canonical [reader.Name] and
// registers a Go-frontend [shape.DetectFunc].
func TestDetector_Identity(t *testing.T) {
	t.Parallel()
	det := reader.Detector()
	if det.Name != reader.Name {
		t.Fatalf("Detector().Name = %q, want %q", det.Name, reader.Name)
	}
	if _, ok := det.Detect["golang"]; !ok {
		t.Fatalf("Detector().Detect missing %q entry", "golang")
	}
}

// TestDetector_MatchesReaderSignatures covers every accepted
// signature variant the docstring promises detects as a reader.
func TestDetector_MatchesReaderSignatures(t *testing.T) {
	t.Parallel()

	t.Run("free function with leading context", func(t *testing.T) {
		t.Parallel()
		fn := readerFunc("Get", true)
		runDetectFunc(t, fn)
		assertShape(t, fn.Meta(), reader.Name, "string", "x.Article")
	})

	t.Run("free function without context", func(t *testing.T) {
		t.Parallel()
		fn := readerFunc("Get", false)
		runDetectFunc(t, fn)
		assertShape(t, fn.Meta(), reader.Name, "string", "x.Article")
	})

	t.Run("struct method", func(t *testing.T) {
		t.Parallel()
		m := readerMethod("Get", true)
		s := &node.Struct{
			Name: "Repo", Package: "x",
			Methods: []*node.Method{m},
		}
		runDetect(t, &node.Package{
			Name: "x", Path: "x",
			Structs: []*node.Struct{s},
		})
		assertShape(t, m.Meta(), reader.Name, "string", "x.Article")
	})

	t.Run("interface method", func(t *testing.T) {
		t.Parallel()
		m := readerMethod("Get", true)
		i := &node.Interface{
			Name: "Repo", Package: "x",
			Methods: []*node.Method{m},
		}
		runDetect(t, &node.Package{
			Name: "x", Path: "x",
			Interfaces: []*node.Interface{i},
		})
		assertShape(t, m.Meta(), reader.Name, "string", "x.Article")
	})
}

// TestDetector_RejectsNonReader covers the negative space — the
// signatures the reader detector must NOT match so neighbouring
// detectors (Writer, Lifecycle, Pure, ReaderNoError, …) can
// claim them.
func TestDetector_RejectsNonReader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fn   *node.Function
	}{
		{
			name: "no error return (would be ReaderNoError)",
			fn: &node.Function{
				Name: "Get", Package: "x",
				Params: []*node.Param{
					{Name: "id", Type: &node.TypeRef{Name: "string"}},
				},
				Returns: []*node.TypeRef{{Name: "Article", Package: "x"}},
			},
		},
		{
			name: "writer signature: value param + error only",
			fn: &node.Function{
				Name: "Save", Package: "x",
				Params: []*node.Param{
					{Name: "a", Type: &node.TypeRef{Name: "Article", Package: "x"}},
				},
				Returns: []*node.TypeRef{{Name: "error"}},
			},
		},
		{
			name: "two non-ctx params (MultiArgWriter / CompositeWriter territory)",
			fn: &node.Function{
				Name: "Get", Package: "x",
				Params: []*node.Param{
					{Name: "a", Type: &node.TypeRef{Name: "string"}},
					{Name: "b", Type: &node.TypeRef{Name: "string"}},
				},
				Returns: []*node.TypeRef{
					{Name: "Article", Package: "x"},
					{Name: "error"},
				},
			},
		},
		{
			name: "two non-error returns (MultiReader territory)",
			fn: &node.Function{
				Name: "Get", Package: "x",
				Params: []*node.Param{
					{Name: "id", Type: &node.TypeRef{Name: "string"}},
				},
				Returns: []*node.TypeRef{
					{Name: "Article", Package: "x"},
					{Name: "Meta", Package: "x"},
					{Name: "error"},
				},
			},
		},
		{
			name: "lifecycle signature: only ctx in, only error out",
			fn: &node.Function{
				Name: "Start", Package: "x",
				Params: []*node.Param{
					{Name: "ctx", Type: &node.TypeRef{Name: "Context", Package: "context"}},
				},
				Returns: []*node.TypeRef{{Name: "error"}},
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

// TestDetector_RejectsUnkeyableParams pins the key contract: the
// stamp asserts the parameter *is* a key, so a parameter Go gives
// no equality — slice, map, func — cannot carry it, and neither can
// an inline interface whose value may not be re-readable.
//
// A named interface (io.Reader) is deliberately absent: it is
// opaque in the node IR and still detects, pending a frontend
// interface-ness stamp.
func TestDetector_RejectsUnkeyableParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		param *node.TypeRef
	}{
		{"slice key has no equality", &node.TypeRef{
			TypeKind: node.TypeRefSlice, Elem: &node.TypeRef{Name: "string"},
		}},
		{"map key has no equality", &node.TypeRef{
			TypeKind: node.TypeRefMap,
			MapKey:   &node.TypeRef{Name: "string"},
			MapValue: &node.TypeRef{Name: "string"},
		}},
		{"func key has no equality", &node.TypeRef{TypeKind: node.TypeRefFunc}},
		{"inline interface may not re-read", &node.TypeRef{
			TypeKind: node.TypeRefAnonInterface,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fn := readerFunc("Get", true)
			fn.Params[len(fn.Params)-1].Type = tc.param
			runDetectFunc(t, fn)
			if shape.IsStamped(fn.Meta()) {
				t.Fatalf("expected no stamp for %s; got shape=%q with key_type=%q",
					tc.name, shape.Get(fn.Meta()), keyTypeOf(t, fn))
			}
		})
	}
}

// TestDetector_NamedInterfaceKeyIsAKnownGap documents the case the
// keyable check cannot yet reach, so the limit is asserted rather
// than assumed. When the frontend stamps interface-ness on refs and
// [reader] consumes it, this test is the one that must flip.
func TestDetector_NamedInterfaceKeyIsAKnownGap(t *testing.T) {
	t.Parallel()

	fn := readerFunc("Load", true)
	fn.Params[len(fn.Params)-1].Type = &node.TypeRef{Name: "Reader", Package: "io"}
	runDetectFunc(t, fn)

	if !shape.IsStamped(fn.Meta()) {
		t.Fatalf("io.Reader key no longer detects as a reader; if the frontend now " +
			"stamps interface-ness, fold the named case into keyable and delete this test")
	}
	assertShape(t, fn.Meta(), reader.Name, "io.Reader", "x.Article")
}

// keyTypeOf reports the stamped key type, for failure messages.
func keyTypeOf(t *testing.T, fn *node.Function) string {
	t.Helper()
	got, _ := shape.MetaKeyType.Get(fn.Meta())
	return got
}

// readerFunc builds a free [node.Function] matching the
// canonical reader signature, optionally including a leading
// context parameter.
func readerFunc(name string, withCtx bool) *node.Function {
	params := []*node.Param{
		{Name: "id", Type: &node.TypeRef{Name: "string"}},
	}
	if withCtx {
		params = append([]*node.Param{
			{Name: "ctx", Type: &node.TypeRef{Name: "Context", Package: "context"}},
		}, params...)
	}
	return &node.Function{
		Name: name, Package: "x",
		Params: params,
		Returns: []*node.TypeRef{
			{Name: "Article", Package: "x"},
			{Name: "error"},
		},
	}
}

// readerMethod builds a [node.Method] matching the canonical
// reader signature for use inside a struct or interface fixture.
func readerMethod(name string, withCtx bool) *node.Method {
	fn := readerFunc(name, withCtx)
	return &node.Method{
		Name: fn.Name, Params: fn.Params, Returns: fn.Returns,
	}
}

// runDetectFunc wires fn into a single-function package and runs
// the reader detector through the umbrella shape plugin.
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
	frontendMarker.Set(pkg.Meta(), "golang", "test")

	p := shape.New().Detectors(reader.Detector())
	ctx := &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   diag.New(),
	}
	if err := p.Annotate(ctx); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
}

// assertShape fails when the three structural-shape meta keys on
// bag don't match the supplied want values. Empty wantKey /
// wantValue mean "key/value must be absent".
func assertShape(t *testing.T, bag *meta.Bag, wantName, wantKey, wantValue string) {
	t.Helper()
	if got := shape.Get(bag); got != wantName {
		t.Fatalf("shape = %q, want %q", got, wantName)
	}
	got, _ := shape.MetaKeyType.Get(bag)
	if got != wantKey {
		t.Fatalf("shape.key_type = %q, want %q", got, wantKey)
	}
	got, _ = shape.MetaValueType.Get(bag)
	if got != wantValue {
		t.Fatalf("shape.value_type = %q, want %q", got, wantValue)
	}
}
