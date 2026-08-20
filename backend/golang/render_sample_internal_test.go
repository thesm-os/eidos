// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"errors"
	"strings"
	"testing"

	langgo "go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/writer"
)

// sampleState builds the minimal renderState renderSample needs —
// internal, like [render_state_internal_test.go], because the method
// is a funcmap closure and templates cannot be handed a Sample from
// test data.
func sampleState() *renderState {
	return &renderState{imports: writer.NewImportSet(nil)}
}

// TestRenderSample covers the dispatch each consumer previously
// hand-wrote — one subtest per arm, plus the not-OK error the
// consumer asked for instead of the empty render that produced
// `foo(, )`.
func TestRenderSample(t *testing.T) {
	t.Parallel()

	t.Run("a bare literal renders as its text", func(t *testing.T) {
		t.Parallel()
		got, err := sampleState().renderSample(langgo.Sample{Text: "42"})
		if err != nil || got != "42" {
			t.Fatalf("= %q, %v; want 42", got, err)
		}
	})

	t.Run("a ref sample renders as a conversion", func(t *testing.T) {
		t.Parallel()
		// Drawn from the sampler rather than hand-built: the table's
		// Duration entry is the conversion arm's canonical producer.
		s, _ := langgo.SampleRefFor(&node.TypeRef{
			TypeKind: node.TypeRefNamed, Package: "time", Name: "Duration",
		}, "d", nil)
		st := sampleState()
		got, err := st.renderSample(s)
		if err != nil || got != "time.Duration(42)" {
			t.Fatalf("= %q, %v; want time.Duration(42)", got, err)
		}
		assertImported(t, st, "time")
	})

	t.Run("a composite sample renders type-then-text", func(t *testing.T) {
		t.Parallel()
		ref := langgo.FromNode(&node.TypeRef{
			TypeKind: node.TypeRefNamed, Package: "example.com/geo", Name: "Point",
		})
		st := sampleState()
		got, err := st.renderSample(langgo.Sample{Ref: ref, Text: "{X: 42}", Composite: true})
		if err != nil || got != "geo.Point{X: 42}" {
			t.Fatalf("= %q, %v; want geo.Point{X: 42}", got, err)
		}
		assertImported(t, st, "example.com/geo")
	})

	t.Run("an expression sample renders through renderExpr", func(t *testing.T) {
		t.Parallel()
		s, _ := langgo.SampleRefFor(&node.TypeRef{
			TypeKind: node.TypeRefNamed, Package: "time", Name: "Time",
		}, "at", nil)
		st := sampleState()
		got, err := st.renderSample(s)
		if err != nil || got != "time.Unix(42, 0)" {
			t.Fatalf("= %q, %v; want time.Unix(42, 0)", got, err)
		}
		assertImported(t, st, "time")
	})

	t.Run("a defined type over a struct renders the composite form", func(t *testing.T) {
		t.Parallel()
		// #49's second half: the flag propagation in definedSample is
		// what makes this the composite `geo.Rec{X: 42}` rather than
		// the conversion `geo.Rec({X: 42})`, which go vet rejects as
		// a missing type in a composite literal.
		inner := &node.Struct{
			Name: "Inner", Package: "example.com/geo",
			Fields: []*node.Field{{Name: "X", Type: &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "int"}}},
		}
		rec := &node.Alias{
			Name: "Rec", Package: "example.com/geo",
			Target: &node.TypeRef{TypeKind: node.TypeRefNamed, Package: "example.com/geo", Name: "Inner"},
		}
		r := tableResolver{"example.com/geo.Rec": rec, "example.com/geo.Inner": inner}
		s, _ := langgo.SampleRefFor(&node.TypeRef{
			TypeKind: node.TypeRefNamed, Package: "example.com/geo", Name: "Rec",
		}, "r", r)
		got, err := sampleState().renderSample(s)
		if err != nil || got != "geo.Rec{X: 42}" {
			t.Fatalf("= %q, %v; want geo.Rec{X: 42}, not the conversion form", got, err)
		}
	})

	t.Run("a sample carrying nothing errors instead of rendering empty", func(t *testing.T) {
		t.Parallel()
		// The consumer's request, verbatim: empty output is how a
		// missed arm ships `foo(, )` and fails three files away.
		got, err := sampleState().renderSample(langgo.Sample{Refusal: langgo.RefusedUnresolved})
		if !errors.Is(err, ErrEmptySample) {
			t.Fatalf("err = %v, want ErrEmptySample", err)
		}
		if got != "" {
			t.Fatalf("rendered %q beside the error", got)
		}
		if !strings.Contains(err.Error(), "unresolved") {
			t.Fatalf("error %q should name the refusal", err)
		}
	})
}

// tableResolver answers from a fixed table, mirroring the lang-side
// test fixture of the same shape.
type tableResolver map[string]node.Node

func (r tableResolver) Resolve(t *node.TypeRef) (node.Node, bool) {
	if t == nil {
		return nil, false
	}
	n, ok := r[langgo.QName(t)]
	return n, ok
}

// assertImported fails when path was not registered on the state's
// import set — the property that separates renderSample from every
// hand-written copy, whose text arms could not register anything.
func assertImported(t *testing.T, s *renderState, path string) {
	t.Helper()
	for _, imp := range s.imports.Imports() {
		if imp.Path == path {
			return
		}
	}
	t.Fatalf("import %q not registered; got %+v", path, s.imports.Imports())
}
