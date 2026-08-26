// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest_test

// The acceptance rung: real TypeScript source, read by the real
// tree-sitter frontend, projected onto the emit graph, rendered by
// the real backend — and the output parses and type-checks.
//
// This is the check the backend's own doc points at. Validity cannot
// be asserted in-backend, because parsing needs tree-sitter and
// depguard forbids a backend importing a frontend — correctly, since
// the alternative is a backend carrying a C toolchain. This package
// matches neither side's file glob and already links both, which is
// what makes it the one place the claim can be stated.
//
// The mirror generator between the two halves copies the ts.* keys it
// names and nothing else — the copied set IS the round-trip claim,
// and a wholesale bag copy would make the test pass for keys nothing
// renders.

import (
	"os"
	"path/filepath"
	"testing"

	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/lang/typescript"
	tsbackend "go.thesmos.sh/eidos/lang/typescript/backend"
	tsfrontend "go.thesmos.sh/eidos/lang/typescript/frontend"
	"go.thesmos.sh/eidos/lang/typescript/typescripttest"
	"go.thesmos.sh/eidos/sdk"
)

// roundtripSource is the module the run reads. Every construct here
// is one the frontend stamps, the mirror copies, and the backend
// renders — which is what makes the file the round trip's contract.
const roundtripSource = `/** Access levels. */
export enum Role {
  Admin = 'admin',
  Guest = 'guest',
}

export const enum Level {
  Low,
  High,
}

/** A user of the system. */
export interface User {
  [key: string]: unknown;
  readonly id: string;
  nick?: string;
  tags: string[];
  role: Role;
  greet(loud: boolean): string;
  close?(): void;
}

export interface Box<T = string> {
  value: T;
}
`

func TestRoundTripAcceptance(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "user.ts"), []byte(roundtripSource), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	run := pipelinetest.New(t).
		WithFrontend(tsfrontend.New()).
		WithPluginOptions(tsfrontend.FrontendName, map[string]string{"dir": dir}).
		WithGenerator(newMirror()).
		WithBackend(tsbackend.New()).
		Build().
		Run("./...")

	gen := typescripttest.Rendered(t, run)

	t.Run("the output parses", func(t *testing.T) {
		t.Parallel()
		// The acceptance claim itself: a broken template fails here, on
		// a run from real source, rather than in a consumer's build.
		gen.AssertParses(t)
	})

	t.Run("the output type-checks under strict tsc", func(t *testing.T) {
		t.Parallel()
		// The assertion the CI gate holds to. It skips where no
		// compiler answers, which is right on a machine that never
		// installed one — so the `check` job sets
		// EIDOS_TYPESCRIPT_TOOLCHAIN, and a skip there is a failure
		// instead. Without that a broken Node step turns the only
		// check on the rendered output into a green no-op, and the
		// regression it was watching for lands unnoticed.
		typescripttest.Rendered(t, run).AssertTypeChecks(t)
	})

	t.Run("the stamps survive the round trip", func(t *testing.T) {
		t.Parallel()
		out := gen.Primary(t)
		out.AssertProperty(t, "User", "id", "string").
			AssertProperty(t, "User", "nick", "string").
			AssertProperty(t, "User", "role", "Role")
		out.AssertContains(t, "readonly id: string;").
			AssertContains(t, "nick?: string;").
			AssertContains(t, "close?(): void;").
			AssertContains(t, "[key: string]: unknown;").
			AssertContains(t, "export const enum Level {").
			AssertContains(t, "interface Box<T = string> {").
			AssertContains(t, "Admin = 'admin',")
	})

	t.Run("the docs survive as JSDoc", func(t *testing.T) {
		t.Parallel()
		gen.Primary(t).AssertInterface(t, "User").AssertDoc(t, "A user of the system.")
	})
}

// mirrorName is the round-trip generator's plugin identifier.
const mirrorName = "mirror"

// mirror projects every source declaration onto its emit counterpart,
// copying the metadata keys named in the copy helpers below.
type mirror struct{ *sdk.Base }

func newMirror() *mirror {
	return &mirror{Base: sdk.NewPlugin(mirrorName).
		Version("1.0.0").
		For(typescripttest.Language, sdk.LanguageSupport{
			Outputs: []sdk.Output{{Suffix: "_mirror.gen.ts"}},
			Builtin: true,
		}).
		Build()}
}

// Generate implements [plugin.Generator].
func (*mirror) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(mirrorName)
	for _, pkg := range ctx.Reader.Packages().Slice() {
		b := c.Package(pkg.Name, pkg.Path)
		for _, src := range pkg.Interfaces {
			mirrorInterface(b, src)
		}
		for _, src := range pkg.Enums {
			mirrorEnum(b, src)
		}
		built, err := b.Build()
		if err != nil {
			return err
		}
		if err := ctx.Store.Emit().AddPackage(built); err != nil {
			return err
		}
	}
	return nil
}

// mirrorInterface projects one source interface, members and stamps
// alike.
func mirrorInterface(b *sdk.PackageBuilder, src *sdk.Interface) {
	b.Interface(src.Name, func(ib *sdk.InterfaceBuilder) {
		ib.Origin(src)
		ib.Docs(src.DocLines...)
		copyString(typescript.MetaIndexSignature, src, ib.Node())
		copyString(typescript.MetaConstructSignature, src, ib.Node())
		for _, p := range src.TypeParams {
			ib.TypeParam(p.Name, nil, func(tb *sdk.TypeParamBuilder) {
				copyString(typescript.MetaTypeParamDefault, p, tb.Node())
			})
		}
		for _, f := range src.Fields {
			ib.Field(f.Name, typescript.FromNode(f.Type), func(fb *sdk.FieldBuilder) {
				copyBool(typescript.MetaReadonly, f, fb.Node())
				copyBool(typescript.MetaOptional, f, fb.Node())
			})
		}
		for _, m := range src.Methods {
			ib.Method(m.Name, func(mb *sdk.MethodBuilder) {
				for _, p := range m.Params {
					mb.Param(p.Name, typescript.FromNode(p.Type))
				}
				for _, r := range m.Returns {
					mb.Return(typescript.FromNode(r.Type))
				}
			})
			emitted := ib.Node().Methods[len(ib.Node().Methods)-1]
			copyBool(typescript.MetaOptional, m, emitted)
		}
	})
}

// mirrorEnum projects one source enum with its verbatim member
// values.
func mirrorEnum(b *sdk.PackageBuilder, src *sdk.Enum) {
	b.Enum(src.Name, nil, func(eb *sdk.EnumBuilder) {
		eb.Origin(src)
		eb.Docs(src.DocLines...)
		copyBool(typescript.MetaConstEnum, src, eb.Node())
		for _, v := range src.Variants {
			var value *sdk.Expr
			if v.Value != "" {
				// Verbatim, so the source's quoting is the output's.
				value = sdk.NewLiteralRaw(v.Value)
			}
			eb.Variant(v.Name, value, nil)
		}
	})
}

// copyBool carries one bool-valued stamp from a source node to its
// emit counterpart.
func copyBool(key sdk.Key[bool], src, dst interface{ EnsureMeta() *sdk.Bag }) {
	if v, ok := key.Get(src.EnsureMeta()); ok {
		key.Set(dst.EnsureMeta(), v, mirrorName)
	}
}

// copyString is [copyBool] over string-valued stamps.
func copyString(key sdk.Key[string], src, dst interface{ EnsureMeta() *sdk.Bag }) {
	if v, ok := key.Get(src.EnsureMeta()); ok {
		key.Set(dst.EnsureMeta(), v, mirrorName)
	}
}
