// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package closer_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/closer"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/poisonaccessor"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = sdk.EnsureKey("frontend", sdk.StringParser)

// nullaryErr builds `func <name>() error`, the signature a teardown
// and a poison accessor share.
func nullaryErr(name string) *sdk.Function {
	return &sdk.Function{
		Name: name, Package: "x",
		Returns: sdk.AnonReturns(&sdk.TypeRef{Name: "error"}),
	}
}

func run(t *testing.T, fns []*sdk.Function, dets ...shape.Detector) {
	t.Helper()
	s := store.New()
	pkg := &sdk.Package{Name: "x", Path: "x", Functions: fns}
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

func TestDetector(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		det := closer.Detector()
		if det.Name != closer.Name {
			t.Fatalf("Name = %q, want %q", det.Name, closer.Name)
		}
		latch := poisonaccessor.Detector().Priority
		if det.Priority <= latch {
			t.Fatalf("Priority = %d, must exceed poisonaccessor's %d or the"+
				" shape it exists to reclaim is claimed first", det.Priority, latch)
		}
	})

	t.Run("every recognised name is drawn", func(t *testing.T) {
		t.Parallel()
		for _, name := range closer.Names {
			fn := nullaryErr(name)
			run(t, []*sdk.Function{fn}, closer.Detector())
			if got := shape.Get(fn.Meta()); got != closer.Name {
				t.Errorf("%s() error stamped %q, want %q", name, got, closer.Name)
			}
		}
	})

	t.Run("it takes Close from poisonaccessor", func(t *testing.T) {
		t.Parallel()
		// The defect: a close-once teardown answers differently the
		// second time, which is what a read-purity law over a poison
		// accessor forbids — so the stamp reddened correct code.
		fn := nullaryErr("Close")
		run(t, []*sdk.Function{fn}, closer.Detector(), poisonaccessor.Detector())
		if got := shape.Get(fn.Meta()); got != closer.Name {
			t.Fatalf("Close() error stamped %q, want %q", got, closer.Name)
		}
	})

	t.Run("poisonaccessor keeps every other bare-error nullary", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"Err", "Ping", "Validate"} {
			fn := nullaryErr(name)
			run(t, []*sdk.Function{fn}, closer.Detector(), poisonaccessor.Detector())
			if got := shape.Get(fn.Meta()); got != poisonaccessor.Name {
				t.Errorf("%s() error stamped %q, want %q", name, got, poisonaccessor.Name)
			}
		}
	})

	t.Run("a recognised name with another signature is not drawn", func(t *testing.T) {
		t.Parallel()
		// The name is a tiebreak between two readings of one shape, not
		// a shape of its own: Close(ctx) error is the lifecycle form and
		// stays there.
		fn := &sdk.Function{
			Name: "Close", Package: "x",
			Params:  []*sdk.Param{{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}}},
			Returns: sdk.AnonReturns(&sdk.TypeRef{Name: "error"}),
		}
		run(t, []*sdk.Function{fn}, closer.Detector())
		if got := shape.Get(fn.Meta()); got == closer.Name {
			t.Fatal("Close(ctx) error was drawn as a bare teardown")
		}
	})
}
