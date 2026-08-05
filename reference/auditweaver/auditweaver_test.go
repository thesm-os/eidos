// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package auditweaver_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/reference/auditweaver"
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

// wovenCall destructures a woven slot entry into the pieces the
// assertions care about: the callee's package and function name,
// and the string literals passed as arguments. Asserting on the
// emit graph rather than on rendered text keeps these tests
// independent of any backend's formatting.
func wovenCall(t *testing.T, n emit.Node) (pkg, fn string, args []string) {
	t.Helper()
	stmt, ok := n.(*emit.Stmt)
	if !ok {
		t.Fatalf("slot entry is %T, want *emit.Stmt", n)
	}
	if stmt.Call == nil {
		t.Fatalf("woven statement carries no call expression")
	}
	callee := stmt.Call.Callee
	if callee == nil {
		t.Fatalf("woven call has no callee")
	}
	// NewExternal carries the import path on Pkg and the symbol on
	// Name; the backend resolves the alias at render time.
	pkg, fn = callee.Pkg, callee.Name
	// String literals hold their unquoted content in RawText.
	for _, a := range stmt.Call.Args {
		args = append(args, a.RawText)
	}
	return pkg, fn, args
}
