// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	"go.thesmos.sh/eidos/plugins/annotator/defaults"
	builderplugin "go.thesmos.sh/eidos/plugins/generator/builder"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// The projection these tests assert on is what the seam bought.
//
// Classification used to happen at render time, inside a template, so
// the only way to check that a `map[K]struct{}` field owed an entry
// setter was to render Go and read it — which this module cannot do,
// since it may not import a backend. Asking the language through
// [sdk.SourceRules] moved the answer into Go, where a table of
// declarations against expected shapes is an ordinary test.

// project drives Generate over one annotated struct and returns the
// builder it queued.
func project(t *testing.T, configure func(*gofixture.StructBuilder)) *builderplugin.Type {
	t.Helper()
	value, _ := projectBoth(t, configure)
	return value
}

// projectBoth returns the builder and the checks queued beside it.
func projectBoth(
	t *testing.T, configure func(*gofixture.StructBuilder),
) (*builderplugin.Type, *builderplugin.Tests) {
	t.Helper()
	b := gofixture.New().
		Package("blog", "example.com/blog").
		Struct("Article", func(sb *gofixture.StructBuilder) {
			sb.Directive(gofixture.Directive(builderplugin.DirectiveName))
			configure(sb)
		})
	s := b.Build()

	// The defaults annotator runs first, exactly as a pipeline orders
	// it: the builder reads a stamp rather than the directive, so a
	// fixture that skipped this step would assert the builder ignores
	// declared defaults — which it does, when nothing stamped them.
	d := diag.Capture()
	if err := defaults.New().Annotate(&sdk.AnnotatorContext{
		Store: s, Reader: store.NewReader(s), Diag: d,
	}); err != nil {
		t.Fatalf("defaults.Annotate: %v", err)
	}
	if err := builderplugin.New().Generate(&sdk.GeneratorContext{
		Store: s, Reader: store.NewReader(s), Diag: d,
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var value *builderplugin.Type
	var checks *builderplugin.Tests
	for _, slot := range s.Emit().PendingOriginSlots() {
		switch item := slot.Item.(type) {
		case *builderplugin.Type:
			value = item
		case *builderplugin.Tests:
			checks = item
		}
	}
	if value == nil {
		t.Fatalf("plugin queued no builder; diagnostics: %+v", d.Diagnostics())
	}
	return value, checks
}

// fieldNamed returns the projected member by name.
func fieldNamed(t *testing.T, value *builderplugin.Type, name string) builderplugin.Field {
	t.Helper()
	for i := range value.Fields {
		if value.Fields[i].Name == name {
			return value.Fields[i]
		}
	}
	t.Fatalf("no member %q in the projection", name)
	return builderplugin.Field{}
}

// Each Go spelling reaches the neutral shape that decides its setters.
func TestProjectionClassifiesGoTypes(t *testing.T) {
	t.Parallel()

	value := project(t, func(sb *gofixture.StructBuilder) {
		sb.Field("Title", gofixture.Named("string"), nil)
		sb.Field("Tags", gofixture.Slice(gofixture.Named("string")), nil)
		sb.Field("Body", gofixture.Slice(gofixture.Named("byte")), nil)
		sb.Field("Meta", gofixture.Map(
			gofixture.Named("string"), gofixture.Named("int"),
		), nil)
		sb.Field("Seen", gofixture.Map(
			gofixture.Named("string"), gofixture.AnonStruct(nil, nil),
		), nil)
		sb.Field("Author", gofixture.Pointer(gofixture.Named("string")), nil)
	})

	for _, tc := range []struct {
		member string
		want   sdk.TypeShape
		why    string
	}{
		{"Title", sdk.ShapeScalar, "a plain value owes one replacing setter"},
		{"Tags", sdk.ShapeSequence, "a slice owes a variadic setter and an appending one"},
		{"Body", sdk.ShapeBytes, "a byte slice also owes a string-accepting setter"},
		{"Meta", sdk.ShapeMapping, "a map owes an entry setter carrying a value"},
		{"Seen", sdk.ShapeSet, "a map to the empty struct owes an entry setter without one"},
		{"Author", sdk.ShapeOptional, "a pointer owes a setter that addresses the value"},
	} {
		t.Run(tc.member+" is "+string(tc.want), func(t *testing.T) {
			t.Parallel()
			if got := fieldNamed(t, value, tc.member).Shape; got != tc.want {
				t.Errorf("%s classified as %q, want %q — %s", tc.member, got, tc.want, tc.why)
			}
		})
	}
}

// A set is recognised before the map it is made of.
//
// The narrower reading has to win: classified as a mapping, a
// `map[K]struct{}` gets an entry setter asking the caller for the one
// thing they cannot vary.
func TestSetIsRecognisedBeforeMapping(t *testing.T) {
	t.Parallel()

	value := project(t, func(sb *gofixture.StructBuilder) {
		sb.Field("Seen", gofixture.Map(
			gofixture.Named("string"), gofixture.AnonStruct(nil, nil),
		), nil)
	})
	f := fieldNamed(t, value, "Seen")

	t.Run("carries a key", func(t *testing.T) {
		t.Parallel()
		if f.Key == nil {
			t.Error("a set is addressed by key, so the key type has to reach the template")
		}
	})

	t.Run("carries no element", func(t *testing.T) {
		t.Parallel()
		if f.Elem != nil {
			t.Error("every value in a set is the same one, so there is none worth carrying")
		}
	})
}

// The composite shapes name the inner types their setters take.
func TestProjectionLiftsInnerTypes(t *testing.T) {
	t.Parallel()

	value := project(t, func(sb *gofixture.StructBuilder) {
		sb.Field("Tags", gofixture.Slice(gofixture.Named("string")), nil)
		sb.Field("Meta", gofixture.Map(
			gofixture.Named("string"), gofixture.Named("int"),
		), nil)
	})

	t.Run("a sequence names its element", func(t *testing.T) {
		t.Parallel()
		if fieldNamed(t, value, "Tags").Elem == nil {
			t.Error("the variadic setter's parameter type comes from the element")
		}
	})

	t.Run("a mapping names both halves", func(t *testing.T) {
		t.Parallel()
		f := fieldNamed(t, value, "Meta")
		if f.Key == nil || f.Elem == nil {
			t.Errorf("the entry setter takes both; got key=%v elem=%v", f.Key, f.Elem)
		}
	})
}

// A member opts out through the tag, and a mistyped value is refused.
func TestSkipTag(t *testing.T) {
	t.Parallel()

	t.Run("the documented value excludes the member", func(t *testing.T) {
		t.Parallel()
		value := project(t, func(sb *gofixture.StructBuilder) {
			sb.Field("Title", gofixture.Named("string"), nil)
			sb.Field("Internal", gofixture.Named("string"), func(fb *gofixture.FieldBuilder) {
				fb.Tag("`builder:\"-\"`")
			})
		})
		for _, f := range value.Fields {
			if f.Name == "Internal" {
				t.Error("a member tagged out still got a setter")
			}
		}
	})

	t.Run("any other value keeps the member", func(t *testing.T) {
		t.Parallel()
		value := project(t, func(sb *gofixture.StructBuilder) {
			sb.Field("Title", gofixture.Named("string"), func(fb *gofixture.FieldBuilder) {
				fb.Tag("`builder:\"skip\"`")
			})
		})
		// Reported rather than obeyed: a typo that silently dropped the
		// setter would leave the author with a builder that cannot set
		// the member and nothing saying why.
		if len(value.Fields) != 1 {
			t.Errorf("got %d members, want the mistyped one kept", len(value.Fields))
		}
	})
}

// A declared default reaches the constructor, and the language decides
// whether it is the zero.
func TestDeclaredDefaults(t *testing.T) {
	t.Parallel()

	withDefault := func(v string) func(*gofixture.StructBuilder) {
		return func(sb *gofixture.StructBuilder) {
			sb.Field("Retries", gofixture.Named("int"), func(fb *gofixture.FieldBuilder) {
				fb.Directive(gofixture.Directive(
					defaults.DirectiveName, gofixture.Arg(v),
				))
			})
		}
	}

	t.Run("a declared value seeds the constructor", func(t *testing.T) {
		t.Parallel()
		value := project(t, withDefault("5"))
		if got := fieldNamed(t, value, "Retries").Default; got != "5" {
			t.Errorf("Default = %q, want the declared 5", got)
		}
		if !value.Seeded() {
			t.Error("a member with a default makes the constructor build a literal")
		}
	})

	t.Run("a zero default is marked as one", func(t *testing.T) {
		t.Parallel()
		value := project(t, withDefault("0"))
		if !fieldNamed(t, value, "Retries").DefaultIsZero {
			t.Error("a check comparing against the zero passes against a constructor " +
				"that ignored the declaration, which is what the flag withholds")
		}
	})

	t.Run("a non-zero default is not", func(t *testing.T) {
		t.Parallel()
		value := project(t, withDefault("5"))
		if fieldNamed(t, value, "Retries").DefaultIsZero {
			t.Error("5 is not the zero of an int")
		}
	})
}

// The builder's name is stamped where a second generator can read it.
func TestPublishesItsTypeName(t *testing.T) {
	t.Parallel()

	b := gofixture.New().
		Package("blog", "example.com/blog").
		Struct("Article", func(sb *gofixture.StructBuilder) {
			sb.Directive(gofixture.Directive(builderplugin.DirectiveName))
			sb.Field("Title", gofixture.Named("string"), nil)
		})
	s := b.Build()
	if err := builderplugin.New().Generate(&sdk.GeneratorContext{
		Store: s, Reader: store.NewReader(s), Diag: diag.Capture(),
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	decl, found := store.NewReader(s).Structs().
		Where(func(x *sdk.Struct) bool { return x.Name == "Article" }).First()
	if !found {
		t.Fatal("fixture lost its struct")
	}
	got, _ := builderplugin.MetaType.Get(decl.Meta())
	if got != "ArticleBuilder" {
		t.Errorf("MetaType = %q, want ArticleBuilder — a downstream generator "+
			"reads this rather than re-deriving the convention", got)
	}
}

// The checks travel with the builder unless asked not to.
func TestChecksAreEmittedBeside(t *testing.T) {
	t.Parallel()

	_, checks := projectBoth(t, func(sb *gofixture.StructBuilder) {
		sb.Field("Title", gofixture.Named("string"), nil)
	})
	if checks == nil {
		t.Fatal("no checks queued; a builder nothing exercises is asserted by nobody")
	}

	t.Run("the constructors are named by the language", func(t *testing.T) {
		t.Parallel()
		if checks.CtorName != "NewArticle" || checks.FromName != "NewArticleFrom" {
			t.Errorf("got %q and %q, want Go's constructor spelling",
				checks.CtorName, checks.FromName)
		}
	})

	t.Run("a plain declaration is instantiable", func(t *testing.T) {
		t.Parallel()
		if !checks.Instantiable() {
			t.Error("a declaration with no type parameters needs no witness")
		}
	})
}

// Two members reaching one setter identifier are refused, naming
// both.
//
// `Data []byte` owes a text-accepting setter beside its plain one, so
// it reaches `WithDataString` — and so does a plain member called
// `DataString`. Emitted, the file declares one method twice and the
// failure lands in the consumer's build. Detecting it needs the
// derived names in the core, which is why they are composed there
// from words the language declares rather than in the template.
func TestSetterNameCollisionIsRefused(t *testing.T) {
	t.Parallel()

	b := gofixture.New().
		Package("blog", "example.com/blog").
		Struct("Payload", func(sb *gofixture.StructBuilder) {
			sb.Directive(gofixture.Directive(builderplugin.DirectiveName))
			sb.Field("Data", gofixture.Slice(gofixture.Named("byte")), nil)
			sb.Field("DataString", gofixture.Named("string"), nil)
		})
	s := b.Build()
	d := diag.Capture()
	if err := builderplugin.New().Generate(&sdk.GeneratorContext{
		Store: s, Reader: store.NewReader(s), Diag: d,
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	t.Run("nothing is queued", func(t *testing.T) {
		t.Parallel()
		if len(s.Emit().PendingOriginSlots()) != 0 {
			t.Error("a builder declaring one method twice does not compile, so " +
				"emitting it moves the failure into the consumer's build")
		}
	})

	t.Run("and both members are named", func(t *testing.T) {
		t.Parallel()
		if !d.HasErrors() {
			t.Fatal("a refusal with nothing to say sends the author looking")
		}
	})
}

// A member that owes no extra setter cannot collide with itself.
//
// The guard has to distinguish the shapes: reading every member as
// though it owed the whole set would report a collision between two
// plain members whose setters differ.
func TestDistinctMembersAreNotRefused(t *testing.T) {
	t.Parallel()

	value := project(t, func(sb *gofixture.StructBuilder) {
		sb.Field("Data", gofixture.Slice(gofixture.Named("byte")), nil)
		sb.Field("Note", gofixture.Named("string"), nil)
	})

	for _, tc := range []struct{ member, want string }{
		{"Data", "WithData"},
		{"Note", "WithNote"},
	} {
		t.Run(tc.member+" sets through "+tc.want, func(t *testing.T) {
			t.Parallel()
			if got := fieldNamed(t, value, tc.member).Set; got != tc.want {
				t.Errorf("Set = %q, want %q — the word is the language's and the "+
					"order is the plugin's", got, tc.want)
			}
		})
	}

	t.Run("only a byte sequence owes a text setter", func(t *testing.T) {
		t.Parallel()
		if fieldNamed(t, value, "Note").SetText != "" {
			t.Error("a plain member owes one setter, and a name derived for a " +
				"second would collide with a member that genuinely has that name")
		}
		if fieldNamed(t, value, "Data").SetText != "WithDataString" {
			t.Error("the text-accepting setter is what makes the collision possible")
		}
	})
}

// A declaration with nothing to set is refused rather than emitted.
func TestEmptyDeclarationIsReported(t *testing.T) {
	t.Parallel()

	b := gofixture.New().
		Package("blog", "example.com/blog").
		Struct("Empty", func(sb *gofixture.StructBuilder) {
			sb.Directive(gofixture.Directive(builderplugin.DirectiveName))
		})
	s := b.Build()
	d := diag.Capture()
	if err := builderplugin.New().Generate(&sdk.GeneratorContext{
		Store: s, Reader: store.NewReader(s), Diag: d,
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !d.HasErrors() {
		t.Error("a builder with no setters configures nothing; emitting the shell " +
			"would hide a declaration that cannot do what it says")
	}
	if len(s.Emit().PendingOriginSlots()) != 0 {
		t.Error("nothing should be queued for a declaration that was refused")
	}
}

// TestConformance_Golang drives the framework suites over
// Go-shaped fixtures.
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
					return gofixture.New().
						Package("blog", "example.com/blog").
						Struct("Article", func(sb *gofixture.StructBuilder) {
							sb.Field("Title", gofixture.Named("string"), nil)
						}).
						Build()
				},
			},
			{
				Name: "annotated struct across every shape",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					b := gofixture.New().
						Package("blog", "example.com/blog").
						Struct("Article", func(sb *gofixture.StructBuilder) {
							sb.Directive(gofixture.Directive(builderplugin.DirectiveName))
							sb.Field("Title", gofixture.Named("string"), nil)
							sb.Field("Tags", gofixture.Slice(gofixture.Named("string")), nil)
							sb.Field("Body", gofixture.Slice(gofixture.Named("byte")), nil)
							sb.Field("Meta", gofixture.Map(
								gofixture.Named("string"), gofixture.Named("int"),
							), nil)
							sb.Field("Author", gofixture.Pointer(gofixture.Named("string")), nil)
						})
					s := b.Build()
					return s
				},
			},
		},
	)
}
