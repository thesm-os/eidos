// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"errors"
	"testing"

	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	builderplugin "go.thesmos.sh/eidos/plugins/generator/builder"
	"go.thesmos.sh/eidos/sdk"
)

// TestConformance_Golang drives the language-neutral
// [plugintest.RunGeneratorSuite] against fixtures shaped
// from Go-frontend conventions — builtin names like "string",
// "int", "byte"; `[]E` / `map[K]V` composites; pointer
// elements; generic type parameters. The plugin's
// [Generate] pass stays neutral but the source nodes the
// fixture builds are populated with Go-shape primitives, so
// these fixtures live alongside the Go funcmap that
// interprets them.
//
// The conformance suite asserts only panic-safety, source-
// frozen-ness, and emit-projection determinism. It never
// looks at the rendered text, so every fixture below would
// pass against a template emitting a redeclared name or a
// setter at the wrong arity. Closing that gap needs a
// backend, which this module may not import; the compiler is
// put behind the rendered builder end-to-end by the
// demoproject acceptance test instead.
func TestConformance_Golang(t *testing.T) {
	t.Parallel()

	plugintest.RunGeneratorSuite(
		t,
		builderplugin.New(),
		[]plugintest.GeneratorFixture{
			{
				Name: "un-annotated struct emits nothing",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return storefixture.New().
						Package("blog", "example.com/blog").
						Struct("Article", func(sb *storefixture.StructBuilder) {
							sb.Field("Title", &sdk.TypeRef{Name: "string"}, nil)
						}).
						Build()
				},
			},
			{
				Name: "annotated struct with scalar fields",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return buildScalarStore(t)
				},
			},
			{
				Name: "annotated struct with slice, map, and bytes fields",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return buildCollectionStore(t)
				},
			},
			{
				Name: "annotated struct with a pointer field",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return buildPointerStore(t)
				},
			},
			{
				Name: "annotated generic struct",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return buildGenericStore(t)
				},
			},
			{
				Name: "annotated struct with defaults=pkg.Func",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return buildDefaultsStore(t)
				},
			},
			{
				Name: "annotated struct with no exported fields",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return buildUnexportedOnlyStore(t)
				},
			},
		},
	)
}

// TestGoDefaultsExpr pins the builder-specific `defaults=`
// parser — both accepted forms and every malformed shape that
// should surface [builder.ErrMalformedDefaults] as a
// render-time error.
// Identifier-convention and type-ref-shape helpers used by
// the same template are tested upstream in the
// [go.thesmos.sh/eidos/lang/golang] package.
func TestGoDefaultsExpr(t *testing.T) {
	t.Parallel()

	t.Run("well-formed value parses into an External call", func(t *testing.T) {
		t.Parallel()
		got, err := builderplugin.GoDefaultsExpr("example.com/blog.ArticleDefaults", "example.com/src")
		if err != nil {
			t.Fatalf("well-formed value must parse: %v", err)
		}
		if got == nil {
			t.Fatalf("well-formed value must return non-nil expression")
		}
		if got.Pkg != "example.com/blog" {
			t.Fatalf("Pkg = %q, want the qualified path, not the source package", got.Pkg)
		}
	})

	t.Run("a bare identifier resolves against the source package", func(t *testing.T) {
		t.Parallel()
		// The factory beside the annotated struct is the common
		// case. Resolving it to a package rather than leaving a
		// bare identifier is what lets the same directive work
		// when `out=` / `pkg=` moves the builder elsewhere: the
		// backend elides the qualifier in the source package and
		// adds it everywhere else.
		got, err := builderplugin.GoDefaultsExpr("defaultUser", "example.com/src")
		if err != nil {
			t.Fatalf("bare identifier must parse: %v", err)
		}
		if got.Pkg != "example.com/src" {
			t.Fatalf("Pkg = %q, want the source package", got.Pkg)
		}
		if got.Name != "defaultUser" {
			t.Fatalf("Name = %q, want defaultUser", got.Name)
		}
	})

	t.Run("malformed values surface ErrMalformedDefaults", func(t *testing.T) {
		t.Parallel()
		malformed := []string{
			"",
			".leading_dot",
			"trailing_dot.",
		}
		for _, raw := range malformed {
			t.Run(raw, func(t *testing.T) {
				t.Parallel()
				_, err := builderplugin.GoDefaultsExpr(raw, "example.com/src")
				if !errors.Is(err, builderplugin.ErrMalformedDefaults) {
					t.Errorf("expected ErrMalformedDefaults for %q; got %v", raw, err)
				}
			})
		}
	})
}

// buildScalarStore returns a [sdk.Store] populated with one
// annotated struct carrying only scalar fields — exercises
// the default With<Field> branch of the per-field-shape
// template.
func buildScalarStore(t *testing.T) *sdk.Store {
	t.Helper()
	return storefixture.New().
		Package("blog", "example.com/blog").
		Struct("Article", func(sb *storefixture.StructBuilder) {
			sb.Directive(storefixture.Directive(builderplugin.DirectiveName))
			sb.Field("Title", &sdk.TypeRef{Name: "string"}, nil)
			sb.Field("Views", &sdk.TypeRef{Name: "int"}, nil)
			sb.Field("Published", &sdk.TypeRef{Name: "bool"}, nil)
		}).
		Build()
}

// buildCollectionStore returns a [sdk.Store] populated with
// one annotated struct carrying a slice, a map, and a []byte
// field — exercises every variadic / entry / string-
// convenience branch of the rendered builder.
func buildCollectionStore(t *testing.T) *sdk.Store {
	t.Helper()
	return storefixture.New().
		Package("blog", "example.com/blog").
		Struct("Article", func(sb *storefixture.StructBuilder) {
			sb.Directive(storefixture.Directive(builderplugin.DirectiveName))
			sb.Field("Tags", storefixture.Slice(&sdk.TypeRef{Name: "string"}), nil)
			sb.Field("Metadata", storefixture.Map(
				&sdk.TypeRef{Name: "string"},
				&sdk.TypeRef{Name: "string"},
			), nil)
			sb.Field("Body", storefixture.Slice(&sdk.TypeRef{Name: "byte"}), nil)
		}).
		Build()
}

// buildPointerStore returns a [sdk.Store] populated with one
// annotated struct carrying a pointer field — exercises the
// pointer-passthrough branch of refconv.FromNode.
func buildPointerStore(t *testing.T) *sdk.Store {
	t.Helper()
	return storefixture.New().
		Package("blog", "example.com/blog").
		Struct("Article", func(sb *storefixture.StructBuilder) {
			sb.Directive(storefixture.Directive(builderplugin.DirectiveName))
			sb.Field("Author", storefixture.Pointer(&sdk.TypeRef{Name: "string"}), nil)
		}).
		Build()
}

// buildGenericStore returns a [sdk.Store] populated with one
// annotated generic struct — exercises the type-parameter
// decl / args plumbing across builder type, constructors,
// setters, Mutate, Clone, and Build.
func buildGenericStore(t *testing.T) *sdk.Store {
	t.Helper()
	return storefixture.New().
		Package("blog", "example.com/blog").
		Struct("Container", func(sb *storefixture.StructBuilder) {
			sb.Directive(storefixture.Directive(builderplugin.DirectiveName))
			sb.TypeParam("T", nil)
			sb.Field("Item", storefixture.TypeParamRef("T"), nil)
			sb.Field("Label", &sdk.TypeRef{Name: "string"}, nil)
		}).
		Build()
}

// buildDefaultsStore returns a [sdk.Store] populated with
// one annotated struct carrying the
// `defaults=example.com/blog.ArticleDefaults` override —
// exercises the additional New<Name>WithDefaults constructor
// branch.
func buildDefaultsStore(t *testing.T) *sdk.Store {
	t.Helper()
	return storefixture.New().
		Package("blog", "example.com/blog").
		Struct("Article", func(sb *storefixture.StructBuilder) {
			sb.Directive(storefixture.Directive(
				builderplugin.DirectiveName,
				storefixture.KV(builderplugin.DefaultsKey, "example.com/blog.ArticleDefaults"),
			))
			sb.Field("Title", &sdk.TypeRef{Name: "string"}, nil)
		}).
		Build()
}

// buildUnexportedOnlyStore returns a [sdk.Store] populated
// with one annotated struct whose only fields are unexported
// — the builder must render an empty setter set without
// short-circuiting the type / constructor / Build emission.
func buildUnexportedOnlyStore(t *testing.T) *sdk.Store {
	t.Helper()
	return storefixture.New().
		Package("blog", "example.com/blog").
		Struct("Article", func(sb *storefixture.StructBuilder) {
			sb.Directive(storefixture.Directive(builderplugin.DirectiveName))
			sb.Field("internal", &sdk.TypeRef{Name: "string"}, nil)
		}).
		Build()
}
