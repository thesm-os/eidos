// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest_test

import (
	"testing"

	tsbackend "go.thesmos.sh/eidos/lang/typescript/backend"
	"go.thesmos.sh/eidos/lang/typescript/typescripttest"
	"go.thesmos.sh/eidos/lang/typescript/typescripttest/tsfixture"
	"go.thesmos.sh/eidos/sdk"
)

// The end-to-end shape the package's own documentation prescribes:
// build a fixture, render it through the real backend, assert on what
// came out. Everything else in this package's tests drives one
// assertion in isolation; this one proves the path a plugin author
// actually walks holds together.

// echoName is the plugin under test in the end-to-end run.
const echoName = "echo"

// fixture returns a package a generator can be pointed at.
func fixture() *tsfixture.Builder {
	return tsfixture.New().
		Interface("User", func(i *tsfixture.InterfaceBuilder) {
			i.Docs("A user of the system.").
				Field("id", tsfixture.Named("string"), nil).
				Field("nick", tsfixture.Named("string"), func(f *tsfixture.FieldBuilder) {
					f.Optional()
				})
		})
}

// echo mirrors every source interface into an emit interface whose
// properties are all strings — the smallest generator that produces
// output worth asserting on.
type echo struct{ *sdk.Base }

// newEcho returns the plugin, declaring that it owns a file but
// renders it through the backend's own kind templates.
func newEcho() *echo {
	return &echo{Base: sdk.NewPlugin(echoName).
		Version("1.0.0").
		For(typescripttest.Language, sdk.LanguageSupport{
			Outputs: []sdk.Output{{Suffix: "_echo.gen.ts"}},
			Builtin: true,
		}).
		Build()}
}

// Generate implements [plugin.Generator].
func (*echo) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(echoName)
	for _, pkg := range ctx.Reader.Packages().Slice() {
		for _, src := range pkg.Interfaces {
			built, err := c.Package(pkg.Name, pkg.Path).
				Interface(src.Name+"Echo", func(i *sdk.InterfaceBuilder) {
					i.Origin(src)
					for _, f := range src.Fields {
						i.Field(f.Name, sdk.Builtin("string"), nil)
					}
				}).
				Build()
			if err != nil {
				return err
			}
			if err := ctx.Store.Emit().AddPackage(built); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestRenderEndToEnd(t *testing.T) {
	t.Parallel()

	gen := typescripttest.Render(t, tsbackend.New(), fixture().PackageNode(), newEcho())

	t.Run("the output parses", func(t *testing.T) {
		t.Parallel()
		// The floor: a substring check passes against a template that
		// renders an unclosed brace.
		gen.AssertParses(t)
	})

	t.Run("the output declares what the generator meant", func(t *testing.T) {
		t.Parallel()
		gen.Primary(t).
			AssertGeneratedHeader(t).
			AssertProperty(t, "UserEcho", "id", "string").
			AssertProperty(t, "UserEcho", "nick", "string").
			AssertNoProperty(t, "UserEcho", "absent")
		gen.Primary(t).AssertInterface(t, "UserEcho")
	})

	t.Run("the output type-checks against the source module", func(t *testing.T) {
		t.Parallel()
		// Skips where no TypeScript compiler answers; see the package
		// doc for why that asymmetry is deliberate.
		typescripttest.
			Render(t, tsbackend.New(), fixture().PackageNode(), newEcho()).
			WithSource(typescripttest.TSFile(fixture().TSSource())).
			AssertTypeChecks(t)
	})
}
