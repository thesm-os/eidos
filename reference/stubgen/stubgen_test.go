// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package stubgen_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	langgo "go.thesmos.sh/eidos/lang/golang"
	backendgolang "go.thesmos.sh/eidos/lang/golang/backend"
	"go.thesmos.sh/eidos/lang/golang/golangtest"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/reference/stubgen"
	"go.thesmos.sh/eidos/sdk"
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
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return gofixture.New().Interface("Plain", nil).Build()
				},
			},
			{
				Name: "annotated interface with one method",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return gofixture.New().
						Interface("Store", func(i *gofixture.InterfaceBuilder) {
							i.Directive(gofixture.Directive("stub"))
							i.Method("Close", func(m *gofixture.MethodBuilder) {
								m.Return(gofixture.Named("error"))
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

	// The plugin publishes no helpers of its own.
	//
	// It once returned the shared lang/golang helpers here under their
	// bare names, which re-registered names the backend already held —
	// a Build-time ErrTemplateFuncCollision that kept stubgen out of
	// any pipeline beside another plugin doing the same. That was
	// answered for a while by giving each plugin its own copy under
	// its own prefix, so a template called a helper by a name no
	// declaration contained.
	//
	// The backend owns that vocabulary now, in one copy, and a plugin
	// publishes only helpers it wrote. stubgen wrote none.
	t.Run("the plugin publishes no helpers of its own", func(t *testing.T) {
		t.Parallel()
		if got := stubgen.New().TemplateFuncs("golang"); len(got) != 0 {
			t.Errorf("TemplateFuncs(golang) = %v, want none", got)
		}
	})

	t.Run("a non-Go language gets no funcmap", func(t *testing.T) {
		t.Parallel()
		for _, lang := range []string{"rust", ""} {
			if got := stubgen.New().TemplateFuncs(lang); got != nil {
				t.Errorf("TemplateFuncs(%q) = %v, want nil", lang, got)
			}
		}
	})

	// The two templates call only the backend's canonical renderers,
	// so there is nothing to specialise.
	t.Run("no canonical entry is replaced", func(t *testing.T) {
		t.Parallel()
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
		s := gofixture.New().
			Interface("Plain", func(i *gofixture.InterfaceBuilder) {
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
	s := gofixture.New().
		Interface("Empty", func(i *gofixture.InterfaceBuilder) {
			i.Directive(gofixture.Directive("stub"))
		}).
		Build()

	d := diag.New()
	if err := stubgen.New().Generate(&sdk.GeneratorContext{
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

// storeBuilder holds the one `+gen:stub` interface every fixture in
// this file works from, spanning the shapes the generator
// discriminates on: a named-return method taking a cross-package
// type, an anonymous multi-return, a variadic tail, a parameter that
// collides with the receiver identifier, unnamed parameters, and a
// void method.
//
// Shared between the unit tests, which take the [sdk.Store], and
// the render suite, which takes the [sdk.Package] — so a signature
// shape added here is exercised by both rather than by whichever
// fixture the author happened to edit. The Go the generated double
// is built against is projected from this same builder by
// [gofixture.Builder.GoSource], so a signature changed here
// cannot leave a hand-written support package behind.
func storeBuilder() *gofixture.Builder {
	return gofixture.New().
		Interface("Store", func(i *gofixture.InterfaceBuilder) {
			i.Directive(gofixture.Directive("stub"))
			i.Method("Get", func(m *gofixture.MethodBuilder) {
				m.Param("ctx", gofixture.PkgNamed("context", "Context"))
				m.Param("id", gofixture.Named("string"))
				m.NamedReturn("item", gofixture.Named("string"))
				m.NamedReturn("err", gofixture.Named("error"))
			})
			i.Method("List", func(m *gofixture.MethodBuilder) {
				m.Return(gofixture.Slice(gofixture.Named("string")))
				m.Return(gofixture.Named("error"))
			})
			// A variadic tail: dropping the marker produces a double
			// that compiles and satisfies nothing.
			i.Method("Put", func(m *gofixture.MethodBuilder) {
				m.Param("id", gofixture.Named("string"))
				m.Variadic("opts", gofixture.Named("string"))
				m.Return(gofixture.Named("error"))
			})
			// A parameter spelled like the receiver identifier the
			// stub type's name would otherwise yield.
			i.Method("Recv", func(m *gofixture.MethodBuilder) {
				m.Param("s", gofixture.Named("string"))
				m.Return(gofixture.Named("error"))
			})
			// Unnamed parameters, which the source is free to leave
			// out and the generated body cannot.
			i.Method("Anon", func(m *gofixture.MethodBuilder) {
				m.Param("", gofixture.Named("int"))
				m.Param("", gofixture.Named("string"))
			})
			i.Method("Close", nil)
		})
}

// storeFixture returns a store holding the [storeBuilder] interface.
func storeFixture(t *testing.T) *sdk.Store {
	t.Helper()
	return storeBuilder().Build()
}

// generate drives p over s and returns the queued contributions.
func generate(t *testing.T, p *stubgen.Plugin, s *sdk.Store) []sdk.PendingOriginSlot {
	t.Helper()
	if err := p.Generate(&sdk.GeneratorContext{
		Store: s, Reader: store.NewReader(s), Diag: diag.New(),
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return s.Emit().PendingOriginSlots()
}

// split separates the queued contributions into the primary stub and
// the tagged companion, failing when either is absent.
func split(t *testing.T, pending []sdk.PendingOriginSlot) (*stubgen.Stub, *stubgen.Tests) {
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

// TestTests_SetOutputPackages pins the [sdk.OutputPackageSetter] half of
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

// TestGenerate_EmbeddedMethodsAreDoubled pins the resolution a stub
// depends on to satisfy the interface it doubles.
//
// Reading the interface's declared methods alone produced a stub
// short whatever the interface embedded — which compiles as a
// standalone type and fails only where a consumer assigns it to the
// interface, reported against the generated file. An interface
// composed purely of embeds was worse: it declares nothing of its
// own, so it was rejected outright as having no methods.
func TestGenerate_EmbeddedMethodsAreDoubled(t *testing.T) {
	t.Parallel()

	// embedding builds Reader (declaring Read) plus a stub-annotated
	// interface embedding it.
	embedding := func(t *testing.T, declared ...string) *sdk.Store {
		t.Helper()
		b := gofixture.New().
			Interface("Reader", func(i *gofixture.InterfaceBuilder) {
				i.Method("Read", nil)
			})
		b = b.Interface("ReadCloser", func(i *gofixture.InterfaceBuilder) {
			i.Directive(gofixture.Directive("stub"))
			for _, m := range declared {
				i.Method(m, nil)
			}
			i.Embed(gofixture.PkgNamed("example.com/test", "Reader"))
		})
		return b.Build()
	}

	t.Run("an embedded method is doubled alongside a declared one", func(t *testing.T) {
		t.Parallel()
		got := generate(t, stubgen.New(), embedding(t, "Close"))
		if len(got) == 0 {
			t.Fatalf("annotated interface queued no contribution")
		}
		stub, _ := split(t, got)
		names := make([]string, 0, len(stub.Methods))
		for _, m := range stub.Methods {
			names = append(names, m.Name)
		}
		for _, want := range []string{"Read", "Close"} {
			if !slices.Contains(names, want) {
				t.Errorf("stub methods %v omit %q; a double missing it does not satisfy the interface", names, want)
			}
		}
	})

	t.Run("an interface composed purely of embeds is doubled", func(t *testing.T) {
		t.Parallel()
		got := generate(t, stubgen.New(), embedding(t))
		if len(got) == 0 {
			t.Fatalf("a purely-embedding interface queued no contribution")
		}
	})
}

// renderStore drives one full pipeline over [storeBuilder] and adopts
// everything it produced, with the interface it describes projected
// alongside so the output can be built the way a consumer will build
// it.
//
// The support package is projected rather than written out because
// none of the generated output compiles without it — the primary
// declares methods against these signatures and the companion asserts
// satisfaction of this exact interface — and a hand-written copy
// would be bound to the fixture only by review. Add a method to
// [storeBuilder] and the stale copy would fail as a compile error
// inside a throwaway module, naming code nobody wrote.
//
// One run per fixture rather than one per assertion: [golangtest]
// caches the module it assembles, and every toolchain assertion
// shells out to `go`.
func renderStore(t *testing.T) *golangtest.Generated {
	t.Helper()
	return golangtest.Render(t, backendgolang.New(), storeBuilder().PackageNode(), stubgen.New()).
		WithSource(golangtest.GoFile(storeBuilder().GoSource()))
}

// TestRendered_ToolchainAcceptsTheDouble is the assertion every
// structural check in this file is a proxy for.
//
// Each of these caught something the plugin's own tests could not
// see. `go build` alone did not: it skips `_test.go`, and the primary
// output compiles as a standalone type no matter what its method set
// looks like. Vetting the companion is what surfaced a variadic
// marker dropped from the generated signature — a double declaring
// `Put(string, string)` against an interface wanting
// `Put(string, ...string)` — and running the generated suite is what
// surfaced a delegate closure whose parameter shadowed the counter
// the suite asserts on.
func TestRendered_ToolchainAcceptsTheDouble(t *testing.T) {
	t.Parallel()

	gen := renderStore(t)

	// Deliberately one subtest and deliberately serial. A Generated
	// caches its assembled module under the testing.TB that first
	// built it, so splitting these across subtests would leave the
	// later ones pointed at a TempDir the earlier one had removed.
	gen.AssertCompiles(t)
	gen.AssertVets(t)
	gen.AssertTestsPass(t)
	gen.AssertSatisfies(t, "StoreStub", "Store")
}

// TestRendered_PrimaryDeclaresTheDouble pins the shape of the
// generated stub: the recorded-call structs, the func and calls
// fields, and the method signatures that have to mirror the source's.
//
// Structural rather than substring throughout — a field spelled as a
// substring carries whatever column padding gofmt applied across its
// neighbours, so adding an unrelated method to the interface would
// break an assertion about this one.
func TestRendered_PrimaryDeclaresTheDouble(t *testing.T) {
	t.Parallel()

	src := renderStore(t).Primary(t)

	t.Run("the file is machine-written, formatted and documented", func(t *testing.T) {
		t.Parallel()
		src.AssertGeneratedHeader(t).
			AssertFormatted(t).
			AssertDocumented(t).
			AssertPackage(t, "test")
	})

	t.Run("only the packages the signatures name are imported", func(t *testing.T) {
		t.Parallel()
		// A generator's import set is part of its API: emitting a new
		// one is a breaking change for every consumer whose module
		// does not already require it, and invisible here, where it
		// always resolves.
		src.AssertImportsOnly(t, "context")
	})

	t.Run("every method has a func field and a recorded-call slice", func(t *testing.T) {
		t.Parallel()
		src.AssertField(t, "StoreStub", "GetFunc", "func(context.Context, string) (string, error)").
			AssertField(t, "StoreStub", "ListFunc", "func() ([]string, error)").
			AssertField(t, "StoreStub", "CloseFunc", "func()").
			AssertField(t, "StoreStub", "GetCalls", "[]StoreGetCall").
			AssertField(t, "StoreStub", "CloseCalls", "[]StoreCloseCall")
	})

	t.Run("recorded-call fields carry the source's declared names", func(t *testing.T) {
		t.Parallel()
		// The source's return names are the documentation a
		// recorded-call struct exists to preserve; unnamed slots fall
		// back to positional.
		src.AssertField(t, "StoreGetCall", "Ctx", "context.Context").
			AssertField(t, "StoreGetCall", "ID", "string").
			AssertField(t, "StoreGetCall", "Item", "string").
			AssertField(t, "StoreGetCall", "Err", "error").
			AssertField(t, "StoreListCall", "Result", "[]string").
			AssertField(t, "StoreListCall", "Err", "error")
	})

	t.Run("the generated methods mirror the source signatures", func(t *testing.T) {
		t.Parallel()
		src.AssertMethod(t, "StoreStub", "Get").
			Signature(t, "(ctx context.Context, id string) (item string, err error)").
			AssertPointerReceiver(t, true)
		src.AssertMethod(t, "StoreStub", "List").Signature(t, "() ([]string, error)")
		src.AssertMethod(t, "StoreStub", "Close").Signature(t, "()")
		// A source that named nothing still needs identifiers in the
		// generated body, which fall back to positional.
		src.AssertMethod(t, "StoreStub", "Anon").Signature(t, "(arg0 int, arg1 string)")
	})

	t.Run("a variadic tail survives into every position it appears in", func(t *testing.T) {
		t.Parallel()
		// The four spellings disagree, and each one that drops the
		// marker produces something that compiles: the declaration
		// wants `...string`, the func field's type wants it too, the
		// delegate call wants the spread, and the recorded field
		// wants the slice the parameter actually is inside the body.
		src.AssertMethod(t, "StoreStub", "Put").Signature(t, "(id string, opts ...string) error")
		src.AssertField(t, "StoreStub", "PutFunc", "func(string, ...string) error")
		src.AssertField(t, "StorePutCall", "Opts", "[]string")
		src.InMethod(t, "StoreStub", "Put").AssertContains(t, "opts...")
	})

	t.Run("a parameter named after the receiver moves the receiver", func(t *testing.T) {
		t.Parallel()
		// The source names the parameter and the generator names the
		// receiver, so the receiver is what gives way. Rendering both
		// as `s` made every `s.<Field>` in the body resolve to the
		// parameter, which does not compile.
		src.AssertMethod(t, "StoreStub", "Recv").Signature(t, "(s string) error")
		src.InMethod(t, "StoreStub", "Recv").
			AssertNotContains(t, "s.RecvFunc(").
			AssertContains(t, ".RecvFunc(s)")
	})

	t.Run("a recorded-call struct precedes the double that holds it", func(t *testing.T) {
		t.Parallel()
		// Slot ordering is otherwise invisible: a struct rendered
		// after the type whose field names it still compiles.
		src.AssertOrder(t, "StoreGetCall", "StoreStub")
	})

	t.Run("the exported surface is what consumers were promised", func(t *testing.T) {
		t.Parallel()
		// The review surface. Every line of a diff here is one a
		// consumer of the generated double would have to react to.
		golangtest.AssertAPIGolden(t, src, filepath.Join("testdata", "store_stub.api"))
	})
}

// TestRendered_CompanionProvesTheDouble pins the generated test
// suite, whose contract is what it asserts rather than what it says.
//
// [Generated.AssertTestsPass] is the half that proves the suite
// passes; these are the half that proves it is there at all — a
// generator that silently stopped emitting a per-method check would
// leave a suite that passes trivially.
func TestRendered_CompanionProvesTheDouble(t *testing.T) {
	t.Parallel()

	src := renderStore(t).Suffixed(t, stubgen.GoTestSuffix)

	t.Run("the suite lands in the external test package", func(t *testing.T) {
		t.Parallel()
		// The framework keys the shift off the filename. Landing in
		// the source package instead would let the suite reach
		// private state and stop proving the double works from
		// outside, which is the only place a consumer holds it.
		src.AssertPackage(t, "test_test").
			AssertGeneratedHeader(t).
			AssertFormatted(t).
			AssertImportsOnly(t, "context", "testing", "example.com/test")
	})

	t.Run("one recording check per interface method", func(t *testing.T) {
		t.Parallel()
		want := []string{
			"TestStoreStubRecordsGet",
			"TestStoreStubRecordsList",
			"TestStoreStubRecordsPut",
			"TestStoreStubRecordsRecv",
			"TestStoreStubRecordsAnon",
			"TestStoreStubRecordsClose",
		}
		got := src.TestFuncs()
		for _, name := range want {
			if !slices.Contains(got, name) {
				t.Errorf("companion declares no %s; it declares %v", name, got)
			}
		}
		if len(got) != len(want) {
			t.Errorf("companion declares %d tests, want %d: %v", len(got), len(want), got)
		}
	})

	t.Run("every generated test goes parallel", func(t *testing.T) {
		t.Parallel()
		// This suite runs in every consumer's CI, once per annotated
		// interface. A serial one taxes each of them for the life of
		// the generator and nobody will look at generated code for
		// the cause.
		for _, name := range src.TestFuncs() {
			src.AssertParallel(t, name)
		}
	})

	t.Run("the satisfaction proof names both types qualified", func(t *testing.T) {
		t.Parallel()
		// The companion is always the external test package of the
		// primary, so neither type is ever in scope unqualified.
		// A bare name would bind to whatever else was in scope.
		body := string(src.Bytes())
		if want := "var _ test.Store = (*test.StoreStub)(nil)"; !strings.Contains(body, want) {
			t.Errorf("companion does not carry %q\n%s", want, body)
		}
	})

	t.Run("a delegate closure declares its parameters blank", func(t *testing.T) {
		t.Parallel()
		// The closure ignores its arguments, and the source's own
		// identifiers would otherwise land in a scope holding the
		// counter the suite asserts on: an interface method taking a
		// parameter called `called` produced a suite that incremented
		// the parameter and failed at run time.
		src.InFunc(t, "TestStoreStubRecordsPut").
			AssertContains(t, "func(_ string, _ ...string) error")
	})
}

// A generic constraint carries terms constraining its type set where
// an ordinary interface carries embeds. Walking one asks the resolver
// for `int`, misses, and reports an embed the run did not load — so
// the author is told to widen a run for a declaration that has no
// method set to double.
func TestGenerate_DeclinesAGenericConstraint(t *testing.T) {
	t.Parallel()

	constraintStore := func(t *testing.T) *sdk.Store {
		t.Helper()
		b := gofixture.New().
			Package("cfg", "example.com/cfg").
			Interface("Numeric", func(i *gofixture.InterfaceBuilder) {
				i.Pos(position.At("cfg/types.go", 1, 1))
				i.Directive(gofixture.Directive(stubgen.DirectiveName))
				i.Embed(gofixture.Named("int"))
				i.Embed(gofixture.Named("int64"))
				// The frontend's stamp, which is the only thing that
				// separates this from `interface{ error }` — one shape
				// in the model, and only one of them is a type set.
				langgo.MetaIsConstraintInterface.Set(
					i.Node().EnsureMeta(), true, "golang",
				)
			})
		return b.Build()
	}

	t.Run("reports the constraint rather than its terms", func(t *testing.T) {
		t.Parallel()
		s := constraintStore(t)
		sink := diag.New()
		if err := stubgen.New().Generate(&plugin.GeneratorContext{
			Store: s, Reader: store.NewReader(s), Diag: sink,
		}); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		got := sink.Diagnostics()
		if len(got) != 1 {
			t.Fatalf("diagnostics = %d, want one", len(got))
		}
		if !strings.Contains(got[0].Message, "generic constraint") {
			t.Fatalf("message = %q, want it to name the constraint", got[0].Message)
		}
		// The failure this replaces: `embeds "int", which not loaded by
		// this run`, sending the author after a package they do not need.
		if strings.Contains(got[0].Message, "not loaded") {
			t.Fatalf("message = %q, want no embed complaint", got[0].Message)
		}
	})

	t.Run("emits nothing for it", func(t *testing.T) {
		t.Parallel()
		s := constraintStore(t)
		if err := stubgen.New().Generate(&plugin.GeneratorContext{
			Store: s, Reader: store.NewReader(s), Diag: diag.New(),
		}); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got := s.Emit().PendingOriginSlots(); len(got) != 0 {
			t.Fatalf("queued %d value(s) for a constraint", len(got))
		}
	})
}
