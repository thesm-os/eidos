// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/lang/golang/backend"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// TestMultiOutput_EndToEnd drives a multi-output plugin through
// the existing Go backend without backend changes — the
// architectural win of the M2 routing surface. The fixture plugin
// declares two outputs (production + tests), builds a struct into
// the primary output and a function into the tagged "test"
// output via the `pkg.File("test")` sub-context, and runs the
// pipeline end-to-end. Both files appear in the sink with the
// expected basenames, the `_test.go → <pkg>_test` package shift
// fires on the tagged output only, and the rendered bodies carry
// the right package clauses and declarations.
func TestMultiOutput_EndToEnd(t *testing.T) {
	t.Parallel()

	t.Run(
		"primary and tagged outputs render through the Go backend with the correct package shifts",
		func(t *testing.T) {
			t.Parallel()
			origin := &node.Struct{
				BaseNode: node.BaseNode{
					SourcePos: position.Pos{File: "internal/users/user.go"},
				},
				Name:    "User",
				Package: "example.com/users",
			}
			nodePkg := &node.Package{
				Name:    "users",
				Path:    "example.com/users",
				Structs: []*node.Struct{origin},
			}

			fixture := &multiOutputFixture{name: "enumkit", origin: origin}
			p := pipelinetest.New(t).
				WithFrontend(pipelinetest.FromNodes(nodePkg)).
				WithGenerator(fixture).
				WithBackend(mustNew(t)).
				Build().
				Run()

			if errs := p.Diagnostics().Diagnostics(); len(errs) != 0 {
				for _, d := range errs {
					t.Logf("diag: %s — %s", d.Severity, d.Message)
				}
				t.Fatalf("multi-output run reported %d diagnostics", len(errs))
			}

			primary := p.AssertFile("user_enum.go")
			primary.Contains("package users")
			primary.Contains("type UserStatus struct")
			primary.NotContains("package users_test")

			test := p.AssertFile("user_enum_test.go")
			test.Contains("package users_test")
			test.Contains("func TestUserStatusRoundTrip")
			test.NotContains("type UserStatus struct")
		},
	)
}

// multiOutputFixture is the test plugin that exercises the M2
// multi-output Builder + Layout surface: it declares two
// [plugin.Output] entries (production + tests) and emits one
// decl into each via the `pkg.File(tag)` sub-context.
type multiOutputFixture struct {
	name   string
	origin node.Node
}

// Name returns the configured plugin identifier.
func (g *multiOutputFixture) Name() string { return g.name }

// Outputs declares the production + test output set in Go. Other
// languages surface no routable output.
func (*multiOutputFixture) Outputs(lang string) []plugin.Output {
	if lang != backend.Language {
		return nil
	}
	return []plugin.Output{
		{Suffix: "_enum.go"},
		{Tag: "test", Suffix: "_enum_test.go"},
	}
}

// Generate builds one struct into the primary output and one test
// function into the tagged "test" output. The sub-context stamps
// OutputTag on every decl built through it; Layout dispatches each
// decl to its declared output's suffix.
func (g *multiOutputFixture) Generate(ctx *plugin.GeneratorContext) error {
	c := builder.For(g.name).Anchor(g.origin)
	pkg, err := c.
		Struct("UserStatus", nil).
		File("test").Function("TestUserStatusRoundTrip", func(fb *builder.FunctionBuilder) {
		fb.Param("t", emit.Ptr(emit.External("testing", "T")), nil)
	}).
		Build()
	if err != nil {
		return err
	}
	return ctx.Store.Emit().AddPackage(pkg)
}
