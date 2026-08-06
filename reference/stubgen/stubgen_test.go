// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package stubgen_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/reference/stubgen"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites. The
// framework checks pin the static contract — stable Name, role
// implementation, deterministic Outputs, well-formed output shape,
// unique directive names; the generator suite pins determinism,
// frozen-source and diagnostic discipline across fixtures.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, stubgen.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(t, stubgen.New(), []plugintest.GeneratorFixture{
			{
				Name: "package with no annotated interface",
				BuildStore: func(t *testing.T) *store.Store {
					t.Helper()
					return storefixture.New().Interface("Plain", nil).Build()
				},
			},
			{
				Name: "annotated interface with one method",
				BuildStore: func(t *testing.T) *store.Store {
					t.Helper()
					return storefixture.New().
						Interface("Store", func(i *storefixture.InterfaceBuilder) {
							i.Directive(storefixture.Directive("stub"))
							i.Method("Close", func(m *storefixture.MethodBuilder) {
								m.Return(storefixture.Named("error"))
							})
						}).
						Build()
				},
			},
		})
	})

	t.Run("options round-trip", func(t *testing.T) {
		t.Parallel()
		plugintest.RunOptionsSuite(t, stubgen.New(), plugintest.OptionsFixture{
			Valid:      map[string]string{"suffix": "Double"},
			UnknownKey: "no_such_field",
		})
	})
}

func TestOutputs_TwoEntriesPrimaryFirst(t *testing.T) {
	t.Parallel()

	t.Run("golang declares the primary and tagged companion", func(t *testing.T) {
		t.Parallel()
		got := stubgen.New().Outputs("golang")
		if len(got) != 2 {
			t.Fatalf("Outputs(golang) = %d entries, want 2", len(got))
		}
		// The framework reserves the empty tag for a plugin's primary
		// output and requires it at index 0 when present.
		if got[0].Tag != "" || got[0].Suffix != stubgen.GoPrimarySuffix {
			t.Errorf("primary output = %+v, want untagged %q", got[0], stubgen.GoPrimarySuffix)
		}
		if got[1].Tag != stubgen.GoTestOutputTag || got[1].Suffix != stubgen.GoTestSuffix {
			t.Errorf("companion output = %+v, want tag %q", got[1], stubgen.GoTestOutputTag)
		}
	})

	t.Run("an unknown language declares no routable output", func(t *testing.T) {
		t.Parallel()
		if got := stubgen.New().Outputs("rust"); got != nil {
			t.Fatalf("Outputs(rust) = %+v, want nil", got)
		}
	})
}

func TestTemplates_ShippedForGoOnly(t *testing.T) {
	t.Parallel()

	t.Run("go ships a template tree", func(t *testing.T) {
		t.Parallel()
		fsys, ok := stubgen.New().Templates("golang")
		if !ok || fsys == nil {
			t.Fatalf("Templates(golang) = %v, %v; want a tree", fsys, ok)
		}
	})

	t.Run("an unknown language ships none", func(t *testing.T) {
		t.Parallel()
		if _, ok := stubgen.New().Templates("rust"); ok {
			t.Fatalf("Templates(rust) reported a tree")
		}
	})

	// The plugin contributes no funcmap entries for any language.
	//
	// It used to return the shared lang/golang helpers here, and this
	// subtest asserted that as the contract. Both were wrong: the
	// backend already merges that map into its overrideable bucket, so
	// returning it re-registers existing names — a Build-time
	// ErrTemplateFuncCollision. The practical effect was that stubgen
	// could not appear in a pipeline beside any other plugin that
	// shipped templates and did the same, which is exactly what
	// happened when the middlewaregen composition set arrived.
	t.Run("funcmap contributes nothing, for any language", func(t *testing.T) {
		t.Parallel()
		for _, lang := range []string{"golang", "rust", ""} {
			if got := stubgen.New().TemplateFuncs(lang); got != nil {
				t.Errorf("TemplateFuncs(%q) = %v, want nil; the shared helpers come from "+
					"the backend and re-registering them collides at Build", lang, got)
			}
		}
		if got := stubgen.New().TemplateOverrides("golang"); got != nil {
			t.Fatalf("TemplateOverrides = %v; the plugin replaces no canonical entry", got)
		}
	})
}

func TestGenerate_QueuesBothOutputs(t *testing.T) {
	t.Parallel()

	t.Run("one stub and one tagged test per annotated interface", func(t *testing.T) {
		t.Parallel()
		pending := generate(t, stubgen.New(), storeFixture(t))
		if len(pending) != 2 {
			t.Fatalf("queued %d contributions, want 2", len(pending))
		}

		stub, tests := split(t, pending)
		if stub.Kind() != stubgen.KindStub {
			t.Errorf("primary kind = %q, want %q", stub.Kind(), stubgen.KindStub)
		}
		if tests.Kind() != stubgen.KindStubTests {
			t.Errorf("companion kind = %q, want %q", tests.Kind(), stubgen.KindStubTests)
		}
		// The tag is what makes the companion independently routable.
		if got := tests.OutputTag(); got != stubgen.GoTestOutputTag {
			t.Errorf("companion OutputTag = %q, want %q", got, stubgen.GoTestOutputTag)
		}
		if got := stub.OutputTag(); got != "" {
			t.Errorf("primary OutputTag = %q, want empty", got)
		}
	})

	t.Run("both land in the same file slot", func(t *testing.T) {
		t.Parallel()
		for _, p := range generate(t, stubgen.New(), storeFixture(t)) {
			if p.SlotName != stubgen.SlotName {
				t.Fatalf("contribution slot = %q, want %q", p.SlotName, stubgen.SlotName)
			}
		}
	})

	t.Run("an interface without the directive is skipped", func(t *testing.T) {
		t.Parallel()
		s := storefixture.New().
			Interface("Plain", func(i *storefixture.InterfaceBuilder) {
				i.Method("Do", nil)
			}).
			Build()
		if got := generate(t, stubgen.New(), s); len(got) != 0 {
			t.Fatalf("unannotated interface queued %d contributions", len(got))
		}
	})
}

func TestGenerate_AnnotatedButEmptyIsReported(t *testing.T) {
	t.Parallel()

	// A double with nothing to stand in for is a mistake. Emitting an
	// empty struct would compile and hide it, so the plugin reports
	// and skips instead.
	s := storefixture.New().
		Interface("Empty", func(i *storefixture.InterfaceBuilder) {
			i.Directive(storefixture.Directive("stub"))
		}).
		Build()

	d := diag.New()
	if err := stubgen.New().Generate(&plugin.GeneratorContext{
		Store: s, Reader: store.NewReader(s), Diag: d,
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !d.HasErrors() {
		t.Fatalf("annotated interface with no methods should be reported")
	}
	if got := len(s.Emit().PendingOriginSlots()); got != 0 {
		t.Fatalf("queued %d contributions for an empty interface, want 0", got)
	}
}

func TestGenerate_SuffixOption(t *testing.T) {
	t.Parallel()

	p := stubgen.New()
	if err := p.SetOptions(opt.New(p.OptionsSchema(), map[string]string{
		"suffix": "Double",
	})); err != nil {
		t.Fatalf("SetOptions: %v", err)
	}
	stub, _ := split(t, generate(t, p, storeFixture(t)))
	if got := stub.TypeName; got != "StoreDouble" {
		t.Fatalf("stub type = %q, want StoreDouble", got)
	}
}

// storeFixture returns a store holding one `+gen:stub` interface with
// a named-return method, an unnamed-return method, and a void method
// — the three shapes the signature rules discriminate on.
func storeFixture(t *testing.T) *store.Store {
	t.Helper()
	return storefixture.New().
		Interface("Store", func(i *storefixture.InterfaceBuilder) {
			i.Directive(storefixture.Directive("stub"))
			i.Method("Get", func(m *storefixture.MethodBuilder) {
				m.Param("ctx", storefixture.PkgNamed("context", "Context"))
				m.Param("id", storefixture.Named("string"))
				m.NamedReturn("item", storefixture.Named("string"))
				m.NamedReturn("err", storefixture.Named("error"))
			})
			i.Method("List", func(m *storefixture.MethodBuilder) {
				m.Return(storefixture.Slice(storefixture.Named("string")))
				m.Return(storefixture.Named("error"))
			})
			i.Method("Close", nil)
		}).
		Build()
}

// generate drives p over s and returns the queued contributions.
func generate(t *testing.T, p *stubgen.Plugin, s *store.Store) []store.PendingOriginSlot {
	t.Helper()
	if err := p.Generate(&plugin.GeneratorContext{
		Store: s, Reader: store.NewReader(s), Diag: diag.New(),
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return s.Emit().PendingOriginSlots()
}

// split separates the queued contributions into the primary stub and
// the tagged companion, failing when either is absent.
func split(t *testing.T, pending []store.PendingOriginSlot) (*stubgen.Stub, *stubgen.Tests) {
	t.Helper()
	var (
		stub  *stubgen.Stub
		tests *stubgen.Tests
	)
	for _, p := range pending {
		switch v := p.Item.(type) {
		case *stubgen.Stub:
			stub = v
		case *stubgen.Tests:
			tests = v
		}
	}
	if stub == nil || tests == nil {
		t.Fatalf("expected one stub and one tests contribution; got %d", len(pending))
	}
	return stub, tests
}

// TestTests_SetOutputPackages pins the [emit.OutputAware] half of
// the plugin: the companion's reference to the stub follows wherever
// Layout routed the primary output, which is the one fact Generate
// cannot know.
func TestTests_SetOutputPackages(t *testing.T) {
	t.Parallel()

	t.Run("the stub reference follows the primary's routed package", func(t *testing.T) {
		t.Parallel()
		_, tests := split(t, generate(t, stubgen.New(), storeFixture(t)))
		tests.SetOutputPackages(map[string]string{"": "example.com/demo/testkit"})
		if got := tests.StubRef.Pkg; got != "example.com/demo/testkit" {
			t.Fatalf("StubRef.Pkg = %q, want the routed path", got)
		}
		if got := tests.StubRef.Name; got != "StoreStub" {
			t.Fatalf("StubRef.Name = %q, want StoreStub", got)
		}
	})

	t.Run("the interface reference is left alone", func(t *testing.T) {
		t.Parallel()
		// The source interface is hand-written and does not move
		// with the generator's output. Repointing it would break
		// exactly the case redirection is supposed to fix.
		_, tests := split(t, generate(t, stubgen.New(), storeFixture(t)))
		before := tests.IfaceRef.Pkg
		tests.SetOutputPackages(map[string]string{"": "example.com/demo/testkit"})
		if got := tests.IfaceRef.Pkg; got != before {
			t.Fatalf("IfaceRef.Pkg = %q, want it unchanged at %q", got, before)
		}
	})

	t.Run("an underivable path leaves the provisional reference in place", func(t *testing.T) {
		t.Parallel()
		// Centralised routing resolves a Target without an import
		// path. A wrong package is a compile error naming the
		// symbol; a bare name silently binds to whatever else is in
		// scope, so the provisional value is the safer residue.
		_, tests := split(t, generate(t, stubgen.New(), storeFixture(t)))
		before := tests.StubRef.Pkg
		tests.SetOutputPackages(map[string]string{"": ""})
		if got := tests.StubRef.Pkg; got != before {
			t.Fatalf("StubRef.Pkg = %q, want it unchanged at %q", got, before)
		}
	})
}
