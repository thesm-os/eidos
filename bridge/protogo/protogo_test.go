// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package protogo_test

import (
	"testing"

	"go.thesmos.sh/eidos/bridge/protogo"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/frontend/protobuf"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites against
// the protogo bridge annotator: the universal
// [plugintest.RunSuite] for stability / role / capability
// contracts, plus the per-role [plugintest.RunAnnotatorSuite]
// for idempotency / frozen-store / diagnostic discipline against
// representative source-side fixtures.
//
// The bridge annotator stamps cross-frontend meta keys on source
// nodes loaded by the proto frontend; for the conformance pass
// we drive it against language-agnostic synthetic stores —
// fixture-level shape suffices for the determinism / frozen-
// store properties the suite asserts.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, protogo.New())
	})

	t.Run("annotator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunAnnotatorSuite(
			t,
			protogo.New(),
			[]plugintest.AnnotatorFixture{
				{
					Name: "empty package",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return storefixture.New().Build()
					},
				},
				{
					Name: "package with one struct",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return storefixture.New().
							Struct("User", nil).
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

// protoStore returns a store whose single package carries the
// proto frontend marker, so the bridge treats it as translatable.
// build populates the package.
func protoStore(t *testing.T, build func(*storefixture.Builder)) *store.Store {
	t.Helper()
	b := storefixture.New().Package("pb", "example.com/pb")
	protobuf.MetaFrontend.Set(b.PackageNode().EnsureMeta(), protobuf.FrontendName, "test")
	if build != nil {
		build(b)
	}
	return b.Build()
}

// annotateProto drives the bridge over s.
func annotateProto(t *testing.T, s *store.Store) {
	t.Helper()
	if err := protogo.New().Annotate(&plugin.AnnotatorContext{
		Store: s, Reader: store.NewReader(s), Diag: diag.New(),
	}); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
}

func TestAnnotate_TranslatesRPCSignatureTypes(t *testing.T) {
	t.Parallel()

	// RPC param and return types reach the bridge through the
	// interface walk, which is a separate path from the
	// field-aware struct walk.
	newStore := func(t *testing.T) *store.Store {
		t.Helper()
		return protoStore(t, func(b *storefixture.Builder) {
			b.Interface("Greeter", func(i *storefixture.InterfaceBuilder) {
				i.Method("Say", func(m *storefixture.MethodBuilder) {
					m.Param("name", storefixture.Named("string"))
					m.Param("tags", storefixture.Slice(storefixture.Named("string")))
					m.Param("attrs", storefixture.Map(
						storefixture.Named("string"), storefixture.Named("int32"),
					))
					m.Return(storefixture.Named("bool"))
				})
			})
		})
	}

	t.Run("scalar, slice and map parameters are stamped with Go types", func(t *testing.T) {
		t.Parallel()
		s := newStore(t)
		annotateProto(t, s)

		iface := store.NewReader(s).Interfaces().Slice()[0]
		params := iface.Methods[0].Params
		want := []string{"string", "[]string", "map[string]int32"}
		for i, w := range want {
			got, ok := protogo.MetaGoType.Get(params[i].Type.Meta())
			if !ok {
				t.Errorf("param %d carries no go.type stamp", i)
				continue
			}
			if got != w {
				t.Errorf("param %d go.type = %q, want %q", i, got, w)
			}
		}
		if got, _ := protogo.MetaGoType.Get(iface.Methods[0].Returns[0].Type.Meta()); got != "bool" {
			t.Errorf("return go.type = %q, want bool", got)
		}
	})
}

func TestAnnotate_WellKnownTypes(t *testing.T) {
	t.Parallel()

	t.Run("a well-known ref gets both a Go type and its import", func(t *testing.T) {
		t.Parallel()
		s := protoStore(t, func(b *storefixture.Builder) {
			b.Interface("Clock", func(i *storefixture.InterfaceBuilder) {
				i.Method("Now", func(m *storefixture.MethodBuilder) {
					ref := storefixture.Named("Timestamp")
					protobuf.MetaWellKnown.Set(ref.EnsureMeta(), "Timestamp", "test")
					m.Return(ref)
				})
			})
		})
		annotateProto(t, s)

		ret := store.NewReader(s).Interfaces().Slice()[0].Methods[0].Returns[0].Type
		if got, _ := protogo.MetaGoType.Get(ret.Meta()); got != "*timestamppb.Timestamp" {
			t.Errorf("well-known go.type = %q, want *timestamppb.Timestamp", got)
		}
		// The import is stamped separately so the render site can
		// register it without parsing the type string.
		want := "google.golang.org/protobuf/types/known/timestamppb"
		if got, _ := protogo.MetaGoImport.Get(ret.Meta()); got != want {
			t.Errorf("well-known go.import = %q, want %q", got, want)
		}
	})

	t.Run("an unknown named ref is left untranslated", func(t *testing.T) {
		t.Parallel()
		// Named message / enum refs are deliberately not stamped:
		// the backend renders them through emit.External instead.
		s := protoStore(t, func(b *storefixture.Builder) {
			b.Interface("Svc", func(i *storefixture.InterfaceBuilder) {
				i.Method("Get", func(m *storefixture.MethodBuilder) {
					m.Return(storefixture.Named("Outer.Inner"))
				})
			})
		})
		annotateProto(t, s)

		ret := store.NewReader(s).Interfaces().Slice()[0].Methods[0].Returns[0].Type
		if got, ok := protogo.MetaGoType.Get(ret.Meta()); ok {
			t.Errorf("named message ref should not be stamped; got %q", got)
		}
	})
}
