// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package debugweaver_test

import (
	"strings"
	"testing"
	"text/template"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	backendgolang "go.thesmos.sh/eidos/lang/golang/backend"
	"go.thesmos.sh/eidos/lang/golang/golangtest"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	"go.thesmos.sh/eidos/reference/debugweaver"
	"go.thesmos.sh/eidos/reference/repogen"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites against
// debugweaver. The plugin is a cross-cutting Prebody-slot weaver
// — it contributes statements to existing methods that other
// generators already produced, so the conformance suite's
// fixtures need only assert the contract holds for any source
// shape; the rendered output is exercised end-to-end through the
// pipeline conformance harnesses downstream.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, debugweaver.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(
			t,
			debugweaver.New(),
			[]plugintest.GeneratorFixture{
				{
					Name: "empty package",
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return gofixture.New().Build()
					},
				},
				{
					Name: "package with a struct",
					BuildStore: func(t *testing.T) *sdk.Store {
						t.Helper()
						return gofixture.New().
							Struct("User", nil).
							Build()
					},
				},
			},
		)
	})

	t.Run("options round-trip", func(t *testing.T) {
		t.Parallel()
		plugintest.RunOptionsSuite(t, debugweaver.New(), plugintest.OptionsFixture{
			Valid: map[string]string{
				"package": "log",
				"func":    "Printf",
				"format":  "debug: %s entered",
			},
			UnknownKey: "no_such_field",
		})
	})
}

func TestTrace_Kind(t *testing.T) {
	t.Parallel()

	t.Run("reports the kind the template is registered under", func(t *testing.T) {
		t.Parallel()
		if got := (&debugweaver.Trace{}).Kind(); got != debugweaver.Kind {
			t.Fatalf("Kind = %q, want %q", got, debugweaver.Kind)
		}
	})
}

func TestPlugin_Templates(t *testing.T) {
	t.Parallel()

	t.Run("ships templates for golang", func(t *testing.T) {
		t.Parallel()
		if _, ok := debugweaver.New().Templates("golang"); !ok {
			t.Fatalf("plugin should ship golang templates")
		}
	})

	t.Run("declines languages it does not target", func(t *testing.T) {
		t.Parallel()
		if _, ok := debugweaver.New().Templates("rust"); ok {
			t.Fatalf("plugin should not claim templates for rust")
		}
	})

	// The backend dispatches by looking up a template named for the
	// value's Kind. A define name that drifts from the declared Kind
	// costs nothing at build time and fails at render time, in
	// someone else's file — so the pairing is pinned here.
	t.Run("defines a template named for the declared Kind", func(t *testing.T) {
		t.Parallel()
		sub, ok := debugweaver.New().Templates("golang")
		if !ok {
			t.Fatalf("plugin should ship golang templates")
		}
		tmpl, err := template.New("probe").Funcs(probeFuncs()).ParseFS(sub, "*.tmpl")
		if err != nil {
			t.Fatalf("parse shipped templates: %v", err)
		}
		if tmpl.Lookup(string(debugweaver.Kind)) == nil {
			t.Fatalf("no template defines %q; the backend would find nothing to render",
				debugweaver.Kind)
		}
	})
}

// probeFuncs stubs the backend helpers the shipped template calls.
// text/template resolves function names at parse time, so parsing the
// template outside the backend needs the names to exist; the bodies
// are never executed here.
func probeFuncs() template.FuncMap {
	return template.FuncMap{
		"renderExpr": func(*sdk.Expr) (string, error) { return "", nil },
	}
}

// emitStoreWithMethod returns a store seeded with one emit struct
// carrying one method, which is the minimum shape a cross-cutting
// weaver needs: its Generate walks the emit store, so a
// source-only fixture leaves the loop body unexecuted.
func emitStoreWithMethod(t *testing.T, pkg, structName, methodName string) *sdk.Store {
	t.Helper()
	c := sdk.NewProvenance("fixture").WithTarget(sdk.EmitTarget{})
	built, err := c.Package(pkg, pkg).
		Struct(structName, func(s *sdk.StructBuilder) {
			s.Method(methodName, func(m *sdk.MethodBuilder) {
				m.Receiver("r", sdk.Ptr(sdk.Internal(s.Node())))
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
	return s
}

// generateInto drives p.Generate against s and returns the single
// emit method the fixture seeded, for slot assertions.
func generateInto(t *testing.T, p *debugweaver.Plugin, s *sdk.Store) *sdk.EmitMethod {
	t.Helper()
	reader := store.NewReader(s)
	if err := p.Generate(&sdk.GeneratorContext{Store: s, Reader: reader, Diag: diag.New()}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	methods := store.NewReader(s).EmitMethods().Slice()
	if len(methods) != 1 {
		t.Fatalf("fixture should hold exactly one emit method, got %d", len(methods))
	}
	return methods[0]
}

func TestGenerate_WeavesTraceCall(t *testing.T) {
	t.Parallel()

	t.Run("appends one trace statement to the method prebody", func(t *testing.T) {
		t.Parallel()
		s := emitStoreWithMethod(t, "users", "Repo", "Get")
		m := generateInto(t, debugweaver.New(), s)

		prebody := m.Slot("prebody")
		if prebody.Len() != 1 {
			t.Fatalf("prebody should carry exactly one woven statement, got %d", prebody.Len())
		}
		// The rendered record identifies the host as <Type>.<Method>,
		// which is what makes an audit line traceable back to source.
		_, _, args := wovenCall(t, prebody.At(0))
		if len(args) != 2 {
			t.Fatalf("trace call should take (format, owner.method); got %d args: %v", len(args), args)
		}
		if args[1] != "Repo.Get" {
			t.Errorf("trace call should name its owner as Repo.Get; got %q", args[1])
		}
	})

	t.Run("defaults are used when options are unset", func(t *testing.T) {
		t.Parallel()
		s := emitStoreWithMethod(t, "users", "Repo", "Get")
		m := generateInto(t, debugweaver.New(), s)

		pkg, fn, args := wovenCall(t, m.Slot("prebody").At(0))
		if pkg != debugweaver.DefaultPackage || fn != debugweaver.DefaultFunc {
			t.Errorf("trace call = %s.%s, want %s.%s", pkg, fn,
				debugweaver.DefaultPackage, debugweaver.DefaultFunc)
		}
		if args[0] != debugweaver.DefaultFormat {
			t.Errorf("trace format = %q, want the documented default %q", args[0], debugweaver.DefaultFormat)
		}
	})

	t.Run("configured options override the defaults", func(t *testing.T) {
		t.Parallel()
		s := emitStoreWithMethod(t, "users", "Repo", "Get")
		p := debugweaver.New()
		if err := p.SetOptions(opt.New(p.OptionsSchema(), map[string]string{
			"package": "trace", "func": "Enter", "format": "enter: %s",
		})); err != nil {
			t.Fatalf("SetOptions: %v", err)
		}
		pkg, fn, args := wovenCall(t, generateInto(t, p, s).Slot("prebody").At(0))
		if pkg != "trace" || fn != "Enter" {
			t.Errorf("trace call = %s.%s, want trace.Enter", pkg, fn)
		}
		if args[0] != "enter: %s" {
			t.Errorf("trace format = %q, want the configured \"enter: %%s\"", args[0])
		}
	})

	t.Run("a negated directive suppresses the weave", func(t *testing.T) {
		t.Parallel()
		s := emitStoreWithMethod(t, "users", "Repo", "Get")
		m := store.NewReader(s).EmitMethods().Slice()[0]
		m.DirectiveList = append(m.DirectiveList, &sdk.Directive{
			Name: debugweaver.DirectiveName, Negated: true,
		})

		if err := debugweaver.New().Generate(&sdk.GeneratorContext{
			Store: s, Reader: store.NewReader(s), Diag: diag.New(),
		}); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got := m.Slot("prebody").Len(); got != 0 {
			t.Fatalf("-gen:%s should suppress the weave; prebody carries %d statements",
				debugweaver.DirectiveName, got)
		}
	})
}

// wovenCall destructures a woven slot entry into the pieces the
// assertions care about: the callee's package and function name,
// and the string literals passed as arguments. Asserting on the
// emit graph rather than on rendered text keeps these tests
// independent of any backend's formatting.
func wovenCall(t *testing.T, n sdk.EmitNode) (pkg, fn string, args []string) {
	t.Helper()
	trace := wovenTrace(t, n)
	if trace.FuncRef == nil {
		t.Fatalf("woven trace carries no function reference")
	}
	// NewExternal carries the import path on Pkg and the symbol on
	// Name; the backend resolves the alias at render time.
	pkg, fn = trace.FuncRef.Pkg, trace.FuncRef.Name
	// String literals hold their unquoted content in RawText.
	for _, a := range []*sdk.Expr{trace.Format, trace.Subject} {
		if a == nil {
			t.Fatalf("woven trace carries a nil argument expression")
		}
		args = append(args, a.RawText)
	}
	return pkg, fn, args
}

// wovenTrace destructures a woven slot entry into the plugin's own
// emit value.
//
// The entry is a render statement rather than a bare one: the prebody
// slot is constrained to [sdk.EmitKindStmt], and the wrapper is what
// lets this plugin put its own kind — and therefore its own template —
// inside that constraint.
func wovenTrace(t *testing.T, n sdk.EmitNode) *debugweaver.Trace {
	t.Helper()
	// This weaver declares its own emit kind and ships a template for
	// it, so the slot entry is a render statement wrapping the value.
	// Its sibling auditweaver takes the constructor route.
	return plugintest.AssertRenderStmt[*debugweaver.Trace](t, n)
}

// hostBuilder is the annotated struct the woven output is generated
// from, and — through [gofixture.Builder.GoSource] — the package
// it is compiled against.
//
// repogen names `User` in every signature it emits, so nothing this
// plugin contributes can be compiled without the declaration. One
// builder supplies both halves so a field or a name changed here
// cannot leave a second, hand-written spelling behind.
func hostBuilder() *gofixture.Builder {
	return gofixture.New().
		Struct("User", func(s *gofixture.StructBuilder) {
			s.Docs("User is the type the generated repository stores.")
			s.Pos(sdk.Pos{File: "user.go", Line: 1})
			s.Directive(gofixture.Directive(repogen.DirectiveName))
			s.Field("ID", gofixture.Named("string"), nil)
		})
}

// hostMethods names every method repogen puts on `UserRepo`, which is
// the set debug-weaver must weave into exactly once each.
var hostMethods = []string{"Get", "List", "Save", "Delete"}

// weaveInto runs repogen and debug-weaver over one annotated struct
// and adopts the rendered output for the Go assertions.
//
// A weaver owns no file. Its contribution exists only inside a host
// generator's output, so the only place its *rendering* can be judged
// — as opposed to its emit graph, which the unit tests above cover —
// is a pipeline that runs a host alongside it. repogen is that host:
// one annotated struct buys four method bodies and an interface, which
// is the smallest shape that exercises the per-method loop and the
// host-file import registration at the same time.
//
// opts, when non-empty, is applied as debug-weaver's plugin options,
// which is how a consumer redirects the trace at their own logging
// surface.
func weaveInto(t *testing.T, opts map[string]string) *golangtest.Generated {
	t.Helper()
	fixture := hostBuilder()
	b := golangtest.Driver(t, backendgolang.New(), fixture.PackageNode(),
		repogen.New(), debugweaver.New())
	if len(opts) > 0 {
		b = b.WithPluginOptions(debugweaver.Name, opts)
	}
	p := b.Build()
	p.Run("./...")
	// pipelinetest treats a run that produced diagnostics as a
	// completed run, so a panicking plugin leaves an empty sink and a
	// green test — every assertion downstream would then be asserting
	// about nothing at all.
	if p.Diagnostics().HasErrors() {
		t.Fatalf("the run reported errors, so there is nothing to assert on: %v",
			p.Diagnostics().Diagnostics())
	}
	return golangtest.Rendered(t, p).WithSource(golangtest.GoFile(fixture.GoSource()))
}

// TestRender_DefaultTraceIsValidGo is the assertion the emit-graph
// tests above are all proxies for.
//
// Those tests prove the plugin builds the right *values*. Nothing in
// them can see that the template renders a call the compiler accepts,
// that the trace package reached the host file's import set, or that
// the format string and its one argument agree — and all three are
// this plugin's own contract rather than the host's.
//
// The toolchain subtests are parallel: [golangtest.Generated] guards
// its built-module cache, so the seconds each `go` invocation costs
// overlap rather than accumulate.
func TestRender_DefaultTraceIsValidGo(t *testing.T) {
	t.Parallel()

	gen := weaveInto(t, nil)

	t.Run("the woven host file compiles", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
	})

	// vet is the assertion that matters most here: the plugin passes
	// exactly one argument to a printf-style function whose format
	// string is a user-settable option, so a default carrying two
	// verbs or none would ship a vet failure into every consumer's
	// build and no substring check would ever see it.
	t.Run("the trace call satisfies vet's printf check", func(t *testing.T) {
		t.Parallel()
		gen.AssertVets(t)
	})

	// A cross-cutting weaver's worst failure mode is disturbing a
	// contract it does not own. The host emits both the implementation
	// and the interface it implements, so the pair is checkable: a
	// contribution that landed outside the body, or on the wrong
	// receiver, compiles and satisfies nothing.
	t.Run("the weave leaves the host's interface satisfied", func(t *testing.T) {
		t.Parallel()
		gen.AssertSatisfies(t, "UserRepo", "UserRepository")
	})

	// Emitting an import is an API change for every consumer whose
	// module does not already require it. `context` is repogen's;
	// `log` is the only one this plugin is entitled to add.
	t.Run("adds exactly the trace package to the host's imports", func(t *testing.T) {
		t.Parallel()
		gen.Primary(t).AssertImportsOnly(t, "context", "log")
	})

	t.Run("weaves each host method exactly once", func(t *testing.T) {
		t.Parallel()
		src := gen.Primary(t)
		for _, name := range hostMethods {
			call := `log.Printf("debug: %s entered", "UserRepo.` + name + `")`
			src.InMethod(t, "UserRepo", name).AssertCount(t, call, 1)
		}
	})

	// Prebody means first. A contribution rendered after the return is
	// dead code that compiles, vets and satisfies everything — the one
	// failure no other assertion in this file can reach.
	t.Run("the trace runs before the method body", func(t *testing.T) {
		t.Parallel()
		src := gen.Primary(t)
		for _, name := range hostMethods {
			assertPrecedes(t, src.InMethod(t, "UserRepo", name).Body(), "log.Printf", "return")
		}
	})
}

// TestRender_ConfiguredTraceIsValidGo pins the configured path, where
// the plugin's two riskiest behaviours live: it rewrites the host
// file's import set from an option, and it embeds an arbitrary
// user-supplied string in the rendered source.
func TestRender_ConfiguredTraceIsValidGo(t *testing.T) {
	t.Parallel()

	// The format carries a double quote on purpose. It reaches the
	// output through [sdk.NewLiteralString], and an implementation
	// that interpolated the raw text would emit a file that does not
	// parse — which is precisely the class of defect a substring
	// assertion cannot express, because the substring would be
	// unparseable too.
	const format = `entering "%s"`
	gen := weaveInto(t, map[string]string{
		"package": "fmt",
		"func":    "Printf",
		"format":  format,
	})

	t.Run("the reconfigured trace compiles", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
	})

	t.Run("the configured package replaces the default in the host's imports", func(t *testing.T) {
		t.Parallel()
		gen.Primary(t).
			AssertImportsOnly(t, "context", "fmt").
			AssertNoImport(t, debugweaver.DefaultPackage)
	})

	t.Run("the format's quotes are escaped rather than interpolated", func(t *testing.T) {
		t.Parallel()
		gen.Primary(t).
			InMethod(t, "UserRepo", "Get").
			AssertContains(t, `fmt.Printf("entering \"%s\"", "UserRepo.Get")`)
	})
}

// assertPrecedes fails when first does not appear before second in
// body.
//
// Local because [golangtest.Scope] has no ordering assertion:
// [golangtest.Source.AssertOrder] answers the same question for
// top-level declarations, and a slot contributor needs it one level
// down, inside a body.
func assertPrecedes(t *testing.T, body, first, second string) {
	t.Helper()
	a, b := strings.Index(body, first), strings.Index(body, second)
	switch {
	case a < 0:
		t.Errorf("body does not contain %q:\n%s", first, body)
	case b < 0:
		t.Errorf("body does not contain %q:\n%s", second, body)
	case a > b:
		t.Errorf("%q must be woven before %q, but follows it:\n%s", first, second, body)
	}
}
