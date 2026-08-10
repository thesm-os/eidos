// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package registrygen_test

import (
	"testing"

	backendgolang "go.thesmos.sh/eidos/backend/golang"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/golangtest"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/reference/registrygen"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites against
// registrygen. Cross-cutting plugins (Generator + emits a
// per-package init-time registration file) verify both the
// universal framework contracts and the per-role determinism /
// frozen-source / diagnostic-discipline contracts.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, registrygen.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(
			t,
			registrygen.New(),
			[]plugintest.GeneratorFixture{
				{
					Name: "empty package",
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return storefixture.New().Build()
					},
				},
				{
					Name: "package with a struct",
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return storefixture.New().
							Struct("Plain", nil).
							Build()
					},
				},
			},
		)
	})

	t.Run("options round-trip", func(t *testing.T) {
		t.Parallel()
		plugintest.RunOptionsSuite(t, registrygen.New(), plugintest.OptionsFixture{
			Valid: map[string]string{
				"register_package": "log",
				"register_func":    "Print",
			},
			UnknownKey: "no_such_field",
		})
	})
}

func TestGenerate_AppendsRegistration(t *testing.T) {
	t.Parallel()

	// pending drives Generate over a fixture and returns the
	// origin-anchored contributions it queued for the Layout phase.
	pending := func(t *testing.T, s *sdk.Store) []sdk.PendingOriginSlot {
		t.Helper()
		if err := registrygen.New().Generate(&sdk.GeneratorContext{
			Store: s, Reader: store.NewReader(s), Diag: diag.New(),
		}); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		return s.Emit().PendingOriginSlots()
	}

	t.Run("one registration per annotated struct", func(t *testing.T) {
		t.Parallel()
		s := storefixture.New().
			Struct("User", func(b *storefixture.StructBuilder) {
				b.Directive(storefixture.Directive("register"))
			}).
			Struct("Plain", nil).
			Build()

		got := pending(t, s)
		if len(got) != 1 {
			t.Fatalf("only the +gen:register struct should register, got %d contributions", len(got))
		}
		if got[0].SlotName != registrygen.SlotName {
			t.Errorf("contribution slot = %q, want %q", got[0].SlotName, registrygen.SlotName)
		}
		reg, ok := got[0].Item.(*registrygen.Registration)
		if !ok {
			t.Fatalf("contribution item is %T, want *registrygen.Registration", got[0].Item)
		}
		if reg.Name != "User" {
			t.Errorf("registration name = %q, want User", reg.Name)
		}
		// The plugin-defined emit kind is what lets the backend pick
		// this node's template.
		if reg.Kind() != registrygen.Kind {
			t.Errorf("registration kind = %q, want %q", reg.Kind(), registrygen.Kind)
		}
	})

	t.Run("a struct without the directive contributes nothing", func(t *testing.T) {
		t.Parallel()
		s := storefixture.New().Struct("Plain", nil).Build()
		if got := pending(t, s); len(got) != 0 {
			t.Errorf("unannotated struct should contribute nothing, got %d", len(got))
		}
	})
}

// blogBuilder is the annotated source graph every render fixture
// drives.
//
// Two `+gen:register` structs share `blog.go` so the rendered output
// has to collect both into one `func init()`; a third sits in
// `comment.go` so the alongside-source routing has a second origin
// basename to compose a filename from; and one unannotated struct is
// present so "registers everything it sees" and "registers what was
// asked for" are distinguishable outcomes.
func blogBuilder() *storefixture.Builder {
	register := func(s *storefixture.StructBuilder, file string, line int) {
		s.Pos(sdk.Pos{File: file, Line: line})
		s.Directive(storefixture.Directive(sdk.DirectiveName(registrygen.DirectiveName)))
	}
	return storefixture.New().
		Struct("Article", func(s *storefixture.StructBuilder) { register(s, "blog.go", 3) }).
		Struct("Author", func(s *storefixture.StructBuilder) { register(s, "blog.go", 9) }).
		Struct("Plain", func(s *storefixture.StructBuilder) {
			s.Pos(sdk.Pos{File: "blog.go", Line: 15})
		}).
		Struct("Comment", func(s *storefixture.StructBuilder) { register(s, "comment.go", 3) })
}

// renderRegistry drives a full pipeline over [blogBuilder] and adopts
// what the backend rendered, with the source it registers projected
// alongside.
//
// The source package is load-bearing rather than decorative: every
// registration renders a composite literal of a source struct, so
// without those declarations the emitted file cannot be compiled and
// the toolchain assertions have nothing to assert against. Projected
// from the builder rather than written beside it, so a struct renamed
// in the fixture cannot leave a stale declaration behind — the
// generated literal would still name the old one and the failure
// would land in generated code.
//
// opts is the plugin's option map, empty for the documented defaults.
func renderRegistry(t *testing.T, opts map[string]string) *golangtest.Generated {
	t.Helper()
	fixture := blogBuilder()
	b := golangtest.Driver(t, backendgolang.New(), fixture.PackageNode(), registrygen.New())
	if len(opts) > 0 {
		b = b.WithPluginOptions(registrygen.Name, opts)
	}
	return golangtest.Rendered(t, b.Build().Run("./...")).
		WithSource(golangtest.GoFile(fixture.GoSource()))
}

// TestRendered_DefaultRegistry asserts on the Go registrygen emits
// under its documented defaults.
//
// The toolchain assertions are what make the rest of this file mean
// anything: the template renders a call, two arguments and an import,
// and a substring check on any of them passes just as well when the
// import is missing, the composite literal names a type that does not
// exist, or the call arity disagrees with the callee. Both run in
// parallel over the same fixture — see the cost note on
// [golangtest.Generated.AssertCompiles].
func TestRendered_DefaultRegistry(t *testing.T) {
	t.Parallel()

	gen := renderRegistry(t, nil)

	t.Run("the emitted registry compiles against the source it registers", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
	})

	t.Run("the emitted registry passes vet", func(t *testing.T) {
		t.Parallel()
		gen.AssertVets(t)
	})

	t.Run("one file per origin basename, named by the declared suffix", func(t *testing.T) {
		t.Parallel()
		files := gen.Files()
		got := make([]string, 0, len(files))
		for _, f := range files {
			got = append(got, f.Path)
		}
		want := []string{"blog" + registrygen.FilenameSuffix, "comment" + registrygen.FilenameSuffix}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("rendered files = %v, want %v", got, want)
		}
	})

	t.Run("the registry file is tooling-legible generated Go", func(t *testing.T) {
		t.Parallel()
		gen.File(t, "blog"+registrygen.FilenameSuffix).
			AssertPackage(t, "test").
			AssertGeneratedHeader(t).
			AssertFormatted(t)
	})

	t.Run("the register call's package is the file's only import", func(t *testing.T) {
		t.Parallel()
		// Pinned rather than merely present: an import the template
		// starts emitting is a new requirement on every consumer's
		// module, and nothing else in this suite would see it.
		gen.File(t, "blog"+registrygen.FilenameSuffix).
			AssertImportsOnly(t, registrygen.DefaultRegisterPackage)
	})

	t.Run("every annotated struct in one origin lands in one init block", func(t *testing.T) {
		t.Parallel()
		gen.File(t, "blog"+registrygen.FilenameSuffix).
			InFunc(t, "init").
			AssertCount(t, "log.Print(", 2).
			AssertContains(t, `log.Print("Article", Article{})`).
			AssertContains(t, `log.Print("Author", Author{})`)
	})

	t.Run("an unannotated struct registers nothing", func(t *testing.T) {
		t.Parallel()
		gen.File(t, "blog"+registrygen.FilenameSuffix).
			InFunc(t, "init").
			AssertNotContains(t, "Plain")
	})

	t.Run("a second origin basename gets its own registry file", func(t *testing.T) {
		t.Parallel()
		gen.File(t, "comment"+registrygen.FilenameSuffix).
			InFunc(t, "init").
			AssertCount(t, "log.Print(", 1).
			AssertContains(t, `log.Print("Comment", Comment{})`)
	})
}

// TestRendered_ConfiguredRegistry asserts that the configured registry
// surface reaches the rendered call.
//
// The fixture exists for the compile: [registrygen.Options.RegisterFunc]
// documents that the configured function must accept `(string, any)`
// or a variadic equivalent, and that claim is only ever tested by
// building the output against a real function. A substring assertion
// on `fmt.Println(` would pass against a call the consumer's compiler
// rejects.
func TestRendered_ConfiguredRegistry(t *testing.T) {
	t.Parallel()

	gen := renderRegistry(t, map[string]string{
		"register_package": "fmt",
		"register_func":    "Println",
	})

	t.Run("the configured registry call compiles", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
	})

	t.Run("the configured package replaces the default import", func(t *testing.T) {
		t.Parallel()
		gen.File(t, "blog"+registrygen.FilenameSuffix).
			AssertImportsOnly(t, "fmt").
			AssertNoImport(t, registrygen.DefaultRegisterPackage)
	})

	t.Run("the configured func is the rendered callee", func(t *testing.T) {
		t.Parallel()
		gen.File(t, "blog"+registrygen.FilenameSuffix).
			InFunc(t, "init").
			AssertContains(t, `fmt.Println("Article", Article{})`).
			AssertNotContains(t, "log.Print")
	})
}
