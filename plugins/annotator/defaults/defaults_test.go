// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package defaults_test

import (
	"slices"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	"go.thesmos.sh/eidos/plugins/annotator/defaults"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// run drives the annotator over a fixture and returns the store plus
// every diagnostic raised.
//
// The language marker is stamped here rather than in each case: the
// plugin picks its rules from it, so a fixture without one exercises
// the unread-language path instead of whatever the case meant to test.
// [runAs] is the knob for cases that want that path.
func run(t *testing.T, b *gofixture.Builder) (*sdk.Store, []sdk.Diag) {
	t.Helper()
	return runAs(t, b, golang.Language)
}

func runAs(t *testing.T, b *gofixture.Builder, lang string) (*sdk.Store, []sdk.Diag) {
	t.Helper()
	s := b.Build()
	pkg := b.PackageNode()
	if lang != "" {
		sdk.MetaFrontend.Set(pkg.EnsureMeta(), lang, "test")
	}
	sink := diag.New()
	ctx := &sdk.AnnotatorContext{Store: s, Reader: store.NewReader(s), Diag: sink}
	if err := defaults.New().Annotate(ctx); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	return s, sink.Diagnostics()
}

// fieldBag returns the metadata bag of one field on one struct.
func fieldBag(t *testing.T, s *sdk.Store, structName, fieldName string) *sdk.Bag {
	t.Helper()
	decl, found := store.NewReader(s).Structs().
		Where(func(x *sdk.Struct) bool { return x.Name == structName }).First()
	if !found {
		t.Fatalf("fixture has no struct %q", structName)
	}
	for _, f := range decl.Fields {
		if f.Name == fieldName {
			return f.Meta()
		}
	}
	t.Fatalf("struct %q has no field %q", structName, fieldName)
	return nil
}

// The framework contracts.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, defaults.New())
	})

	t.Run("annotator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunAnnotatorSuite(t, defaults.New(), []plugintest.AnnotatorFixture{
			{
				Name: "empty store",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return gofixture.New().Build()
				},
			},
			{
				Name: "fields declaring defaults both ways",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return declaredBothWays().Build()
				},
			},
		})
	})
}

// declaredBothWays is the canonical fixture: one directive, one tag,
// one field carrying both, and one carrying neither.
func declaredBothWays() *gofixture.Builder {
	return gofixture.New().
		Struct("Config", func(s *gofixture.StructBuilder) {
			s.Field("Host", gofixture.Named("string"), func(f *gofixture.FieldBuilder) {
				f.Directive(gofixture.Directive(
					defaults.DirectiveName, gofixture.Arg(`"localhost"`),
				))
			})
			s.Field("Port", gofixture.Named("int"), func(f *gofixture.FieldBuilder) {
				f.Tag("`default:\"8080\"`")
			})
			s.Field("Retries", gofixture.Named("int"), func(f *gofixture.FieldBuilder) {
				f.Tag("`default:\"1\"`")
				f.Directive(gofixture.Directive(
					defaults.DirectiveName, gofixture.Arg("5"),
				))
			})
			s.Field("Plain", gofixture.Named("string"), nil)
		})
}

// Both declaration forms reach the same stamp.
func TestDeclarationForms(t *testing.T) {
	t.Parallel()

	s, _ := run(t, declaredBothWays())

	t.Run("a directive stamps its value verbatim", func(t *testing.T) {
		t.Parallel()
		bag := fieldBag(t, s, "Config", "Host")
		if got := defaults.DefaultOf(bag); got != `"localhost"` {
			t.Errorf("DefaultOf = %q, want the literal as the author spelled it", got)
		}
	})

	t.Run("a tag stamps its value", func(t *testing.T) {
		t.Parallel()
		bag := fieldBag(t, s, "Config", "Port")
		if got := defaults.DefaultOf(bag); got != "8080" {
			t.Errorf("DefaultOf = %q, want 8080", got)
		}
	})

	t.Run("each records which form declared it", func(t *testing.T) {
		t.Parallel()
		if got := defaults.DefaultSource(fieldBag(t, s, "Config", "Host")); got != defaults.SourceDirective {
			t.Errorf("Host source = %q, want %q", got, defaults.SourceDirective)
		}
		if got := defaults.DefaultSource(fieldBag(t, s, "Config", "Port")); got != defaults.SourceTag {
			t.Errorf("Port source = %q, want %q", got, defaults.SourceTag)
		}
	})

	t.Run("the directive wins over the tag", func(t *testing.T) {
		t.Parallel()
		bag := fieldBag(t, s, "Config", "Retries")
		if got := defaults.DefaultOf(bag); got != "5" {
			t.Errorf("DefaultOf = %q, want the directive's 5 — a tag cannot override "+
				"the more specific statement", got)
		}
	})

	t.Run("a field declaring neither is unstamped", func(t *testing.T) {
		t.Parallel()
		if got := defaults.DefaultOf(fieldBag(t, s, "Config", "Plain")); got != "" {
			t.Errorf("DefaultOf = %q, want empty — the absence is the empty string", got)
		}
	})
}

// An explicit zero is a declared value, not an absence.
//
// The distinction the whole stamp exists for: a reader seeing a value
// knows the author asked for it, and one reading a bare zero would
// emit the same constructor either way.
func TestExplicitZeroIsDeclared(t *testing.T) {
	t.Parallel()

	for _, zero := range []string{"0", "nil", "false", `""`} {
		t.Run("stamps "+zero, func(t *testing.T) {
			t.Parallel()
			s, _ := run(t, gofixture.New().
				Struct("T", func(sb *gofixture.StructBuilder) {
					sb.Field("F", gofixture.Named("int"), func(f *gofixture.FieldBuilder) {
						f.Directive(gofixture.Directive(
							defaults.DirectiveName, gofixture.Arg(zero),
						))
					})
				}))
			if got := defaults.DefaultOf(fieldBag(t, s, "T", "F")); got != zero {
				t.Errorf("DefaultOf = %q, want %q", got, zero)
			}
		})
	}
}

// A qualified value carries the package a rendered file must import.
func TestQualifiedValue(t *testing.T) {
	t.Parallel()

	t.Run("a full path splits into package and symbol", func(t *testing.T) {
		t.Parallel()
		s, _ := run(t, gofixture.New().
			Struct("T", func(sb *gofixture.StructBuilder) {
				sb.Field("Region", gofixture.Named("string"), func(f *gofixture.FieldBuilder) {
					f.Directive(gofixture.Directive(defaults.DirectiveName,
						gofixture.Arg("example.com/seed.DefaultRegion")))
				})
			}))
		bag := fieldBag(t, s, "T", "Region")
		if got := defaults.DefaultOf(bag); got != "DefaultRegion" {
			t.Errorf("DefaultOf = %q, want the symbol", got)
		}
		if got := defaults.DefaultPackage(bag); got != "example.com/seed" {
			t.Errorf("DefaultPackage = %q, want the import path", got)
		}
	})

	t.Run("a plain literal names no package", func(t *testing.T) {
		t.Parallel()
		s, _ := run(t, declaredBothWays())
		if got := defaults.DefaultPackage(fieldBag(t, s, "Config", "Host")); got != "" {
			t.Errorf("DefaultPackage = %q, want empty — a value written out imports nothing", got)
		}
	})
}

// A value the language cannot render is refused where it was written.
func TestMalformedValueIsReported(t *testing.T) {
	t.Parallel()

	s, diags := run(t, gofixture.New().
		Struct("T", func(sb *gofixture.StructBuilder) {
			sb.Field("F", gofixture.Named("string"), func(f *gofixture.FieldBuilder) {
				f.Directive(gofixture.Directive(
					defaults.DirectiveName, gofixture.Arg(`"unterminated`),
				))
			})
		}))

	t.Run("nothing is stamped", func(t *testing.T) {
		t.Parallel()
		if got := defaults.DefaultOf(fieldBag(t, s, "T", "F")); got != "" {
			t.Errorf("DefaultOf = %q; a value that cannot render must not reach a template", got)
		}
	})

	t.Run("the declaration is named", func(t *testing.T) {
		t.Parallel()
		namesField := func(d sdk.Diag) bool { return strings.Contains(d.Message, "T.F") }
		if !slices.ContainsFunc(diags, namesField) {
			t.Errorf("no diagnostic names the field; got %v", diags)
		}
	})
}

// A language the plugin cannot read is reported, not skipped.
//
// Silence here is the failure: every default in those packages goes
// unstamped and every constructor seeds nothing, which renders as a
// plausible file that ignored the source.
func TestUnreadLanguageIsReported(t *testing.T) {
	t.Parallel()

	s, diags := runAs(t, declaredBothWays(), "fortran")

	t.Run("nothing is stamped", func(t *testing.T) {
		t.Parallel()
		if got := defaults.DefaultOf(fieldBag(t, s, "Config", "Host")); got != "" {
			t.Errorf("DefaultOf = %q; an unread language must not stamp", got)
		}
	})

	t.Run("the language and the alternatives are named", func(t *testing.T) {
		t.Parallel()
		namesBoth := func(d sdk.Diag) bool {
			return strings.Contains(d.Message, "fortran") &&
				strings.Contains(d.Message, golang.Language)
		}
		if !slices.ContainsFunc(diags, namesBoth) {
			t.Errorf("the diagnostic must name the language and what would have been read; got %v", diags)
		}
	})
}

// The plugin declares what it reads through the same mechanism a
// generator declares what it renders.
func TestDeclaredLanguages(t *testing.T) {
	t.Parallel()

	p := defaults.New()

	t.Run("Go is declared", func(t *testing.T) {
		t.Parallel()
		if got := p.Languages(); !slices.Contains(got, golang.Language) {
			t.Errorf("Languages = %v, want it to include %q", got, golang.Language)
		}
	})

	t.Run("the read side is reachable", func(t *testing.T) {
		t.Parallel()
		if _, ok := p.Source(golang.Language); !ok {
			t.Error("Source declined Go; an annotator that renders nothing still reads")
		}
	})

	t.Run("nothing is emitted for it", func(t *testing.T) {
		t.Parallel()
		if got := p.Outputs(golang.Language); len(got) != 0 {
			t.Errorf("Outputs = %v; an output Layout composes a name for is one this "+
				"plugin never writes", got)
		}
	})
}
