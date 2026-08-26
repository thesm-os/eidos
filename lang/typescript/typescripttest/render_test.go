// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	tsbackend "go.thesmos.sh/eidos/lang/typescript/backend"
	"go.thesmos.sh/eidos/lang/typescript/typescripttest"
	"go.thesmos.sh/eidos/lang/typescript/typescripttest/tsfixture"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sdk"
)

func TestDriver(t *testing.T) {
	t.Parallel()

	t.Run("a builder stops one call short of running", func(t *testing.T) {
		t.Parallel()
		// Which is what a test needing a builder option, or wanting the
		// pipeline rather than the files, reaches for.
		run := typescripttest.
			Driver(t, tsbackend.New(), fixture().PackageNode(), newEcho()).
			Build().
			Run("./...")
		if run.Sink().Len() == 0 {
			t.Fatal("the driven run wrote nothing")
		}
	})

	t.Run("several packages are driven together", func(t *testing.T) {
		t.Parallel()
		// A generator whose subject is what happens between modules
		// cannot be exercised by a fixture with one package in it.
		first := tsfixture.New().Package("a", "src/a").Interface("A", nil).PackageNode()
		second := tsfixture.New().Package("b", "src/b").Interface("B", nil).PackageNode()

		gen := typescripttest.
			RenderOf(t, tsbackend.New(), []*node.Package{first, second}, newEcho())
		if len(gen.Files()) != 2 {
			t.Fatalf("produced %v, want one file per package", gen.Files())
		}
	})

	t.Run("no fixture package is refused rather than driven", func(t *testing.T) {
		t.Parallel()
		// The run would generate nothing and every assertion about its
		// output would pass having looked at nothing.
		s := probe(t)
		typescripttest.DriverOf(s, tsbackend.New(), nil)
		assertReports(t, s, "no fixture packages")
	})
}

func TestRendered(t *testing.T) {
	t.Parallel()

	t.Run("a run that recorded errors stops the test", func(t *testing.T) {
		t.Parallel()
		// pipelinetest swallows the "had errors" disposition so a test
		// about diagnostics can inspect them, which leaves an empty
		// sink — and every assertion downstream of one passes.
		run := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes(fixture().PackageNode())).
			WithPlugins(newFailing()).
			WithBackend(tsbackend.New()).
			Build().
			Run("./...")

		s := probe(t)
		typescripttest.Rendered(s, run)
		assertReports(t, s, "recorded errors", "vacuous", "deliberate failure")
	})

	t.Run("a routed file keeps its directory", func(t *testing.T) {
		t.Parallel()
		// Two files that landed in different directories have to stay
		// there for a relative specifier between them to resolve the way
		// it will for a consumer.
		pkg := tsfixture.New().
			Interface("User", func(i *tsfixture.InterfaceBuilder) {
				i.Field("id", tsfixture.Named("string"), nil).
					Directive(tsfixture.RouteTo(echoName, "stubs", "stubs"))
			}).
			PackageNode()

		gen := typescripttest.Render(t, tsbackend.New(), pkg, newEcho())
		for _, f := range gen.Files() {
			// Resolved alongside the origin, so the routed directory is
			// a subdirectory of the one the declaration was read from
			// rather than a replacement for it.
			if !strings.HasSuffix(f.Dir(), "stubs") {
				t.Errorf("file %q landed in %q, want a stubs directory", f.Path, f.Dir())
			}
		}
	})
}

// failing is a generator that reports an error and emits nothing.
type failing struct{ *sdk.Base }

func newFailing() *failing {
	return &failing{Base: sdk.NewPlugin("failing").
		Version("1.0.0").
		For(typescripttest.Language, sdk.LanguageSupport{
			Outputs: []sdk.Output{{Suffix: "_fail.gen.ts"}},
			Builtin: true,
		}).
		Build()}
}

// Generate implements [plugin.Generator] by reporting and emitting
// nothing, which is the shape a plugin that bailed leaves behind.
func (*failing) Generate(ctx *plugin.GeneratorContext) error {
	ctx.Diag.For("failing").Errorf(position.Pos{}, "deliberate failure")
	return nil
}
