// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sentinel_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	sentinelplugin "go.thesmos.sh/eidos/plugins/generator/sentinel"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites against
// the sentinel plugin. The framework checks pin the static
// contract; the generator suite drives the plugin against
// fixtures covering the empty-package short-circuit, a package
// with Err* sentinels only, a package with custom error types
// only, and the canonical mixed shape.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, sentinelplugin.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(
			t,
			sentinelplugin.New(),
			[]plugintest.GeneratorFixture{
				{
					Name: "empty package",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return storefixture.New().Build()
					},
				},
				{
					Name: "annotated package with Err* sentinels only",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return buildSentinelOnlyStore(t)
					},
				},
				{
					Name: "annotated package with a custom error type only",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return buildErrorTypeOnlyStore(t)
					},
				},
				{
					Name: "annotated package mixing sentinels and error types",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return buildMixedStore(t)
					},
				},
				{
					Name: "un-annotated package emits nothing",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						// Package without the +gen:sentinel directive
						// must be skipped silently.
						return storefixture.New().
							Package("auth", "example.com/auth").
							Build()
					},
				},
			},
		)
	})

	t.Run("options round-trip", func(t *testing.T) {
		t.Parallel()
		plugintest.RunOptionsSuite(t, sentinelplugin.New(), plugintest.OptionsFixture{
			Valid:      map[string]string{},
			UnknownKey: "no_such_key",
		})
	})

	t.Run("prefix= override is honoured per package", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(
			t,
			sentinelplugin.New(),
			[]plugintest.GeneratorFixture{
				{
					Name: "prefix=custom overrides the default",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return buildStoreWithPrefixOverride(t, "custom: ")
					},
				},
				{
					Name: "prefix=off disables the prefix subtest",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return buildStoreWithPrefixOverride(t, "off")
					},
				},
				{
					Name: "prefix= (empty) disables the prefix subtest",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return buildStoreWithPrefixOverride(t, "")
					},
				},
			},
		)
	})
}

// TestErrorTypeDetection pins that a struct is recognised by its
// method signatures' types.
//
// [node.Return] carries a binding name alongside the type, and
// both spell that field `Name`. Classifying on the binding name
// compiles and matches nothing: every `Error() string` in the wild
// is written anonymously, so the whole custom-error-type half of
// this plugin goes silent — no diagnostic, no output, no failing
// test, because the fixtures only ever asserted the framework
// contract.
func TestErrorTypeDetection(t *testing.T) {
	t.Parallel()

	typesFor := func(t *testing.T, methods func(*storefixture.StructBuilder)) []sentinelplugin.ErrorType {
		t.Helper()
		b := storefixture.New().
			Package("auth", "example.com/auth").
			Struct("ValidationError", func(sb *storefixture.StructBuilder) {
				sb.Field("Field", &node.TypeRef{Name: "string"}, nil)
				methods(sb)
			})
		annotatePackage(b)
		s := b.Build()
		d := diag.Capture()
		if err := sentinelplugin.New().Generate(&plugin.GeneratorContext{
			Store: s, Reader: store.NewReader(s), Diag: d,
		}); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		for _, slot := range s.Emit().PendingOriginSlots() {
			if tests, ok := slot.Item.(*sentinelplugin.Tests); ok {
				return tests.ErrorTypes
			}
		}
		return nil
	}

	t.Run("an anonymous Error() string is recognised", func(t *testing.T) {
		t.Parallel()
		got := typesFor(t, addErrorMethod)
		if len(got) != 1 {
			t.Fatalf("error types = %d, want 1 — the canonical spelling must be detected", len(got))
		}
	})

	t.Run("an anonymous Unwrap() error is recognised", func(t *testing.T) {
		t.Parallel()
		got := typesFor(t, func(sb *storefixture.StructBuilder) {
			addErrorMethod(sb)
			sb.Method(sentinelplugin.UnwrapMethodName, func(mb *storefixture.MethodBuilder) {
				mb.Return(&node.TypeRef{Name: "error"})
			})
		})
		if len(got) != 1 || !got[0].HasUnwrap {
			t.Fatalf("HasUnwrap = %v, want true", len(got) == 1 && got[0].HasUnwrap)
		}
	})

	t.Run("an anonymous Is(error) bool is recognised", func(t *testing.T) {
		t.Parallel()
		got := typesFor(t, func(sb *storefixture.StructBuilder) {
			addErrorMethod(sb)
			sb.Method(sentinelplugin.IsMethodName, func(mb *storefixture.MethodBuilder) {
				mb.Param("target", &node.TypeRef{Name: "error"})
				mb.Return(&node.TypeRef{Name: "bool"})
			})
		})
		if len(got) != 1 || !got[0].HasIs {
			t.Fatalf("HasIs = %v, want true", len(got) == 1 && got[0].HasIs)
		}
	})

	t.Run("a struct without Error() is not an error type", func(t *testing.T) {
		t.Parallel()
		if got := typesFor(t, func(*storefixture.StructBuilder) {}); len(got) != 0 {
			t.Fatalf("error types = %d, want 0", len(got))
		}
	})

	t.Run("a same-named method returning the wrong type is rejected", func(t *testing.T) {
		t.Parallel()
		// Reading the type is what keeps the check a check.
		got := typesFor(t, func(sb *storefixture.StructBuilder) {
			sb.Method(sentinelplugin.ErrorMethodName, func(mb *storefixture.MethodBuilder) {
				mb.Return(&node.TypeRef{Name: "int"})
			})
		})
		if len(got) != 0 {
			t.Fatalf("error types = %d, want 0 for Error() int", len(got))
		}
	})
}

// TestFieldSelection pins which struct fields reach the rendered
// tests.
//
// The rendered test uses a field's sample in two positions Go
// constrains beyond a composite literal: `wantF := <sample>` needs
// a default type and `target.F != wantF` needs a comparable one.
// Neither holds for the nil keyword, and a private zero-literal
// table that answered nil for every width it omitted meant an int8
// field rendered `Code: nil` — a file that does not compile,
// failing in the consumer's build rather than here.
func TestFieldSelection(t *testing.T) {
	t.Parallel()

	fieldsFor := func(t *testing.T) []sentinelplugin.Field {
		t.Helper()
		b := storefixture.New().
			Package("auth", "example.com/auth").
			Struct("ValidationError", func(sb *storefixture.StructBuilder) {
				sb.Field("Field", &node.TypeRef{Name: "string"}, nil)
				sb.Field("Code", &node.TypeRef{Name: "int8"}, nil)
				sb.Field("Ratio", &node.TypeRef{Name: "float64"}, nil)
				sb.Field("Cause", &node.TypeRef{
					TypeKind: node.TypeRefPointer,
					Elem:     &node.TypeRef{Name: "int"},
				}, nil)
				sb.Field("Tags", &node.TypeRef{
					TypeKind: node.TypeRefSlice,
					Elem:     &node.TypeRef{Name: "string"},
				}, nil)
				sb.Field("Elapsed", &node.TypeRef{Package: "time", Name: "Duration"}, nil)
				addErrorMethod(sb)
			})
		annotatePackage(b)
		s := b.Build()

		d := diag.Capture()
		if err := sentinelplugin.New().Generate(&plugin.GeneratorContext{
			Store: s, Reader: store.NewReader(s), Diag: d,
		}); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		for _, slot := range s.Emit().PendingOriginSlots() {
			if tests, ok := slot.Item.(*sentinelplugin.Tests); ok {
				return tests.ErrorTypes[0].Fields
			}
		}
		t.Fatalf("plugin queued no Tests contribution")
		return nil
	}

	names := func(fields []sentinelplugin.Field) []string {
		out := make([]string, 0, len(fields))
		for _, f := range fields {
			out = append(out, f.Name)
		}
		return out
	}

	t.Run("a narrow integer samples as zero rather than nil", func(t *testing.T) {
		t.Parallel()
		for _, f := range fieldsFor(t) {
			if f.Name == "Code" && f.SampleValue != "0" {
				t.Fatalf("Code.SampleValue = %q, want 0", f.SampleValue)
			}
		}
	})

	t.Run("exercises exactly the comparable fields", func(t *testing.T) {
		t.Parallel()
		// A pointer and a slice zero to nil, which has no default
		// type; time.Duration's zero needs a resolution the model
		// cannot do. Each is dropped rather than rendered wrong.
		got := names(fieldsFor(t))
		if !slices.Equal(got, []string{"Field", "Code", "Ratio"}) {
			t.Fatalf("exercised fields = %v, want [Field Code Ratio]", got)
		}
	})

	t.Run("keeps the surviving fields in source order", func(t *testing.T) {
		t.Parallel()
		// Dropping is a filter, not a re-sort: the format assertion
		// reads substrings in declaration order.
		got := names(fieldsFor(t))
		if !slices.IsSorted([]int{slices.Index(got, "Field"), slices.Index(got, "Ratio")}) {
			t.Fatalf("exercised fields = %v, want source order", got)
		}
	})
}

// buildStoreWithPrefixOverride returns a [store.Store] populated
// with an annotated package whose `+gen:sentinel` directive
// carries the supplied prefix value. Both "off" and the empty
// string disable the prefix subtest; any other value pins the
// prefix the subtest asserts.
func buildStoreWithPrefixOverride(t *testing.T, prefix string) *store.Store {
	t.Helper()
	b := storefixture.New().
		Package("auth", "example.com/auth").
		Variable("ErrFoo", func(vb *storefixture.VariableBuilder) {
			vb.Type(&node.TypeRef{Name: "error"})
		})
	pkg := b.PackageNode()
	pkg.DirectiveList = append(pkg.DirectiveList, storefixture.Directive(
		sentinelplugin.DirectiveName,
		storefixture.KV(sentinelplugin.PrefixKey, prefix),
	))
	return b.Build()
}

// buildSentinelOnlyStore returns a [store.Store] populated with
// one annotated source package declaring two Err* sentinel
// variables but no custom error types — exercises the
// Sentinels-only branch of the rendered template.
func buildSentinelOnlyStore(t *testing.T) *store.Store {
	t.Helper()
	b := storefixture.New().
		Package("auth", "example.com/auth").
		Variable("ErrUnauthorised", func(vb *storefixture.VariableBuilder) {
			vb.Type(&node.TypeRef{Name: "error"})
		}).
		Variable("ErrTokenExpired", func(vb *storefixture.VariableBuilder) {
			vb.Type(&node.TypeRef{Name: "error"})
		})
	annotatePackage(b)
	return b.Build()
}

// buildErrorTypeOnlyStore returns a [store.Store] populated
// with one annotated source package declaring a custom error
// type but no Err* sentinels — exercises the ErrorTypes-only
// branch of the rendered template.
func buildErrorTypeOnlyStore(t *testing.T) *store.Store {
	t.Helper()
	b := storefixture.New().
		Package("auth", "example.com/auth").
		Struct("ValidationError", func(sb *storefixture.StructBuilder) {
			sb.Field("Field", &node.TypeRef{Name: "string"}, nil)
			sb.Field("Reason", &node.TypeRef{Name: "string"}, nil)
			addErrorMethod(sb)
		})
	annotatePackage(b)
	return b.Build()
}

// buildMixedStore returns a [store.Store] populated with one
// annotated source package declaring both Err* sentinels and a
// custom error type — the canonical real-world shape testkit's
// sentinel generator targets.
func buildMixedStore(t *testing.T) *store.Store {
	t.Helper()
	b := storefixture.New().
		Package("auth", "example.com/auth").
		Variable("ErrUnauthorised", func(vb *storefixture.VariableBuilder) {
			vb.Type(&node.TypeRef{Name: "error"})
		}).
		Variable("ErrTokenExpired", func(vb *storefixture.VariableBuilder) {
			vb.Type(&node.TypeRef{Name: "error"})
		}).
		Struct("ValidationError", func(sb *storefixture.StructBuilder) {
			sb.Field("Field", &node.TypeRef{Name: "string"}, nil)
			addErrorMethod(sb)
		})
	annotatePackage(b)
	return b.Build()
}

// annotatePackage attaches a `+gen:sentinel` directive to the
// builder's accumulating package node. The storefixture builder
// has no top-level package-directive setter, so the directive
// list is mutated directly through [storefixture.Builder.PackageNode].
func annotatePackage(b *storefixture.Builder) {
	pkg := b.PackageNode()
	pkg.DirectiveList = append(pkg.DirectiveList, storefixture.Directive(sentinelplugin.DirectiveName))
}

// addErrorMethod attaches an `Error() string` method to the
// struct so [structImplementsError] recognises it as a custom
// error type. The method's body is empty — the test fixtures
// exercise the plugin's source-detection path, not the
// rendered tests' runtime behaviour.
func addErrorMethod(sb *storefixture.StructBuilder) {
	sb.Method(sentinelplugin.ErrorMethodName, func(mb *storefixture.MethodBuilder) {
		mb.Return(&node.TypeRef{Name: "string"})
	})
}

// Silence the "imported and not used" lint when the directive
// package isn't referenced in tests that don't override the
// per-package directive (the storefixture's Directive helper
// uses the import transitively).
var _ = directive.Validate
