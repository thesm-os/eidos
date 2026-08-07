// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package mockgen_test

import (
	"maps"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/reference/mockgen"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites against
// the mockgen plugin.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, mockgen.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(
			t,
			mockgen.New(),
			[]plugintest.GeneratorFixture{
				{
					Name: "package with no annotated interfaces",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return storefixture.New().
							Interface("Plain", nil).
							Build()
					},
				},
				{
					Name: "package with one mock-annotated interface",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return storefixture.New().
							Interface("Reader", func(i *storefixture.InterfaceBuilder) {
								i.Directive(storefixture.Directive("mock"))
							}).
							Build()
					},
				},
			},
		)
	})

	t.Run("options round-trip", func(t *testing.T) {
		t.Parallel()
		plugintest.RunOptionsSuite(t, mockgen.New(), plugintest.OptionsFixture{
			Valid:      map[string]string{"suffix": "Stub"},
			UnknownKey: "no_such_field",
		})
	})
}

// mockedStore returns a store holding one `+gen:mock` interface
// whose methods carry the parameter and return shapes the emit
// path branches on: a named parameter, an unnamed one, and both
// the value-returning and void method forms.
func mockedStore(t *testing.T) *store.Store {
	t.Helper()
	return storefixture.New().
		Interface("UserStore", func(i *storefixture.InterfaceBuilder) {
			i.Directive(storefixture.Directive("mock"))
			i.Method("Get", func(m *storefixture.MethodBuilder) {
				m.Param("id", storefixture.Named("string"))
				m.Return(storefixture.Named("string"))
				m.Return(storefixture.Named("error"))
			})
			// An unnamed parameter drives the `arg<N>` fallback the
			// dispatch body needs to reference it.
			i.Method("Put", func(m *storefixture.MethodBuilder) {
				m.Param("", storefixture.Named("string"))
				m.Return(storefixture.Named("error"))
			})
			// A void method drives the no-return dispatch branch.
			i.Method("Close", nil)
		}).
		Build()
}

// generateMocks drives mockgen over s and returns the emitted
// structs keyed by name.
func generateMocks(t *testing.T, s *store.Store) map[string]*emit.Struct {
	t.Helper()
	if err := mockgen.New().Generate(&plugin.GeneratorContext{
		Store: s, Reader: store.NewReader(s), Diag: diag.New(),
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := map[string]*emit.Struct{}
	for _, st := range store.NewReader(s).EmitStructs().Slice() {
		out[st.Name] = st
	}
	return out
}

func TestGenerate_EmitsFuncValuedMock(t *testing.T) {
	t.Parallel()

	t.Run("emits one mock struct per annotated interface", func(t *testing.T) {
		t.Parallel()
		structs := generateMocks(t, mockedStore(t))

		mock, ok := structs["UserStoreMock"]
		if !ok {
			t.Fatalf("expected a UserStoreMock struct; got %v", slices.Sorted(maps.Keys(structs)))
		}
		// One Func field and one dispatching method per source
		// method — the shape that makes the mock assignable to the
		// interface while staying configurable per test.
		if got := len(mock.Fields); got != 3 {
			t.Errorf("mock should carry one Func field per method, got %d", got)
		}
		if got := len(mock.Methods); got != 3 {
			t.Errorf("mock should carry one method per source method, got %d", got)
		}
	})

	t.Run("each method dispatches through its Func field", func(t *testing.T) {
		t.Parallel()
		mock := generateMocks(t, mockedStore(t))["UserStoreMock"]

		byName := map[string]*emit.Method{}
		for _, m := range mock.Methods {
			byName[m.Name] = m
		}
		for _, name := range []string{"Get", "Put", "Close"} {
			m, ok := byName[name]
			if !ok {
				t.Fatalf("mock is missing method %q", name)
			}
			if len(m.Body) != 1 {
				t.Fatalf("%s should dispatch in a single statement, got %d", name, len(m.Body))
			}
		}
		// A void method dispatches without a return; a
		// value-returning one returns the call's result.
		if got := byName["Close"].Body[0].Returns; len(got) != 0 {
			t.Errorf("void method should dispatch without returning, got %d return exprs", len(got))
		}
		if got := byName["Get"].Body[0].Returns; len(got) == 0 {
			t.Errorf("value-returning method should return its dispatch call")
		}
	})

	t.Run("an unnamed parameter gets a positional identifier", func(t *testing.T) {
		t.Parallel()
		mock := generateMocks(t, mockedStore(t))["UserStoreMock"]

		var put *emit.Method
		for _, m := range mock.Methods {
			if m.Name == "Put" {
				put = m
			}
		}
		if put == nil {
			t.Fatalf("mock is missing method Put")
		}
		if len(put.Params) != 1 {
			t.Fatalf("Put should take one parameter, got %d", len(put.Params))
		}
		// The dispatch body has to name the parameter to forward it,
		// so an unnamed source parameter becomes arg0.
		if got := put.Params[0].Name; got != "arg0" {
			t.Errorf("unnamed parameter should be named arg0, got %q", got)
		}
	})

	t.Run("a negated directive suppresses the mock", func(t *testing.T) {
		t.Parallel()
		s := storefixture.New().
			Interface("Skipped", func(i *storefixture.InterfaceBuilder) {
				i.Directive(storefixture.Directive("mock", storefixture.Negated()))
				i.Method("Do", nil)
			}).
			Build()
		if got := generateMocks(t, s); len(got) != 0 {
			t.Errorf("-gen:mock should emit nothing; got %v", slices.Sorted(maps.Keys(got)))
		}
	})
}

func TestGenerate_GenericInterface(t *testing.T) {
	t.Parallel()

	t.Run("type parameters carry through to the mock struct", func(t *testing.T) {
		t.Parallel()
		s := storefixture.New().
			Interface("Cache", func(i *storefixture.InterfaceBuilder) {
				i.Directive(storefixture.Directive("mock"))
				i.TypeParam("T", storefixture.Constraint(storefixture.Named("any")))
				i.Method("Load", func(m *storefixture.MethodBuilder) {
					m.Param("key", storefixture.Named("string"))
					m.Return(storefixture.Named("T"))
				})
			}).
			Build()

		mock, ok := generateMocks(t, s)["CacheMock"]
		if !ok {
			t.Fatalf("expected a CacheMock struct")
		}
		// The mock must be generic in the same parameters as its
		// interface, or it cannot implement it.
		if got := len(mock.TypeParams); got != 1 {
			t.Fatalf("mock should carry the interface's type parameters, got %d", got)
		}
		if got := mock.TypeParams[0].Name; got != "T" {
			t.Errorf("type parameter name = %q, want T", got)
		}
	})
}

func TestGenerate_EmitSideInterface(t *testing.T) {
	t.Parallel()

	t.Run("mocks interfaces another generator emitted", func(t *testing.T) {
		t.Parallel()
		// mockgen runs after foundation generators, so an interface
		// may exist only on the emit side. That path groups by the
		// host emit package rather than by source package.
		c := builder.For("fixture").WithTarget(emit.Target{})
		built, err := c.Package("users", "example.com/users").
			Interface("Repo", func(i *builder.InterfaceBuilder) {
				i.Method("Get", func(m *builder.MethodBuilder) {
					m.Param("id", emit.Builtin("string"))
					m.Return(emit.Builtin("error"))
				})
			}).
			Build()
		if err != nil {
			t.Fatalf("build emit fixture: %v", err)
		}
		s := store.New()
		if err := s.Emit().AddPackage(built); err != nil {
			t.Fatalf("seed emit package: %v", err)
		}

		if _, ok := generateMocks(t, s)["RepoMock"]; !ok {
			t.Errorf("emit-side interface should be mocked; got %v",
				slices.Sorted(maps.Keys(generateMocks(t, s))))
		}
	})
}
