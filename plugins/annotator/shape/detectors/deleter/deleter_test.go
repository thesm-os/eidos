// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package deleter_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/deleter"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/writer"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = sdk.EnsureKey("frontend", sdk.StringParser)

// keyedErr builds `func <name>(ctx, key string) error` — the
// signature a removal and a write share, which is the whole reason
// this detector reads the name.
func keyedErr(name string) *sdk.Function {
	return &sdk.Function{
		Name: name, Package: "x",
		Params: []*sdk.Param{
			{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}},
			{Name: "key", Type: &sdk.TypeRef{Name: "string"}},
		},
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

func shapeOf(fn *sdk.Function) string {
	got, _ := shape.MetaShape.Get(fn.Meta())
	return got
}

func TestDetector(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		det := deleter.Detector()
		if det.Name != deleter.Name {
			t.Fatalf("Name = %q, want %q", det.Name, deleter.Name)
		}
		latch := writer.Detector().Priority
		if det.Priority <= latch {
			t.Fatalf("Priority = %d, must exceed writer's %d or the signature"+
				" they share is claimed as a write first", det.Priority, latch)
		}
	})

	t.Run("every recognised name is drawn", func(t *testing.T) {
		t.Parallel()
		// A name in the set that no signature reaches is a claim the
		// catalog cannot honour.
		for _, name := range deleter.Names {
			fn := keyedErr(name)
			run(t, []*sdk.Function{fn}, deleter.Detector())
			if got := shapeOf(fn); got != deleter.Name {
				t.Errorf("%s: shape = %q, want %q", name, got, deleter.Name)
			}
		}
	})

	t.Run("it wins the signature it shares with a writer", func(t *testing.T) {
		t.Parallel()
		del, put := keyedErr("Delete"), keyedErr("Put")
		run(t, []*sdk.Function{del, put}, deleter.Detector(), writer.Detector())
		if got := shapeOf(del); got != deleter.Name {
			t.Fatalf("Delete = %q, want %q — a write-then-read-back law would"+
				" assert the reverse of correct behaviour", got, deleter.Name)
		}
		if got := shapeOf(put); got != writer.Name {
			t.Fatalf("Put = %q, want %q — the gate must not swallow writes", got, writer.Name)
		}
	})

	t.Run("the parameter records as the key, not the value", func(t *testing.T) {
		t.Parallel()
		// What writer recorded when a delete fell through to it: the
		// key it addresses, labelled as the value it stores.
		del := keyedErr("Delete")
		run(t, []*sdk.Function{del}, deleter.Detector())
		if got, _ := shape.MetaKeyType.Get(del.Meta()); got != "string" {
			t.Fatalf("key_type = %q, want string", got)
		}
		if got, ok := shape.MetaValueType.Get(del.Meta()); ok && got != "" {
			t.Fatalf("value_type = %q, want unset — a delete stores nothing", got)
		}
	})

	t.Run("an unrecognised name is left to the signature detectors", func(t *testing.T) {
		t.Parallel()
		fn := keyedErr("Drop")
		run(t, []*sdk.Function{fn}, deleter.Detector())
		if got := shapeOf(fn); got != "" {
			t.Fatalf("Drop = %q; the set deliberately omits words that also"+
				" mean something else", got)
		}
	})

	t.Run("a removal answering a value is not this shape", func(t *testing.T) {
		t.Parallel()
		// `Delete(ctx, key) (Doc, error)` returns what it removed.
		// That is a different claim and wants its own detector rather
		// than this one's law.
		fn := keyedErr("Delete")
		fn.Returns = sdk.AnonReturns(
			&sdk.TypeRef{Name: "Doc", Package: "x"},
			&sdk.TypeRef{Name: "error"},
		)
		run(t, []*sdk.Function{fn}, deleter.Detector())
		if got := shapeOf(fn); got != "" {
			t.Fatalf("shape = %q, want unstamped", got)
		}
	})

	t.Run("a removal with no error is not this shape", func(t *testing.T) {
		t.Parallel()
		fn := keyedErr("Delete")
		fn.Returns = nil
		run(t, []*sdk.Function{fn}, deleter.Detector())
		if got := shapeOf(fn); got != "" {
			t.Fatalf("shape = %q, want unstamped", got)
		}
	})
}
