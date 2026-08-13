// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package answeringwriter_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/answeringwriter"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/reader"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = sdk.EnsureKey("frontend", sdk.StringParser)

// fn builds a callable taking one non-context param and returning
// (result, error).
func fn(name, param, result string) *sdk.Function {
	return &sdk.Function{
		Name: name, Package: "x",
		Params: []*sdk.Param{
			{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}},
			{Name: "v", Type: &sdk.TypeRef{Name: param, Package: "x"}},
		},
		Returns: sdk.AnonReturns(
			&sdk.TypeRef{Name: result, Package: "x"},
			&sdk.TypeRef{Name: "error"},
		),
	}
}

// run dispatches the umbrella over pkg with the supplied detectors, in
// registration order.
func run(t *testing.T, pkg *sdk.Package, dets ...shape.Detector) {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")
	p := shape.New().Detectors(dets...)
	ctx := &sdk.AnnotatorContext{Store: s, Reader: store.NewReader(s), Diag: diag.New()}
	if err := p.Annotate(ctx); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
}

func stamped(bag *sdk.Bag) string { return shape.Get(bag) }

func TestDetector(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		det := answeringwriter.Detector()
		if det.Name != answeringwriter.Name {
			t.Fatalf("Name = %q, want %q", det.Name, answeringwriter.Name)
		}
		if _, ok := det.Detect["golang"]; !ok {
			t.Fatal("no golang DetectFunc registered")
		}
	})

	t.Run("a write answered by its own type is drawn", func(t *testing.T) {
		t.Parallel()
		store := fn("Store", "Value", "Value")
		run(t, &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{store}},
			answeringwriter.Detector())
		if got := stamped(store.Meta()); got != answeringwriter.Name {
			t.Fatalf("shape = %q, want %q", got, answeringwriter.Name)
		}
	})

	t.Run("differing parameter and result types are not drawn", func(t *testing.T) {
		t.Parallel()
		// Type identity is the whole rule. Accepting any single
		// non-error result would take every reader with it, which is
		// the failure the writer detector records from the other side.
		get := fn("Get", "ID", "Article")
		run(t, &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{get}},
			answeringwriter.Detector())
		if got := stamped(get.Meta()); got == answeringwriter.Name {
			t.Fatal("a read with distinct key and value types was drawn")
		}
	})

	t.Run("it wins the answered shape from reader", func(t *testing.T) {
		t.Parallel()
		// The point of the priority: this signature stamped reader
		// before, recording the written value as a key.
		store := fn("Store", "Value", "Value")
		run(t, &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{store}},
			answeringwriter.Detector(), reader.Detector())
		if got := stamped(store.Meta()); got != answeringwriter.Name {
			t.Fatalf("shape = %q, want %q to win", got, answeringwriter.Name)
		}
	})

	t.Run("reader keeps every other read", func(t *testing.T) {
		t.Parallel()
		get := fn("Get", "ID", "Article")
		run(t, &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{get}},
			answeringwriter.Detector(), reader.Detector())
		if got := stamped(get.Meta()); got != reader.Name {
			t.Fatalf("shape = %q, want reader to keep it", got)
		}
	})
}
