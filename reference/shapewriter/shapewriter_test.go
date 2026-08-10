// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shapewriter_test

import (
	"io"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/reference/shapewriter"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites against
// the shapewriter plugin: the universal [plugintest.RunSuite] for
// stability / role / capability contracts, plus the per-role
// [plugintest.RunAnnotatorSuite] for idempotency / frozen-store
// / diagnostic discipline against representative source fixtures.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, shapewriter.New())
	})

	t.Run("annotator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunAnnotatorSuite(
			t,
			shapewriter.New(),
			[]plugintest.AnnotatorFixture{
				{
					Name: "package with no relevant structs",
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return storefixture.New().
							Struct("Plain", nil).
							Build()
					},
				},
				{
					Name: "package with three structs",
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return storefixture.New().
							Struct("User", nil).
							Struct("Order", nil).
							Struct("Invoice", nil).
							Build()
					},
				},
			},
		)
	})
}

// The types below are the heuristic's oracle. Each one is a real Go
// declaration of a signature the detection table also builds as a
// node fixture, so the question "should this have been detected?" is
// answered by the compiler — `implementsWriter` — instead of by a
// hand-written boolean recording the test author's memory of what
// io.Writer's signature is.
//
// That matters because the plugin's whole claim is agreement with
// io.Writer. A `wantDetected: true` beside a fixture nobody checked
// asserts the heuristic against itself; asserting against
// `x.(io.Writer)` asserts it against the definition. The variadic
// row is what the difference bought: `Write(...[]byte) (int, error)`
// reads as `[]byte` in the node graph and passed the old table's
// eyeball review, while the compiler had never considered it a
// writer.
//
// The two halves cannot be derived from one another here — the
// reference module carries no Go frontend, so nothing in this package
// can turn Go source into a node store. They are written side by side
// in one table row and must be reviewed as a pair. Binding them
// mechanically needs a frontend, and belongs in cmd/eidos-reference,
// which already depends on both.

// canonicalWriter is the signature the heuristic targets.
type canonicalWriter struct{}

func (canonicalWriter) Write(p []byte) (int, error) { return len(p), nil }

// namedReturnWriter writes the same signature with named results,
// which changes the source text and not the method set.
type namedReturnWriter struct{}

func (namedReturnWriter) Write(p []byte) (n int, err error) { return len(p), nil }

// aliasWriter spells the element type as `uint8`, byte's alias.
type aliasWriter struct{}

func (aliasWriter) Write(p []uint8) (int, error) { return len(p), nil }

// noMethodWriter has no methods at all.
type noMethodWriter struct{}

// stringParamWriter takes a string where io.Writer takes a byte slice.
type stringParamWriter struct{}

func (stringParamWriter) Write(s string) (int, error) { return len(s), nil }

// swappedReturnWriter returns the right types in the wrong order.
type swappedReturnWriter struct{}

//nolint:revive,staticcheck // error-return/ST1008: the misplaced error is the point — this is the near miss the heuristic must reject.
func (swappedReturnWriter) Write(p []byte) (error, int) { return nil, len(p) }

// shortReturnWriter drops the byte count.
type shortReturnWriter struct{}

func (shortReturnWriter) Write([]byte) error { return nil }

// renamedWriter has the right signature under the wrong name.
type renamedWriter struct{}

func (renamedWriter) Emit(p []byte) (int, error) { return len(p), nil }

// variadicByteWriter takes `...byte`, which a frontend records as the
// element type `byte` — not a slice, and not an io.Writer.
type variadicByteWriter struct{}

func (variadicByteWriter) Write(p ...byte) (int, error) { return len(p), nil }

// variadicSliceWriter takes `...[]byte`, whose recorded element type
// is the very `[]byte` the heuristic looks for. Go does not consider
// it an io.Writer, and the parameter's variadic marker is the only
// thing that says so.
type variadicSliceWriter struct{}

func (variadicSliceWriter) Write(p ...[]byte) (int, error) { return len(p), nil }

// implementsWriter reports whether the compiler considers v an
// io.Writer. It is the expected value for every heuristic case.
func implementsWriter(v any) bool {
	_, ok := v.(io.Writer)
	return ok
}

// writeMethod adds the canonical io.Writer signature —
// Write([]byte) (int, error) — to the struct under construction.
func writeMethod(b *storefixture.StructBuilder) {
	b.Method("Write", func(m *storefixture.MethodBuilder) {
		m.Param("p", storefixture.Slice(storefixture.Named("byte")))
		m.Return(storefixture.Named("int"))
		m.Return(storefixture.Named("error"))
	})
}

// annotate runs the plugin over a one-struct fixture and returns
// the struct so the test can read the stamped meta back.
func annotate(t *testing.T, build func(*storefixture.StructBuilder)) *sdk.Struct {
	t.Helper()
	s := storefixture.New().Struct("Sink", build).Build()
	p := shapewriter.New()
	if err := p.Annotate(&sdk.AnnotatorContext{
		Store: s, Reader: store.NewReader(s), Diag: diag.New(),
	}); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	structs := store.NewReader(s).Structs().Slice()
	if len(structs) != 1 {
		t.Fatalf("fixture should hold exactly one struct, got %d", len(structs))
	}
	return structs[0]
}

// TestOnStruct_WriterDetection pins the heuristic against the
// compiler. Every case supplies a node fixture and the equivalent Go
// declaration; the expected verdict is whichever answer Go gives for
// that declaration, so the table cannot encode a wrong idea of what
// io.Writer is.
//
// Scope: the equivalence holds over a struct's own directly declared
// methods, which is all the fixtures build. Method promotion through
// an embedded field is a separate question the heuristic does not
// answer — see the package notes.
func TestOnStruct_WriterDetection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		build func(*storefixture.StructBuilder)
		// goShape is the same signature written as real Go. The
		// compiler's verdict on it is the expected verdict.
		goShape any
	}{
		{
			name:    "canonical Write signature is detected",
			build:   writeMethod,
			goShape: canonicalWriter{},
		},
		{
			name: "named result slots do not change the method set",
			build: func(b *storefixture.StructBuilder) {
				b.Method("Write", func(m *storefixture.MethodBuilder) {
					m.Param("p", storefixture.Slice(storefixture.Named("byte")))
					m.NamedReturn("n", storefixture.Named("int"))
					m.NamedReturn("err", storefixture.Named("error"))
				})
			},
			goShape: namedReturnWriter{},
		},
		{
			name:    "a struct with no methods is not a writer",
			build:   nil,
			goShape: noMethodWriter{},
		},
		{
			name: "a Write taking the wrong parameter type is rejected",
			build: func(b *storefixture.StructBuilder) {
				b.Method("Write", func(m *storefixture.MethodBuilder) {
					m.Param("s", storefixture.Named("string"))
					m.Return(storefixture.Named("int"))
					m.Return(storefixture.Named("error"))
				})
			},
			goShape: stringParamWriter{},
		},
		{
			name: "a Write returning the wrong types is rejected",
			build: func(b *storefixture.StructBuilder) {
				b.Method("Write", func(m *storefixture.MethodBuilder) {
					m.Param("p", storefixture.Slice(storefixture.Named("byte")))
					m.Return(storefixture.Named("error"))
					m.Return(storefixture.Named("int"))
				})
			},
			goShape: swappedReturnWriter{},
		},
		{
			name: "a Write with the wrong arity is rejected",
			build: func(b *storefixture.StructBuilder) {
				b.Method("Write", func(m *storefixture.MethodBuilder) {
					m.Param("p", storefixture.Slice(storefixture.Named("byte")))
					m.Return(storefixture.Named("error"))
				})
			},
			goShape: shortReturnWriter{},
		},
		{
			name: "a non-Write method of the right shape is ignored",
			build: func(b *storefixture.StructBuilder) {
				b.Method("Emit", func(m *storefixture.MethodBuilder) {
					m.Param("p", storefixture.Slice(storefixture.Named("byte")))
					m.Return(storefixture.Named("int"))
					m.Return(storefixture.Named("error"))
				})
			},
			goShape: renamedWriter{},
		},
		{
			name: "byte's uint8 alias is accepted",
			build: func(b *storefixture.StructBuilder) {
				b.Method("Write", func(m *storefixture.MethodBuilder) {
					m.Param("p", storefixture.Slice(storefixture.Named("uint8")))
					m.Return(storefixture.Named("int"))
					m.Return(storefixture.Named("error"))
				})
			},
			goShape: aliasWriter{},
		},
		{
			// `...byte` records the element type, so the parameter is
			// not a slice and the heuristic rejects it on type alone.
			name: "a variadic byte parameter is rejected",
			build: func(b *storefixture.StructBuilder) {
				b.Method("Write", func(m *storefixture.MethodBuilder) {
					m.Variadic("p", storefixture.Named("byte"))
					m.Return(storefixture.Named("int"))
					m.Return(storefixture.Named("error"))
				})
			},
			goShape: variadicByteWriter{},
		},
		{
			// `...[]byte` records `[]byte` as the parameter's type, so
			// nothing but the variadic marker distinguishes it from the
			// canonical signature.
			name: "a variadic slice parameter is rejected",
			build: func(b *storefixture.StructBuilder) {
				b.Method("Write", func(m *storefixture.MethodBuilder) {
					m.Variadic("p", storefixture.Slice(storefixture.Named("byte")))
					m.Return(storefixture.Named("int"))
					m.Return(storefixture.Named("error"))
				})
			},
			goShape: variadicSliceWriter{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := implementsWriter(tc.goShape)
			s := annotate(t, tc.build)
			if got, _ := shapewriter.Detected.Get(s.Meta()); got != want {
				t.Errorf("detected = %v, but the compiler reports %T implements io.Writer = %v",
					got, tc.goShape, want)
			}
			// The back-link is recorded exactly when the heuristic
			// matched, so it tracks the compiler's verdict too.
			wantQName := ""
			if want {
				wantQName = s.QName() + ".Write"
			}
			if got, _ := shapewriter.MethodQName.Get(s.Meta()); got != wantQName {
				t.Errorf("method qname = %q, want %q", got, wantQName)
			}
		})
	}
}

func TestOnStruct_DirectiveOverridesHeuristic(t *testing.T) {
	t.Parallel()

	t.Run("a negated directive suppresses a real match", func(t *testing.T) {
		t.Parallel()
		s := annotate(t, func(b *storefixture.StructBuilder) {
			writeMethod(b)
			b.Directive(&sdk.Directive{Name: shapewriter.DirectiveName, Negated: true})
		})
		if detected, _ := shapewriter.Detected.Get(s.Meta()); detected {
			t.Errorf("-gen:%s should suppress detection on a real writer", shapewriter.DirectiveName)
		}
		// The back-link is only recorded when the heuristic matched
		// AND detection survived, so suppression clears it.
		if got, _ := shapewriter.MethodQName.Get(s.Meta()); got != "" {
			t.Errorf("suppressed struct should carry no method qname; got %q", got)
		}
	})

	t.Run("a positive directive forces detection without a Write method", func(t *testing.T) {
		t.Parallel()
		s := annotate(t, func(b *storefixture.StructBuilder) {
			b.Directive(&sdk.Directive{Name: shapewriter.DirectiveName})
		})
		if detected, _ := shapewriter.Detected.Get(s.Meta()); !detected {
			t.Errorf("+gen:%s should force detection", shapewriter.DirectiveName)
		}
		// Documented behaviour: a directive-driven match with no real
		// Write method records detected=true and an empty back-link.
		if got, _ := shapewriter.MethodQName.Get(s.Meta()); got != "" {
			t.Errorf("directive-forced match has no method to link; got %q", got)
		}
	})
}
