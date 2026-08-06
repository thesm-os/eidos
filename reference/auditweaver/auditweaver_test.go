// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package auditweaver_test

import (
	"strings"
	"testing"
	"text/template"

	backendgolang "go.thesmos.sh/eidos/backend/golang"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/reference/auditweaver"
	"go.thesmos.sh/eidos/reference/debugweaver"
	"go.thesmos.sh/eidos/reference/repogen"
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
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return storefixture.New().Build()
					},
				},
				{
					Name: "package with a struct",
					BuildStore: func(t *testing.T) *store.Store {
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

func TestRecord_Kind(t *testing.T) {
	t.Parallel()

	t.Run("reports the kind the template is registered under", func(t *testing.T) {
		t.Parallel()
		if got := (&auditweaver.Record{}).Kind(); got != auditweaver.Kind {
			t.Fatalf("Kind = %q, want %q", got, auditweaver.Kind)
		}
	})
}

func TestPlugin_Templates(t *testing.T) {
	t.Parallel()

	t.Run("ships templates for golang", func(t *testing.T) {
		t.Parallel()
		if _, ok := auditweaver.New().Templates("golang"); !ok {
			t.Fatalf("plugin should ship golang templates")
		}
	})

	t.Run("declines languages it does not target", func(t *testing.T) {
		t.Parallel()
		if _, ok := auditweaver.New().Templates("rust"); ok {
			t.Fatalf("plugin should not claim templates for rust")
		}
	})

	// The backend dispatches by looking up a template named for the
	// value's Kind. A define name that drifts from the declared Kind
	// costs nothing at build time and fails at render time, in
	// someone else's file — so the pairing is pinned here.
	t.Run("defines a template named for the declared Kind", func(t *testing.T) {
		t.Parallel()
		sub, ok := auditweaver.New().Templates("golang")
		if !ok {
			t.Fatalf("plugin should ship golang templates")
		}
		tmpl, err := template.New("probe").Funcs(probeFuncs()).ParseFS(sub, "*.tmpl")
		if err != nil {
			t.Fatalf("parse shipped templates: %v", err)
		}
		if tmpl.Lookup(string(auditweaver.Kind)) == nil {
			t.Fatalf("no template defines %q; the backend would find nothing to render",
				auditweaver.Kind)
		}
	})
}

// probeFuncs stubs the backend helpers the shipped template calls.
// text/template resolves function names at parse time, so parsing the
// template outside the backend needs the names to exist; the bodies
// are never executed here.
func probeFuncs() template.FuncMap {
	return template.FuncMap{
		"renderExpr": func(*emit.Expr) (string, error) { return "", nil },
	}
}

// emitStoreWithMethod returns a store seeded with one emit struct
// carrying one method, which is the minimum shape a cross-cutting
// weaver needs: its Generate walks the emit store, so a
// source-only fixture leaves the loop body unexecuted.
func emitStoreWithMethod(t *testing.T, pkg, structName, methodName string) *store.Store {
	t.Helper()
	c := builder.For("fixture", emit.Target{})
	built, err := c.Package(pkg, pkg).
		Struct(structName, func(s *builder.StructBuilder) {
			s.Method(methodName, func(m *builder.MethodBuilder) {
				m.Receiver("r", emit.Ptr(emit.Internal(s.Node())))
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
func generateInto(t *testing.T, p *auditweaver.Plugin, s *store.Store) *emit.Method {
	t.Helper()
	reader := store.NewReader(s)
	if err := p.Generate(&plugin.GeneratorContext{Store: s, Reader: reader, Diag: diag.New()}); err != nil {
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
		m.DirectiveList = append(m.DirectiveList, &directive.Directive{
			Name: auditweaver.DirectiveName, Negated: true,
		})

		if err := auditweaver.New().Generate(&plugin.GeneratorContext{
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

// wovenRecord destructures a woven slot entry into the plugin's own
// emit value.
//
// The entry is a render statement rather than a bare one: the prebody
// slot is constrained to [emit.KindStmt], and the wrapper is what lets
// this plugin put its own kind — and therefore its own template —
// inside that constraint.
func wovenRecord(t *testing.T, n emit.Node) *auditweaver.Record {
	t.Helper()
	stmt, ok := n.(*emit.Stmt)
	if !ok {
		t.Fatalf("slot entry is %T, want *emit.Stmt", n)
	}
	if stmt.StmtKind != emit.StmtRender {
		t.Fatalf("slot entry should be a render statement; got StmtKind=%s", stmt.StmtKind)
	}
	record, ok := stmt.Node.(*auditweaver.Record)
	if !ok {
		t.Fatalf("render statement wraps %T, want *auditweaver.Record", stmt.Node)
	}
	return record
}

// wovenCall destructures a woven slot entry into the pieces the
// assertions care about: the callee's package and function name,
// and the string literals passed as arguments. Asserting on the
// emit graph rather than on rendered text keeps these tests
// independent of any backend's formatting.
func wovenCall(t *testing.T, n emit.Node) (pkg, fn string, args []string) {
	t.Helper()
	record := wovenRecord(t, n)
	if record.FuncRef == nil {
		t.Fatalf("woven record carries no function reference")
	}
	// NewExternal carries the import path on Pkg and the symbol on
	// Name; the backend resolves the alias at render time.
	pkg, fn = record.FuncRef.Pkg, record.FuncRef.Name
	// String literals hold their unquoted content in RawText.
	for _, a := range []*emit.Expr{record.Format, record.Subject} {
		if a == nil {
			t.Fatalf("woven record carries a nil argument expression")
		}
		args = append(args, a.RawText)
	}
	return pkg, fn, args
}

// repoPkg builds one struct carrying +gen:repo, which makes repogen
// emit a struct whose methods have bodies — the host shape a prebody
// weaver needs.
func repoPkg(t *testing.T) *node.Package {
	t.Helper()
	return storefixture.New().
		Struct("User", func(s *storefixture.StructBuilder) {
			s.Pos(position.Pos{File: "user.go", Line: 1})
			s.Directive(storefixture.Directive(repogen.DirectiveName))
		}).PackageNode()
}

func TestWeave_WeaversRenderThroughTheirOwnTemplates(t *testing.T) {
	t.Parallel()

	t.Run("both weavers render into the host's method bodies", func(t *testing.T) {
		t.Parallel()
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes(repoPkg(t))).
			WithGenerator(repogen.New()).
			WithGenerator(debugweaver.New()).
			WithGenerator(auditweaver.New()).
			WithBackend(backendgolang.New()).
			Build()
		p.Run("./...")

		body := p.AssertFile("user_repo.go").String()
		for _, want := range []string{
			`log.Printf("debug: %s entered", "UserRepo.Get")`,
			`log.Printf("audit: %s", "UserRepo.Get")`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("rendered file is missing %q:\n%s", want, body)
			}
		}
	})

	// auditweaver names debugweaver's capability in Requires, so the
	// trace must render first however the two were registered — which
	// is why this subtest registers them in the opposite order to the
	// one it asserts.
	t.Run("Requires orders the prebody, registration order does not", func(t *testing.T) {
		t.Parallel()
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes(repoPkg(t))).
			WithGenerator(repogen.New()).
			WithGenerator(auditweaver.New()).
			WithGenerator(debugweaver.New()).
			WithBackend(backendgolang.New()).
			Build()
		p.Run("./...")

		body := p.AssertFile("user_repo.go").String()
		trace := strings.Index(body, `"debug: %s entered"`)
		audit := strings.Index(body, `"audit: %s"`)
		if trace < 0 || audit < 0 {
			t.Fatalf("both weaves must render; got trace=%d audit=%d:\n%s", trace, audit, body)
		}
		if trace >= audit {
			t.Errorf("trace must render before the audit record, the order auditweaver's "+
				"Requires declares:\n%s", body)
		}
	})

	// The import is registered by the backend rendering the FuncRef
	// expression, not by either plugin managing imports itself. A
	// template that interpolated the package name as text would render
	// the same call and produce an uncompilable file.
	t.Run("the weavers' package is imported by the host file", func(t *testing.T) {
		t.Parallel()
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes(repoPkg(t))).
			WithGenerator(repogen.New()).
			WithGenerator(debugweaver.New()).
			WithGenerator(auditweaver.New()).
			WithBackend(backendgolang.New()).
			Build()
		p.Run("./...")

		body := p.AssertFile("user_repo.go").String()
		if !strings.Contains(body, `"log"`) {
			t.Errorf("rendered file should import log for the woven calls:\n%s", body)
		}
	})
}
