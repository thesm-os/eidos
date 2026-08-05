// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shapewriter_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/reference/shapewriter"
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
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return storefixture.New().
							Struct("Plain", nil).
							Build()
					},
				},
				{
					Name: "package with three structs",
					BuildStore: func(t *testing.T) *store.Store {
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
func annotate(t *testing.T, build func(*storefixture.StructBuilder)) *node.Struct {
	t.Helper()
	s := storefixture.New().Struct("Sink", build).Build()
	p := shapewriter.New()
	if err := p.Annotate(&plugin.AnnotatorContext{
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

func TestOnStruct_WriterDetection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		build        func(*storefixture.StructBuilder)
		wantDetected bool
		// wantLink records whether a method back-link is expected.
		// The qname itself is package-qualified, so it is derived
		// from the fixture rather than hardcoded.
		wantLink bool
	}{
		{
			name:         "canonical Write signature is detected",
			build:        writeMethod,
			wantDetected: true,
			wantLink:     true,
		},
		{
			name:         "a struct with no methods is not a writer",
			build:        nil,
			wantDetected: false,
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
			wantDetected: false,
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
			wantDetected: false,
		},
		{
			name: "a Write with the wrong arity is rejected",
			build: func(b *storefixture.StructBuilder) {
				b.Method("Write", func(m *storefixture.MethodBuilder) {
					m.Param("p", storefixture.Slice(storefixture.Named("byte")))
					m.Return(storefixture.Named("error"))
				})
			},
			wantDetected: false,
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
			wantDetected: false,
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
			wantDetected: true,
			wantLink:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := annotate(t, tc.build)
			if got, _ := shapewriter.Detected.Get(s.Meta()); got != tc.wantDetected {
				t.Errorf("detected = %v, want %v", got, tc.wantDetected)
			}
			want := ""
			if tc.wantLink {
				want = s.QName() + ".Write"
			}
			if got, _ := shapewriter.MethodQName.Get(s.Meta()); got != want {
				t.Errorf("method qname = %q, want %q", got, want)
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
			b.Directive(&directive.Directive{Name: shapewriter.DirectiveName, Negated: true})
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
			b.Directive(&directive.Directive{Name: shapewriter.DirectiveName})
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
