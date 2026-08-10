// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package mockgen_test

import (
	"maps"
	"slices"
	"testing"

	backendgolang "go.thesmos.sh/eidos/backend/golang"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/golangtest"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/reference/mockgen"
	"go.thesmos.sh/eidos/reference/repogen"
	"go.thesmos.sh/eidos/sdk"
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
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return storefixture.New().
							Interface("Plain", nil).
							Build()
					},
				},
				{
					Name: "package with one mock-annotated interface",
					BuildStore: func(t *testing.T) *sdk.Store {
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

// modulePath is the module the rendered fixture is built inside. It
// has to be pinned rather than derived: the mocks land in the
// `users_test` external test package, whose import path is not a
// prefix-strippable form of the directory they render into, so the
// harness would otherwise fall back to its placeholder module and the
// hand-written `example.com/app/users` import would not resolve.
const modulePath = "example.com/app"

// fixtureBuilder is the source package the rendered fixtures drive.
//
// One package carrying every shape the emit path branches on, because
// a toolchain assertion costs a second and a fixture per branch would
// cost a minute: a struct routed through repogen (so the emit-side
// mock path has an interface to consume), an interface with named,
// anonymous, variadic and void method forms, and a generic interface.
func fixtureBuilder() *storefixture.Builder {
	return storefixture.New().
		Package("users", modulePath+"/users").
		// repogen synthesises `UserRepository` on the emit side, which
		// is what gives mockgen's emit-side entry point something to
		// mock — the composition the plugin exists for.
		Struct("User", func(s *storefixture.StructBuilder) {
			s.Directive(storefixture.Directive("repo"))
			s.Field("ID", storefixture.Named("string"), nil)
		}).
		Interface("UserStore", func(i *storefixture.InterfaceBuilder) {
			i.Directive(storefixture.Directive("mock"))
			i.Method("Get", func(m *storefixture.MethodBuilder) {
				m.Param("ctx", storefixture.PkgNamed("context", "Context"))
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
			// A parameter named for the receiver letter: the receiver is
			// disambiguated against the signature, or the parameter
			// shadows it and `<recv>.PutFunc` resolves against a string.
			i.Method("Shadow", func(m *storefixture.MethodBuilder) {
				m.Param("u", storefixture.Named("string"))
				m.Return(storefixture.Named("error"))
			})
			// A declared `arg0` beside an anonymous parameter: the
			// fallback for the second collides with the first unless the
			// identifiers are uniquified across the whole list.
			i.Method("Collide", func(m *storefixture.MethodBuilder) {
				m.Param("arg0", storefixture.Named("string"))
				m.Param("", storefixture.Named("string"))
				m.Return(storefixture.Named("error"))
			})
			// A void method drives the no-return dispatch branch.
			i.Method("Close", nil)
			// A variadic method is the shape that compiles either way
			// and satisfies the interface only one of them: see the
			// satisfaction assertion in satisfactionFile.
			i.Method("Log", func(m *storefixture.MethodBuilder) {
				m.Param("format", storefixture.Named("string"))
				m.Variadic("args", storefixture.Named("any"))
			})
		}).
		Interface("Cache", func(i *storefixture.InterfaceBuilder) {
			i.Directive(storefixture.Directive("mock"))
			i.TypeParam("T", storefixture.Constraint(storefixture.Named("any")))
			i.Method("Load", func(m *storefixture.MethodBuilder) {
				m.Param("key", storefixture.Named("string"))
				m.Return(storefixture.Named("T"))
				m.Return(storefixture.Named("bool"))
			})
		})
}

// satisfactionFile is the one support file the projection cannot
// supply: an assertion rather than a declaration.
//
// It is a support file rather than a
// [golangtest.Generated.AssertSatisfies] call because mockgen's only
// output is a `_test.go`: AssertSatisfies lands its assertion in the
// package of the first non-test file the run produced, which here is
// repogen's, and a mock declared in `users_test` is not nameable from
// there. The check itself is the point — a mock that drops a variadic
// marker, or loses a method through an embed, compiles cleanly and
// implements nothing.
func satisfactionFile() golangtest.File {
	return golangtest.GoFile("users/satisfies_test.go", `package users_test

import "example.com/app/users"

var (
	_ users.UserStore      = (*UserStoreMock)(nil)
	_ users.Cache[string]  = (*CacheMock[string])(nil)
	_ users.UserRepository = (*UserRepositoryMock)(nil)
)
`)
}

// renderFixture drives repogen and mockgen over [fixtureBuilder] and
// adopts every rendered file.
//
// The Go the mocks are built against is projected from the same
// builder that drove the run, so the declarations they double cannot
// drift from the declarations they were generated from. Only the
// satisfaction assertion is written by hand, because it is a claim
// rather than a declaration.
//
// The returned [golangtest.Generated] caches the module it assembles,
// so the toolchain assertions in one test share a single setup. Each
// caller still pays for its own render, which is why the toolchain
// assertions all live in one test function.
func renderFixture(t *testing.T) *golangtest.Generated {
	t.Helper()
	fixture := fixtureBuilder()
	return golangtest.Render(t, backendgolang.New(), fixture.PackageNode(),
		repogen.New(), mockgen.New()).
		WithModulePath(modulePath).
		WithSource(golangtest.GoFile(fixture.GoSource()), satisfactionFile())
}

// TestRendered_OutputIsValidGo is the assertion every other one in
// this file is a proxy for.
//
// One toolchain pass over one fixture, deliberately: `go vet` builds
// the module *and* type-checks the `_test.go` files `go build` skips,
// which is where every mock this plugin emits lands. It therefore
// covers the satisfaction assertions satisfactionFile supplies too —
// the check that a mock is assignable to the interface it doubles,
// which no structural assertion can make.
func TestRendered_OutputIsValidGo(t *testing.T) {
	t.Parallel()

	gen := renderFixture(t)

	t.Run("the non-test output builds", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
	})

	t.Run("the mocks type-check and satisfy their interfaces", func(t *testing.T) {
		t.Parallel()
		gen.AssertVets(t)
	})
}

// TestRendered_SourceSideMock covers the mock synthesised from a
// source-side `+gen:mock` interface.
func TestRendered_SourceSideMock(t *testing.T) {
	t.Parallel()

	src := renderFixture(t).Suffixed(t, "userstore_mock_test.go")

	t.Run("lands in the external test package", func(t *testing.T) {
		t.Parallel()
		// The `_test` suffix is what keeps the mock out of production
		// binaries and keeps its references back into the regular
		// package qualified rather than elided.
		src.AssertPackage(t, "users_test")
	})

	t.Run("carries the machine-written conventions", func(t *testing.T) {
		t.Parallel()
		src.AssertGeneratedHeader(t).
			AssertFormatted(t).
			AssertDocumented(t)
	})

	t.Run("imports nothing the signatures do not need", func(t *testing.T) {
		t.Parallel()
		// A generated import is an API commitment: every consumer whose
		// module does not already require it breaks, and nothing else in
		// this suite would notice the addition.
		src.AssertImportsOnly(t, "context")
	})

	t.Run("offers exactly the surface a consumer configures", func(t *testing.T) {
		t.Parallel()
		// The named assertions below each say a declaration is present;
		// only this one says nothing *else* is. A mock's whole surface
		// is what a test writer assigns to, so an added field or a
		// renamed one is a change they have to react to — and the golden
		// diff is that list, rather than the two hundred lines a byte
		// golden would churn on a comment reflow.
		golangtest.AssertAPIGolden(t, src, "testdata/userstore_mock.api.golden")
	})

	t.Run("declares one func-valued field per method", func(t *testing.T) {
		t.Parallel()
		src.AssertField(t, "UserStoreMock", "GetFunc", "func(context.Context, string) (string, error)").
			AssertField(t, "UserStoreMock", "PutFunc", "func(string) error").
			AssertField(t, "UserStoreMock", "CloseFunc", "func()")
	})

	t.Run("declares one dispatching method per source method", func(t *testing.T) {
		t.Parallel()
		// Pointer receivers throughout: the mock is configured after
		// construction, so a value receiver would dispatch through a
		// copy taken before the test set its field.
		src.AssertMethod(t, "UserStoreMock", "Get").
			Signature(t, "(ctx context.Context, id string) (string, error)").
			AssertPointerReceiver(t, true)
		src.AssertMethod(t, "UserStoreMock", "Close").
			Signature(t, "()").
			AssertPointerReceiver(t, true)
	})

	t.Run("names an unnamed parameter positionally", func(t *testing.T) {
		t.Parallel()
		// The dispatch body has to name the parameter to forward it, so
		// an anonymous source parameter becomes arg0.
		src.AssertMethod(t, "UserStoreMock", "Put").Signature(t, "(arg0 string) error")
		src.InMethod(t, "UserStoreMock", "Put").AssertContains(t, "return u.PutFunc(arg0)")
	})

	t.Run("dispatches through the field, returning only where there is one", func(t *testing.T) {
		t.Parallel()
		src.InMethod(t, "UserStoreMock", "Get").
			AssertContains(t, "return u.GetFunc(ctx, id)")
		// A void method dispatches without a return; emitting one would
		// not compile, which is why AssertVets is the assertion behind
		// this one.
		src.InMethod(t, "UserStoreMock", "Close").
			AssertContains(t, "u.CloseFunc()").
			AssertNotContains(t, "return")
	})

	t.Run("keeps a variadic method variadic", func(t *testing.T) {
		t.Parallel()
		// The marker is the whole assertion: `Log(format string, args
		// []any)` compiles, reads almost identically, and does not
		// implement UserStore. The field takes the slice form because
		// the emit layer's func shape carries no variadic marker, and
		// the body forwards the parameter unspread to match.
		src.AssertMethod(t, "UserStoreMock", "Log").
			Signature(t, "(format string, args ...any)")
		src.AssertField(t, "UserStoreMock", "LogFunc", "func(string, []any)")
		src.InMethod(t, "UserStoreMock", "Log").AssertContains(t, "u.LogFunc(format, args)")

		// The receiver must not be the identifier the signature already
		// binds. `Shadow(u string)` on UserStoreMock would otherwise
		// emit `func (u *UserStoreMock) Shadow(u string)`, where the
		// parameter shadows the receiver and `u.ShadowFunc` resolves
		// against a string — output that compiles nowhere.
		src.AssertMethod(t, "UserStoreMock", "Shadow").AssertPointerReceiver(t, true)
		src.InMethod(t, "UserStoreMock", "Shadow").AssertNotContains(t, "u.ShadowFunc")

		// A declared `arg0` beside an anonymous parameter: the second
		// must not fall back onto the first's name. Two parameters of
		// one name do not compile.
		src.InMethod(t, "UserStoreMock", "Collide").
			AssertContains(t, "CollideFunc(arg0, arg1)")
	})
}

// TestRendered_SuffixOption covers the plugin's one option reaching
// the rendered output.
//
// [plugintest.RunOptionsSuite] proves the value round-trips through
// SetOptions; nothing proved it was ever read. A default that quietly
// won would pass both the round-trip suite and every assertion above,
// because they all name the default spelling.
func TestRendered_SuffixOption(t *testing.T) {
	t.Parallel()

	p := golangtest.Driver(t, backendgolang.New(), fixtureBuilder().PackageNode(), mockgen.New()).
		WithPluginOptions(mockgen.Name, map[string]string{"suffix": "Stub"}).
		Build().
		Run("./...")

	src := golangtest.Rendered(t, p).
		WithModulePath(modulePath).
		Suffixed(t, "userstore_mock_test.go")

	t.Run("the configured suffix names the mock struct", func(t *testing.T) {
		t.Parallel()
		src.AssertType(t, "UserStoreStub")
		src.AssertNoType(t, "UserStoreMock")
	})

	t.Run("the methods land on the renamed receiver", func(t *testing.T) {
		t.Parallel()
		// The receiver reference is built from the struct's own node
		// rather than from the name, so a suffix that renamed the type
		// and not the receiver would leave the methods on a type that
		// does not exist.
		src.AssertMethod(t, "UserStoreStub", "Get").AssertPointerReceiver(t, true)
	})
}

// TestRendered_GenericMock covers a generic source interface, whose
// type parameters have to thread through the struct, its receivers and
// its field types together or the mock cannot implement the interface
// at any instantiation.
func TestRendered_GenericMock(t *testing.T) {
	t.Parallel()

	src := renderFixture(t).Suffixed(t, "cache_mock_test.go")

	t.Run("the mock is generic in the interface's parameters", func(t *testing.T) {
		t.Parallel()
		src.AssertType(t, "CacheMock")
		src.AssertField(t, "CacheMock", "LoadFunc", "func(string) (T, bool)")
	})

	t.Run("the receiver carries the type arguments", func(t *testing.T) {
		t.Parallel()
		// A receiver written `(m *CacheMock)` on a generic type does not
		// compile; one written `(m *CacheMock[T])` is what the renderer
		// has to produce from the propagated parameter list.
		src.AssertMethod(t, "CacheMock", "Load").
			Signature(t, "(key string) (T, bool)").
			AssertPointerReceiver(t, true)
		src.InMethod(t, "CacheMock", "Load").AssertContains(t, "return c.LoadFunc(key)")
	})
}

// TestRendered_EmitSideMock covers the interface mockgen never saw in
// source: repogen synthesises `UserRepository` during the same run and
// mockgen doubles it from the emit store.
func TestRendered_EmitSideMock(t *testing.T) {
	t.Parallel()

	src := renderFixture(t).Suffixed(t, "user_mock_test.go")

	t.Run("mocks the interface an upstream generator emitted", func(t *testing.T) {
		t.Parallel()
		src.AssertType(t, "UserRepositoryMock")
		src.AssertMethod(t, "UserRepositoryMock", "Get").
			Signature(t, "(ctx context.Context, id string) (*users.User, error)")
	})

	t.Run("qualifies references back into the regular package", func(t *testing.T) {
		t.Parallel()
		// The mock lives in `users_test`, whose import identity differs
		// from `users`, so same-package elision must stay inert and the
		// entity type must render qualified. Elided, the file would name
		// an undefined `*User` — which is exactly what AssertVets
		// catches and a substring for "User" would not.
		src.AssertImportsOnly(t, "context", modulePath+"/users").
			AssertField(t, "UserRepositoryMock", "ListFunc",
				"func(context.Context) ([]*users.User, error)")
	})
}

// TestGenerate_Suppression covers the two ways a target opts out.
//
// Kept at the store level rather than rendered: both assert an
// absence, and an absent decl produces no file for a [golangtest]
// assertion to address.
func TestGenerate_Suppression(t *testing.T) {
	t.Parallel()

	t.Run("a negated directive suppresses the whole mock", func(t *testing.T) {
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

	t.Run("a negated directive on one method drops only that method", func(t *testing.T) {
		t.Parallel()
		s := storefixture.New().
			Interface("Partial", func(i *storefixture.InterfaceBuilder) {
				i.Directive(storefixture.Directive("mock"))
				i.Method("Kept", nil)
				i.Method("Dropped", func(m *storefixture.MethodBuilder) {
					m.Directive(storefixture.Directive("mock", storefixture.Negated()))
				})
			}).
			Build()

		mock, ok := generateMocks(t, s)["PartialMock"]
		if !ok {
			t.Fatalf("expected a PartialMock struct")
		}
		// A per-method opt-out makes the mock deliberately incomplete,
		// so it is the one shape this plugin emits that cannot satisfy
		// the interface it names — which is why it is not in the
		// rendered fixture.
		if got := len(mock.Methods); got != 1 {
			t.Errorf("mock should carry only the un-negated method, got %d", got)
		}
		for _, m := range mock.Methods {
			if m.Name == "Dropped" {
				t.Errorf("-gen:mock on a method should drop it from the mock")
			}
		}
	})
}

// TestGenerate_EmitSideInterfaceWithoutSource covers the emit-side
// entry point in isolation: an interface that exists only in the emit
// store, with no source package behind it.
func TestGenerate_EmitSideInterfaceWithoutSource(t *testing.T) {
	t.Parallel()

	t.Run("mocks an interface with no source-side counterpart", func(t *testing.T) {
		t.Parallel()
		c := sdk.NewProvenance("fixture").WithTarget(sdk.EmitTarget{})
		built, err := c.Package("users", "example.com/users").
			Interface("Repo", func(i *sdk.InterfaceBuilder) {
				i.Method("Get", func(m *sdk.MethodBuilder) {
					m.Param("id", sdk.Builtin("string"))
					m.Return(sdk.Builtin("error"))
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

		got := generateMocks(t, s)
		if _, ok := got["RepoMock"]; !ok {
			t.Errorf("emit-side interface should be mocked; got %v", slices.Sorted(maps.Keys(got)))
		}
	})
}

// generateMocks drives mockgen over s and returns the emitted
// structs keyed by name.
func generateMocks(t *testing.T, s *sdk.Store) map[string]*sdk.EmitStruct {
	t.Helper()
	if err := mockgen.New().Generate(&sdk.GeneratorContext{
		Store: s, Reader: store.NewReader(s), Diag: diag.New(),
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := map[string]*sdk.EmitStruct{}
	for _, st := range store.NewReader(s).EmitStructs().Slice() {
		out[st.Name] = st
	}
	return out
}
