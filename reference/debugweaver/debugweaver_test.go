// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package debugweaver_test

import (
	"testing"
	"text/template"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/reference/debugweaver"
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
func generateInto(t *testing.T, p *debugweaver.Plugin, s *store.Store) *emit.Method {
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
		m.DirectiveList = append(m.DirectiveList, &directive.Directive{
			Name: debugweaver.DirectiveName, Negated: true,
		})

		if err := debugweaver.New().Generate(&plugin.GeneratorContext{
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
func wovenCall(t *testing.T, n emit.Node) (pkg, fn string, args []string) {
	t.Helper()
	trace := wovenTrace(t, n)
	if trace.FuncRef == nil {
		t.Fatalf("woven trace carries no function reference")
	}
	// NewExternal carries the import path on Pkg and the symbol on
	// Name; the backend resolves the alias at render time.
	pkg, fn = trace.FuncRef.Pkg, trace.FuncRef.Name
	// String literals hold their unquoted content in RawText.
	for _, a := range []*emit.Expr{trace.Format, trace.Subject} {
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
// slot is constrained to [emit.KindStmt], and the wrapper is what lets
// this plugin put its own kind — and therefore its own template —
// inside that constraint.
func wovenTrace(t *testing.T, n emit.Node) *debugweaver.Trace {
	t.Helper()
	stmt, ok := n.(*emit.Stmt)
	if !ok {
		t.Fatalf("slot entry is %T, want *emit.Stmt", n)
	}
	if stmt.StmtKind != emit.StmtRender {
		t.Fatalf("slot entry should be a render statement; got StmtKind=%s", stmt.StmtKind)
	}
	trace, ok := stmt.Node.(*debugweaver.Trace)
	if !ok {
		t.Fatalf("render statement wraps %T, want *debugweaver.Trace", stmt.Node)
	}
	return trace
}
