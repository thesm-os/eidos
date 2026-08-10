// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package auditweaver_test

import (
	"strings"
	"testing"

	backendgolang "go.thesmos.sh/eidos/backend/golang"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/eidostest/golangtest"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/reference/auditweaver"
	"go.thesmos.sh/eidos/reference/debugweaver"
	"go.thesmos.sh/eidos/reference/repogen"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites against
// auditweaver. Cross-cutting weavers operate on emit graphs
// other generators populate, so the suite's per-fixture
// generator probes verify only the contract surface.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, auditweaver.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(
			t,
			auditweaver.New(),
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
							Struct("User", nil).
							Build()
					},
				},
			},
		)
	})

	t.Run("options round-trip", func(t *testing.T) {
		t.Parallel()
		plugintest.RunOptionsSuite(t, auditweaver.New(), plugintest.OptionsFixture{
			Valid: map[string]string{
				"package": "log",
				"func":    "Printf",
				"format":  "audit: %s",
			},
			UnknownKey: "no_such_field",
		})
	})
}

func TestPlugin_ShipsNoTemplates(t *testing.T) {
	t.Parallel()

	// This weaver builds its contribution from the emit constructors,
	// so it owns no kind and needs no template tree. Pinned because the
	// absence is the point: its sibling debugweaver takes the
	// custom-kind route, and the pair is the reference for choosing
	// between them.
	t.Run("declares no template tree for any language", func(t *testing.T) {
		t.Parallel()
		for _, lang := range []string{"golang", "rust"} {
			if _, ok := auditweaver.New().Templates(lang); ok {
				t.Fatalf("plugin claims %s templates; it renders through the backend's own", lang)
			}
		}
	})
}

// TestOptions_TagDefaultsMatchTheDocumentedConstants pins the two
// spellings of every default against each other.
//
// Each default is written twice — once as an exported const the package
// doc points at, once as a `default=` in an [auditweaver.Options] struct
// tag — and a struct tag cannot reference a const, so nothing in the
// language keeps them in step. The tag is the one that wins under a
// pipeline: the holder materialises tag defaults into the struct before
// Generate runs, so p.format()'s const fallback is only ever reached by
// a caller that bypassed SetOptions. Editing the const alone therefore
// changes the documentation, the emit-level tests below, and nothing a
// consumer would ever see.
//
// Found by mutating [auditweaver.DefaultFormat] to a two-verb format
// and watching every assertion in this file stay green.
func TestOptions_TagDefaultsMatchTheDocumentedConstants(t *testing.T) {
	t.Parallel()

	schema := auditweaver.New().OptionsSchema()
	for name, want := range map[string]string{
		"package": auditweaver.DefaultPackage,
		"func":    auditweaver.DefaultFunc,
		"format":  auditweaver.DefaultFormat,
	} {
		t.Run("the "+name+" tag default is the documented constant", func(t *testing.T) {
			t.Parallel()
			f, ok := schema.Lookup(name)
			if !ok {
				t.Fatalf("options schema declares no %q; it declares %v", name, schema.Names())
			}
			if !f.HasDefault {
				t.Fatalf("option %q declares no tag default, so an unset option renders "+
					"the zero value rather than %q", name, want)
			}
			if f.DefaultStr != want {
				t.Errorf("option %q defaults to %q in its struct tag but %q in the exported "+
					"constant the docs cite; the tag is what a pipeline uses",
					name, f.DefaultStr, want)
			}
		})
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
func generateInto(t *testing.T, p *auditweaver.Plugin, s *sdk.Store) *sdk.EmitMethod {
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

func TestGenerate_WeavesAuditCall(t *testing.T) {
	t.Parallel()

	t.Run("appends one audit statement to the method prebody", func(t *testing.T) {
		t.Parallel()
		s := emitStoreWithMethod(t, "users", "Repo", "Get")
		m := generateInto(t, auditweaver.New(), s)

		prebody := m.Slot("prebody")
		if prebody.Len() != 1 {
			t.Fatalf("prebody should carry exactly one woven statement, got %d", prebody.Len())
		}
		// The rendered record identifies the host as <Type>.<Method>,
		// which is what makes an audit line traceable back to source.
		_, _, args := wovenCall(t, prebody.At(0))
		if len(args) != 2 {
			t.Fatalf("audit call should take (format, owner.method); got %d args: %v", len(args), args)
		}
		if args[1] != "Repo.Get" {
			t.Errorf("audit call should name its owner as Repo.Get; got %q", args[1])
		}
	})

	t.Run("defaults are used when options are unset", func(t *testing.T) {
		t.Parallel()
		s := emitStoreWithMethod(t, "users", "Repo", "Get")
		m := generateInto(t, auditweaver.New(), s)

		pkg, fn, args := wovenCall(t, m.Slot("prebody").At(0))
		if pkg != auditweaver.DefaultPackage || fn != auditweaver.DefaultFunc {
			t.Errorf("audit call = %s.%s, want %s.%s", pkg, fn,
				auditweaver.DefaultPackage, auditweaver.DefaultFunc)
		}
		if args[0] != auditweaver.DefaultFormat {
			t.Errorf("audit format = %q, want the documented default %q", args[0], auditweaver.DefaultFormat)
		}
	})

	t.Run("configured options override the defaults", func(t *testing.T) {
		t.Parallel()
		s := emitStoreWithMethod(t, "users", "Repo", "Get")
		p := auditweaver.New()
		if err := p.SetOptions(opt.New(p.OptionsSchema(), map[string]string{
			"package": "audit", "func": "Record", "format": "trail: %s",
		})); err != nil {
			t.Fatalf("SetOptions: %v", err)
		}
		pkg, fn, args := wovenCall(t, generateInto(t, p, s).Slot("prebody").At(0))
		if pkg != "audit" || fn != "Record" {
			t.Errorf("audit call = %s.%s, want audit.Record", pkg, fn)
		}
		if args[0] != "trail: %s" {
			t.Errorf("audit format = %q, want the configured \"trail: %%s\"", args[0])
		}
	})

	t.Run("a negated directive suppresses the weave", func(t *testing.T) {
		t.Parallel()
		s := emitStoreWithMethod(t, "users", "Repo", "Get")
		m := store.NewReader(s).EmitMethods().Slice()[0]
		m.DirectiveList = append(m.DirectiveList, &sdk.Directive{
			Name: auditweaver.DirectiveName, Negated: true,
		})

		if err := auditweaver.New().Generate(&sdk.GeneratorContext{
			Store: s, Reader: store.NewReader(s), Diag: diag.New(),
		}); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got := m.Slot("prebody").Len(); got != 0 {
			t.Fatalf("-gen:%s should suppress the weave; prebody carries %d statements",
				auditweaver.DirectiveName, got)
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
	// This weaver builds its contribution from the emit constructors
	// rather than through a kind of its own, so the slot entry is a
	// plain expression statement. Its sibling debugweaver takes the
	// other route, and asserts through AssertRenderStmt.
	return plugintest.AssertExternalCall(t, n)
}

// repoBuilder builds one struct carrying +gen:repo, which makes
// repogen emit a struct whose methods have bodies — the host shape a
// prebody weaver needs.
func repoBuilder() *storefixture.Builder {
	return storefixture.New().
		Struct("User", func(s *storefixture.StructBuilder) {
			s.Docs("User is the domain type repogen builds a repository around.")
			s.Pos(sdk.Pos{File: "user.go", Line: 1})
			s.Directive(storefixture.Directive(repogen.DirectiveName))
			s.Field("ID", storefixture.Named("string"), nil)
		})
}

// weavePipeline runs the whole stack — frontend, repogen as the host,
// the weavers in the order given, the Go backend — and returns the run.
//
// The generator order is a parameter rather than fixed because
// registration order is exactly what the ordering subtest has to vary:
// auditweaver names debugweaver's capability in Requires, and the claim
// is that the plan's topology decides the prebody order regardless of
// how the two were registered.
func weavePipeline(t *testing.T, gens ...sdk.Generator) *pipelinetest.Pipeline {
	t.Helper()
	return golangtest.Driver(t, backendgolang.New(), repoBuilder().PackageNode(), gens...).
		Build().Run("./...")
}

// hostSource is the package the woven file references, projected from
// the same builder that drove the run.
//
// repogen names User in four signatures and declares it nowhere, so
// the generated file does not compile without it — and every
// toolchain assertion below is therefore gated on this being present
// and correct. Projected rather than spelled out so "correct" is a
// property of the fixture rather than of whoever last edited both.
func hostSource() golangtest.File {
	return golangtest.GoFile(repoBuilder().GoSource())
}

// TestWeave_TheWovenFileIsValidGoAgainstItsDomainType compiles the file
// two weavers wrote into.
//
// The assertion every claim in this file is a proxy for. auditweaver
// contributes a call it never sees rendered: the callee is an
// [sdk.Expr] the backend turns into an import plus a qualified name,
// the arguments are literals the backend escapes, and the statement
// lands inside a method body another plugin owns. Nothing short of a
// compiler can say those four decisions agree — a template that
// interpolated the package name as text, or dropped the import
// registration, renders a byte-identical call and produces a file that
// does not build.
//
// `go vet` is the second half and is not redundant here: the plugin
// emits a printf call whose format string and argument count are chosen
// independently, in Go, from the format option and the subject
// expression. A default that grew a second verb compiles perfectly and
// prints `%!s(MISSING)` in every consumer's audit trail.
//
// # Cost
//
// Three toolchain invocations, some seconds, overlapped because the
// subtests are parallel; every structural claim below is free.
func TestWeave_TheWovenFileIsValidGoAgainstItsDomainType(t *testing.T) {
	t.Parallel()

	gen := golangtest.Rendered(t,
		weavePipeline(t, repogen.New(), debugweaver.New(), auditweaver.New()),
	).WithSource(hostSource())

	// Parsed once, up front: a Source is read-only, so every subtest
	// can share it rather than re-parsing the same bytes.
	src := gen.Primary(t)

	t.Run("builds and vets as a consumer would build it", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
		gen.AssertVets(t)
	})

	// Weaving happens inside method bodies of a type whose whole
	// purpose is to implement the interface generated beside it. A
	// contribution that widened a signature — or a backend that
	// re-rendered the method while splicing the prebody in — leaves a
	// type that still compiles and satisfies nothing.
	t.Run("the woven host still implements the interface it was generated for", func(t *testing.T) {
		t.Parallel()
		gen.AssertSatisfies(t, "UserRepo", "UserRepository")
	})

	t.Run("both weavers render into the host's method bodies", func(t *testing.T) {
		t.Parallel()
		body := src.InMethod(t, "UserRepo", "Get")
		body.AssertContains(t, `log.Printf("debug: %s entered", "UserRepo.Get")`)
		body.AssertContains(t, `log.Printf("audit: %s", "UserRepo.Get")`)
	})

	// Every method, not just the first: Generate ranges over the emit
	// store, so a contribution appended to the wrong host — or appended
	// twice — shows up as a count on some other method while the one
	// the suite happens to sample stays correct.
	t.Run("every method carries exactly one audit record naming its own host", func(t *testing.T) {
		t.Parallel()
		for _, method := range []string{"Get", "List", "Save", "Delete"} {
			src.InMethod(t, "UserRepo", method).
				AssertCount(t, `log.Printf("audit: %s", "UserRepo.`+method+`")`, 1)
		}
	})

	// The import set is the weave's real footprint on a consumer's
	// module: `log` is here only because two weavers named it, and a
	// plugin that started reaching for a second package would raise the
	// bar for every project running this pipeline. Pinning the whole set
	// turns that into a one-line diff — and it is the structural proof
	// that the backend registered the import from the FuncRef
	// expression rather than the template spelling the package as text,
	// which would render the same call and import nothing.
	t.Run("the weave forces exactly the imports its calls need", func(t *testing.T) {
		t.Parallel()
		src.AssertImportsOnly(t, "context", "log")
	})

	// Splicing statements into a body another plugin rendered is the
	// step most likely to leave the file unformatted, and an
	// unformatted generated file surfaces as a diff in the consumer's
	// next `gofmt -l` run, blamed on them.
	t.Run("the spliced file is still gofmt-canonical", func(t *testing.T) {
		t.Parallel()
		src.AssertFormatted(t)
	})
}

// TestWeave_RequiresOrdersThePrebody pins the ordering claim against a
// run that registers the weavers in the opposite order.
//
// Ordering inside a slot is the plan's resolved capability topology,
// not append order. Held in its own test because it is the one claim
// that needs a second pipeline, and it needs no toolchain: the run in
// TestWeave_TheWovenFileIsValidGoAgainstItsDomainType already proved
// this output compiles, and swapping registration order cannot change
// that.
func TestWeave_RequiresOrdersThePrebody(t *testing.T) {
	t.Parallel()

	t.Run("the trace renders before the audit record whatever the registration order", func(t *testing.T) {
		t.Parallel()
		// Registered audit-then-debug, asserted debug-then-audit.
		src := golangtest.Rendered(t,
			weavePipeline(t, repogen.New(), auditweaver.New(), debugweaver.New()),
		).Primary(t)

		assertOrderedInBody(t, src.InMethod(t, "UserRepo", "Get"),
			`"debug: %s entered"`, `"audit: %s"`)
	})
}

// assertOrderedInBody fails unless every marker appears in the scope's
// body, in the order given.
//
// Offsets into the rendered body rather than a structural query:
// golangtest ranks top-level declarations with
// [golangtest.Source.AssertOrder], but slot ordering is a claim about
// statements *inside* one declaration and [golangtest.Scope] carries no
// counterpart. Still worth more than the file-wide strings.Index it
// replaces — the body belongs to one named method, so a second method's
// trace cannot satisfy the claim by accident, and the failure names
// which pair is inverted against a body the parser already accepted.
func assertOrderedInBody(t *testing.T, sc *golangtest.Scope, markers ...string) {
	t.Helper()
	body := sc.Body()
	at := make([]int, len(markers))
	for i, m := range markers {
		at[i] = strings.Index(body, m)
		if at[i] < 0 {
			t.Fatalf("body is missing %q; every weave must render\n--- body ---\n%s", m, body)
		}
	}
	for i := 1; i < len(markers); i++ {
		if at[i-1] >= at[i] {
			t.Errorf("body renders %q at %d, at or after %q at %d; the prebody order is "+
				"auditweaver's declared Requires topology, not registration order\n"+
				"--- body ---\n%s", markers[i-1], at[i-1], markers[i], at[i], body)
		}
	}
}
