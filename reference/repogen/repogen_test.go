// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package repogen_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/eidostest/plugintest"
	langgo "go.thesmos.sh/eidos/lang/golang"
	backendgolang "go.thesmos.sh/eidos/lang/golang/backend"
	"go.thesmos.sh/eidos/lang/golang/golangtest"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	"go.thesmos.sh/eidos/reference/repogen"
	"go.thesmos.sh/eidos/sdk"
)

// TestConformance runs the framework conformance suites against
// the repogen plugin: the universal [plugintest.RunSuite] for
// stability / role / capability contracts, plus the per-role
// [plugintest.RunGeneratorSuite] for determinism / frozen-source
// against representative input fixtures, plus
// [plugintest.RunOptionsSuite] for the options round-trip.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, repogen.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(
			t,
			repogen.New(),
			[]plugintest.GeneratorFixture{
				{
					Name: "package with no annotated structs",
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return gofixture.New().
							Struct("Plain", nil).
							Build()
					},
				},
				{
					Name: "package with one repo-annotated struct",
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return gofixture.New().
							Struct("User", func(s *gofixture.StructBuilder) {
								s.Directive(gofixture.Directive("repo"))
								s.Field("ID", gofixture.Named("string"), nil)
							}).
							Build()
					},
				},
				{
					Name: "package with three repo-annotated structs",
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return gofixture.New().
							Struct("User", func(s *gofixture.StructBuilder) {
								s.Directive(gofixture.Directive("repo"))
							}).
							Struct("Order", func(s *gofixture.StructBuilder) {
								s.Directive(gofixture.Directive("repo"))
							}).
							Struct("Invoice", func(s *gofixture.StructBuilder) {
								s.Directive(gofixture.Directive("repo"))
							}).
							Build()
					},
				},
			},
		)
	})

	t.Run("options round-trip", func(t *testing.T) {
		t.Parallel()
		plugintest.RunOptionsSuite(t, repogen.New(), plugintest.OptionsFixture{
			Valid: map[string]string{
				"interface_suffix": "Storage",
				"struct_suffix":    "Storer",
				"naming":           "Pascal",
			},
			UnknownKey: "no_such_field",
		})
	})
}

// TestTemplateSurfaceIsInert pins the one thing embedding
// [sdkgo.Base] changes that is not a declaration: repogen now
// satisfies [sdk.TemplateProvider], where before it implemented none
// of that interface's methods and the backend skipped it entirely.
//
// Satisfying it is not free. The backend's merge walks every
// TemplateProvider twice — once to parse trees, once to fold
// TemplateFuncs into a registry where a name collision between two
// plugins aborts the whole run before a single file is written. A
// plugin joining that merge with an empty contribution has to stay
// empty, so both halves are asserted rather than assumed.
//
// The golden is the empirical half, and it is the one the structural
// assertions cannot stand in for: they say the declared contribution
// is empty, and only a render through the real Go backend says the
// bytes on a consumer's disk are the same as before the plugin had a
// template surface at all. The golden was recorded from the
// pre-migration plugin, so a regression here is a diff against code
// that never implemented [sdk.TemplateProvider].
func TestTemplateSurfaceIsInert(t *testing.T) {
	t.Parallel()

	provider, ok := any(repogen.New()).(sdk.TemplateProvider)
	if !ok {
		t.Fatalf("repogen.New() does not satisfy sdk.TemplateProvider; " +
			"the assertions below exist to bound what satisfying it costs")
	}

	t.Run("declares no template tree", func(t *testing.T) {
		t.Parallel()
		// (nil, false) is the supported answer for "no templates for
		// this language" and what the backend keys its per-plugin
		// parse on. A non-nil filesystem alongside ok=false is the
		// shape that reads as a tree to half the callers.
		fsys, has := provider.Templates(langgo.Language)
		if has || fsys != nil {
			t.Errorf("Templates(%q) = %v, %v; want nil, false — repogen renders "+
				"every decl through the backend's own kind templates",
				langgo.Language, fsys, has)
		}
	})

	t.Run("registers no template helper", func(t *testing.T) {
		t.Parallel()
		// A plugin's helpers are bound to its own templates at parse
		// time, so a bundle shipped without a tree is unreachable by
		// construction — and every name in it is one more chance to
		// collide with a plugin whose helpers can actually be called.
		if got := provider.TemplateFuncs(langgo.Language); len(got) != 0 {
			t.Errorf("TemplateFuncs(%q) registered %d helper(s) for a plugin with "+
				"no template able to call them: %v", langgo.Language, len(got), got)
		}
	})

	t.Run("overrides no backend builtin", func(t *testing.T) {
		t.Parallel()
		// An override replaces a helper for this plugin's templates
		// only. With no templates there is nothing to replace it in.
		if got := provider.TemplateOverrides(langgo.Language); len(got) != 0 {
			t.Errorf("TemplateOverrides(%q) replaced %d builtin(s) with no template "+
				"to apply them to: %v", langgo.Language, len(got), got)
		}
	})

	t.Run("renders the bytes it rendered before it had a template surface", func(t *testing.T) {
		t.Parallel()
		render(t, nil, "User").Primary(t).AssertGolden(t, "testdata/user_repo.golden")
	})
}

// crudMethod names one method of the canonical CRUD set together with
// the signature repogen must emit for it under default options, and is
// what the per-method subtests iterate.
//
// The signature is spelled the way [golangtest.Decl.Signature] wants
// it — no `func`, no name — because the assertion that produced the
// Decl already established both.
type crudMethod struct {
	name string
	sig  string
}

// crudSet is the contract repogen's docblock claims: `Get` / `List` /
// `Save` / `Delete`, every one taking a context first and reporting an
// error last.
//
//nolint:gochecknoglobals // a test table.
var crudSet = []crudMethod{
	{"Get", "(ctx context.Context, id string) (*User, error)"},
	{"List", "(ctx context.Context) ([]*User, error)"},
	{"Save", "(ctx context.Context, value *User) error"},
	{"Delete", "(ctx context.Context, id string) error"},
}

// TestRender_DefaultOptions is the plugin's real contract: the Go it
// puts on a consumer's disk.
//
// The toolchain assertions are the ones every structural check below
// is a proxy for — a signature assertion passes just as well against a
// file that references a type nobody declared. They cost a `go`
// invocation each (a second or two), so they run together in one
// subtest, against a [golangtest.Generated] that caches its built
// module; the structural assertions carry the rest for free.
func TestRender_DefaultOptions(t *testing.T) {
	t.Parallel()

	gen := render(t, nil, "User").WithSource(contractSource())
	src := gen.Primary(t)

	t.Run("the emitted package builds and honours the CRUD contract", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
		// Broader than the build on the axis a generator gets wrong:
		// vet reaches the analyses that type-check clean — a shadowed
		// receiver, an unreachable return after a slot insertion, a
		// context dropped on the floor. Shares the cached module with
		// the build above, so it costs one `go` invocation, not a
		// second setup.
		gen.AssertVets(t)
		// The assertion no substring reaches. A dropped variadic, a
		// parameter typed `User` where the contract says `*User`, or a
		// method landing on the value receiver when the consumer holds a
		// pointer all compile perfectly and satisfy nothing — and this
		// pins the emitted struct against a hand-written spelling of the
		// contract rather than against repogen's own interface, so a
		// consistent drift in both is still caught.
		gen.AssertSatisfies(t, "UserRepo", "UserRepositoryContract")
	})

	t.Run("routes to <source-basename>_repo.go beside its origin", func(t *testing.T) {
		t.Parallel()
		if got := src.Path(); got != "test/user_repo.go" {
			t.Errorf("rendered file is %q, want test/user_repo.go", got)
		}
	})

	t.Run("carries the generated marker downstream tooling keys off", func(t *testing.T) {
		t.Parallel()
		src.AssertGeneratedHeader(t)
	})

	t.Run("is gofmt-canonical", func(t *testing.T) {
		t.Parallel()
		// Not a template regression guard so much as a guard on every
		// consumer's next `gofmt -l` run, which blames them for it.
		src.AssertFormatted(t)
	})

	t.Run("documents every exported declaration", func(t *testing.T) {
		t.Parallel()
		src.AssertDocumented(t)
	})

	t.Run("imports nothing but context", func(t *testing.T) {
		t.Parallel()
		// A generator's import set is part of its API: a new one is a
		// breaking change for every consumer whose module does not
		// already require it, and it is invisible in the generator's own
		// tests where the import always resolves.
		src.AssertImportsOnly(t, "context")
	})

	t.Run("declares the interface and its default implementation", func(t *testing.T) {
		t.Parallel()
		src.AssertType(t, "UserRepository").
			AssertDoc(t, "UserRepository stores and retrieves User values.")
		src.AssertType(t, "UserRepo").
			AssertDoc(t, "default in-memory implementation of UserRepository")
	})

	t.Run("the implementation precedes the interface it names", func(t *testing.T) {
		t.Parallel()
		// Pinned the way it renders, not the way emitOne writes it.
		// emitOne builds the interface first, but [sdk.EmitPackage] buckets
		// declarations by kind and the backend walks the buckets, so
		// emission order does not survive into the file. Worth an
		// assertion because the struct's doc comment forward-references
		// a type declared thirty lines below it — a reader's first
		// question, and a diff every consumer would see if the bucket
		// order ever changed.
		src.AssertOrder(t, "UserRepo", "UserRepository")
	})

	for _, m := range crudSet {
		t.Run("emits "+m.name+" on a pointer receiver with the contract signature", func(t *testing.T) {
			t.Parallel()
			src.AssertMethod(t, "UserRepo", m.name).
				Signature(t, m.sig).
				AssertPointerReceiver(t, true).
				AssertDoc(t, "The body is empty until a cross-cutting plugin")
		})
	}

	t.Run("leaves every body empty for the weaver slots", func(t *testing.T) {
		t.Parallel()
		// The emptiness is the feature — downstream weavers fill the
		// method's prebody / postbody slots — so it is asserted rather
		// than tolerated. Scoped per method: a file-wide substring count
		// for "return nil" cannot say which method carries it.
		src.InMethod(t, "UserRepo", "Get").AssertContains(t, "return nil, nil")
		src.InMethod(t, "UserRepo", "List").AssertContains(t, "return nil, nil")
		src.InMethod(t, "UserRepo", "Save").AssertContains(t, "return nil")
		src.InMethod(t, "UserRepo", "Delete").AssertContains(t, "return nil")
	})

	t.Run("keeps its exported surface", func(t *testing.T) {
		t.Parallel()
		// The one assertion that covers the *interface's* method set:
		// golangtest has no per-interface-method query, and every check
		// above reaches the struct's methods only. A diff here is the
		// list of changes a consumer would have to react to.
		golangtest.AssertAPIGolden(t, src, "testdata/user_repo.api.golden")
	})
}

// TestRender_CamelNaming pins the one option that changes what a
// consumer can see: `naming=Camel` makes every emitted identifier
// unexported, so the whole surface leaves the package.
//
// The renaming is applied in four independent places — interface,
// struct, interface method, struct method — and a rename that reached
// three of them would still compile. Only the satisfaction assertion
// says the four agree.
func TestRender_CamelNaming(t *testing.T) {
	t.Parallel()

	gen := render(t, map[string]string{"naming": repogen.NamingCamel}, "User")
	src := gen.Primary(t)

	t.Run("the unexported surface builds and stays self-consistent", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
		gen.AssertSatisfies(t, "userRepo", "userRepository")
	})

	t.Run("exports nothing", func(t *testing.T) {
		t.Parallel()
		src.AssertNoType(t, "UserRepository")
		src.AssertNoType(t, "UserRepo")
		src.AssertType(t, "userRepository")
		src.AssertType(t, "userRepo")
	})

	t.Run("lower-cases the method set too", func(t *testing.T) {
		t.Parallel()
		src.AssertNoMethod(t, "userRepo", "Get")
		src.AssertMethod(t, "userRepo", "get").
			Signature(t, "(ctx context.Context, id string) (*User, error)")
	})
}

// TestRender_SuffixOptions pins the two suffix options against the
// emitted identifiers, since a suffix that reached the interface but
// not the struct's doc comment is invisible to a compile.
func TestRender_SuffixOptions(t *testing.T) {
	t.Parallel()

	gen := render(t, map[string]string{
		"interface_suffix": "Storage",
		"struct_suffix":    "Storer",
	}, "User")
	src := gen.Primary(t)

	t.Run("the renamed pair builds and stays self-consistent", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
		gen.AssertSatisfies(t, "UserStorer", "UserStorage")
	})

	t.Run("both suffixes reach the emitted identifiers", func(t *testing.T) {
		t.Parallel()
		src.AssertType(t, "UserStorage")
		src.AssertType(t, "UserStorer").AssertDoc(t, "implementation of UserStorage")
		src.AssertNoType(t, "UserRepository")
		src.AssertNoType(t, "UserRepo")
	})
}

// TestRender_OneFilePerSourceStruct covers the grouping half of
// Generate: three annotated structs in one package produce three
// files, each named for its own origin, and all three have to build
// together — the case where a shared package clause or a duplicated
// helper would collide.
func TestRender_OneFilePerSourceStruct(t *testing.T) {
	t.Parallel()

	gen := render(t, nil, "User", "Order", "Invoice")

	t.Run("all three files build as one package", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
	})

	t.Run("each source struct gets its own file", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"User", "Order", "Invoice"} {
			file := gen.File(t, "test/"+strings.ToLower(name)+"_repo.go")
			file.AssertType(t, name+"Repository")
			file.AssertMethod(t, name+"Repo", "Save").
				Signature(t, "(ctx context.Context, value *"+name+") error")
		}
	})
}

// render runs repogen over a synthetic package holding one
// `+gen:repo` struct per name and adopts everything the backend
// rendered.
//
// The hand-written package the output references is attached here
// rather than by each caller: every generated method names the source
// struct, so nothing repogen emits compiles without it, and a fixture
// that forgot would fail on a missing type rather than on the thing it
// meant to assert.
func render(t *testing.T, opts map[string]string, names ...string) *golangtest.Generated {
	t.Helper()

	fixture := gofixture.New()
	for _, name := range names {
		fixture.Struct(name, func(s *gofixture.StructBuilder) {
			s.Docs(name + " is a source struct carrying +gen:repo.")
			s.Directive(gofixture.Directive(repogen.DirectiveName))
			s.Field("ID", gofixture.Named("string"), nil)
		})
	}

	build := golangtest.Driver(t, backendgolang.New(), fixture.PackageNode(), repogen.New())
	if len(opts) > 0 {
		build = build.WithPluginOptions(repogen.Name, opts)
	}

	// The source package is projected from the same builder that drove
	// the run rather than assembled beside it: every generated method
	// names the source struct, so nothing repogen emits compiles
	// without it, and a second spelling of the same structs would go
	// stale the first time a field changed here.
	return golangtest.Rendered(t, build.Build().Run("./...")).
		WithSource(golangtest.GoFile(fixture.GoSource()))
}

// contractSource spells the CRUD contract the way a consumer would
// write it, so the generated pair can be checked against something
// other than itself.
//
// Two assertions hang off it. [golangtest.Generated.AssertSatisfies]
// points the emitted struct at it, and the `var _` below points the
// emitted *interface* at it — the only check in this file that reaches
// the interface's method set, because golangtest exposes no
// per-interface-method query and a compile is what is left.
func contractSource() golangtest.File {
	return golangtest.File{
		Path: "test/contract.go",
		Src: []byte(`package test

import "context"

// UserRepositoryContract is the repository surface repogen promises,
// written by hand so a drift that moves the generated interface and
// the generated struct together is still a failure.
type UserRepositoryContract interface {
	Get(ctx context.Context, id string) (*User, error)
	List(ctx context.Context) ([]*User, error)
	Save(ctx context.Context, value *User) error
	Delete(ctx context.Context, id string) error
}

// The generated interface has to carry the contract too: a consumer
// stores repogen's interface in a field and calls it, so a method
// missing from the interface is a break even when the struct has it.
var _ UserRepositoryContract = (UserRepository)(nil)
`),
	}
}
