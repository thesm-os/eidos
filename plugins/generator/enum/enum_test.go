// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package enum_test

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"text/template"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/plugin"
	enumplugin "go.thesmos.sh/eidos/plugins/generator/enum"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites against
// the enum plugin. The framework checks pin the static contract
// (stable Name, role implementation, deterministic Outputs,
// well-formed shape); the generator suite pins the
// determinism / frozen-source / diagnostic-discipline contracts
// across a representative fixture set.
//
// Every fixture here is an input the plugin handles cleanly, which
// is what a conformance fixture means: the suite fails any fixture
// producing an Error-severity diagnostic, because the pipeline turns
// one into a non-zero exit for the user. The one input that does
// produce a diagnostic — an annotated enum with no variants — is
// pinned directly in [TestGenerateOnAnnotatedEnumWithNoVariants],
// where the assertion is about the diagnostic rather than about the
// contracts a valid input satisfies.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, enumplugin.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(
			t,
			enumplugin.New(),
			[]plugintest.GeneratorFixture{
				{
					Name: "empty package",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return storefixture.New().Build()
					},
				},
				{
					Name: "annotated enum with two variants",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return buildEnumStore(t, withoutOverrides)
					},
				},
				{
					Name: "annotated enum with a +gen:value override",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return buildEnumStore(t, withOverride)
					},
				},
			},
		)
	})

	t.Run("options round-trip", func(t *testing.T) {
		t.Parallel()
		plugintest.RunOptionsSuite(t, enumplugin.New(), plugintest.OptionsFixture{
			Valid: map[string]string{
				"strip_prefix":    "true",
				"parse_prefix":    "Parse",
				"sentinel_prefix": "ErrUnknown",
			},
			UnknownKey: "no_such_key",
		})
	})
}

// TestGenerateOnAnnotatedEnumWithNoVariants pins the plugin's only
// diagnostic path: an enum carrying `+gen:enum` with nothing to
// enumerate.
//
// Driven directly rather than through [plugintest.RunGeneratorSuite]
// because the two disagree about what the input is. To the suite a
// fixture is an input the plugin handles; this one it refuses, on
// purpose, and the refusal is the behaviour worth pinning — with its
// severity and its position, neither of which a conformance run
// inspects.
func TestGenerateOnAnnotatedEnumWithNoVariants(t *testing.T) {
	t.Parallel()

	t.Run("the empty enum is reported as an Error-severity diagnostic", func(t *testing.T) {
		t.Parallel()

		_, d := generateEmptyEnum(t)

		if !d.HasErrors() {
			t.Fatalf("an annotated enum with no variants produced no Error diagnostic; got %+v",
				d.Diagnostics())
		}
		if got := d.Diagnostics()[0].Message; !strings.Contains(got, enumplugin.ErrEnumHasNoVariants.Error()) {
			t.Errorf("diagnostic message %q does not name ErrEnumHasNoVariants", got)
		}
	})

	t.Run("the diagnostic carries the enum's own position", func(t *testing.T) {
		t.Parallel()

		_, d := generateEmptyEnum(t)

		got := d.Diagnostics()[0].Pos
		if got.IsZero() {
			t.Fatal("the diagnostic carries no position; it renders as a dash where the file and " +
				"line belong and the user cannot find the enum to fix")
		}
		if want := emptyEnumPos; got != want {
			t.Errorf("diagnostic position = %v; want the enum's own %v", got, want)
		}
	})

	t.Run("the empty enum contributes nothing to the emit graph", func(t *testing.T) {
		t.Parallel()

		s, _ := generateEmptyEnum(t)

		if got := len(s.Emit().PendingOriginSlots()); got != 0 {
			t.Errorf("plugin queued %d contribution(s) for an enum it reported on; want 0 — a "+
				"diagnostic that does not also stop the emit renders a broken file", got)
		}
	})
}

// emptyEnumPos is the position the no-variant fixture declares, so
// the position assertion compares against a value the fixture owns
// rather than a literal repeated on both sides.
var emptyEnumPos = position.At("empty/empty.go", 1, 1)

// generateEmptyEnum drives Generate against a store holding one
// annotated enum with no variants and returns the store together
// with the diagnostics it produced.
//
// The sink is [diag.Capture] rather than [diag.Discard]: the whole
// point of this test is what was emitted.
func generateEmptyEnum(t *testing.T) (*store.Store, *diag.Sink) {
	t.Helper()
	s := storefixture.New().
		Package("empty", "example.com/empty").
		Enum("Empty", func(eb *storefixture.EnumBuilder) {
			eb.Pos(emptyEnumPos)
			eb.Directive(storefixture.Directive(enumplugin.DirectiveName))
		}).
		Build()
	d := diag.Capture()
	if err := enumplugin.New().Generate(&plugin.GeneratorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   d,
	}); err != nil {
		t.Fatalf("Generate returned an error on an enum it should have reported on: %v", err)
	}
	return s, d
}

// buildEnumStore returns a [store.Store] populated with one
// source enum named Status declared in `status/status.go`. The
// configure hook tweaks the per-test variant set / directive
// layout. Used by the generator suite, which expects a full
// store; the backend-driven end-to-end acceptance test lives
// outside this package (plugins cannot import backends).
func buildEnumStore(t *testing.T, configure func(*storefixture.EnumBuilder)) *store.Store {
	t.Helper()
	return storefixture.New().
		Package("status", "example.com/status").
		Enum("Status", func(eb *storefixture.EnumBuilder) {
			eb.Pos(position.At("status/status.go", 1, 1))
			eb.Directive(storefixture.Directive(enumplugin.DirectiveName))
			configure(eb)
		}).
		Build()
}

// withoutOverrides configures the canonical two-variant enum
// (`StatusActive`, `StatusInactive`) with no per-variant
// `+gen:value` directives — the default prefix-stripping rule
// resolves both to `"Active"` / `"Inactive"`.
func withoutOverrides(eb *storefixture.EnumBuilder) {
	eb.Variant("StatusActive", "0")
	eb.Variant("StatusInactive", "1")
}

// withOverride adds a third variant whose rendered string-form
// is pinned via `+gen:value pending_review` — exercising the
// override branch of the variant-name resolver.
func withOverride(eb *storefixture.EnumBuilder) {
	eb.Variant("StatusActive", "0")
	eb.Variant("StatusInactive", "1")
	eb.Variant("StatusPending", "2")
	// Reach into the just-appended variant to attach the
	// override directive — the storefixture's Variant signature
	// is flat (no callback), so the directive list is mutated
	// after construction.
	enum := eb.Node()
	pending := enum.Variants[len(enum.Variants)-1]
	pending.DirectiveList = append(pending.DirectiveList, &directive.Directive{
		Name: "value",
		Args: []string{"pending_review"},
	})
}

// TestStringValuedEnum covers the string-underlying path.
//
// The rendered API is not uniform across underlying types. A string
// enum's declared value *is* its textual form and is already written
// down; deriving one from the identifier instead loses the only thing
// the declaration said, so a value read from JSON, a database column
// or an HTTP parameter did not parse and MarshalJSON emitted the
// identifier. The `String` fallback compounded it: `int(v)` applied to
// a string-valued type does not compile at all, so every generated
// file for a string enum failed the consumer's build.
func TestStringValuedEnum(t *testing.T) {
	t.Parallel()

	// apiFor drives Generate over one enum and returns the queued
	// production-API contribution.
	apiFor := func(t *testing.T, underlying string, configure func(*storefixture.EnumBuilder)) *enumplugin.API {
		t.Helper()
		s := storefixture.New().
			Package("region", "example.com/region").
			Enum("Region", func(eb *storefixture.EnumBuilder) {
				eb.Pos(position.At("region/region.go", 1, 1))
				eb.Directive(storefixture.Directive(enumplugin.DirectiveName))
				eb.Underlying(storefixture.Named(underlying))
				configure(eb)
			}).
			Build()
		d := diag.Capture()
		if err := enumplugin.New().Generate(&plugin.GeneratorContext{
			Store: s, Reader: store.NewReader(s), Diag: d,
		}); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if d.HasErrors() {
			t.Fatalf("unexpected diagnostics: %+v", d.Diagnostics())
		}
		for _, slot := range s.Emit().PendingOriginSlots() {
			if api, ok := slot.Item.(*enumplugin.API); ok {
				return api
			}
		}
		t.Fatalf("plugin queued no API contribution")
		return nil
	}

	// stringVariants declares the reported fixture: values that
	// differ from their identifiers in both case and content.
	stringVariants := func(eb *storefixture.EnumBuilder) {
		eb.Variant("US", `"us-east"`)
		eb.Variant("EU", `"eu-west"`)
	}

	t.Run("the underlying type reaches the template data", func(t *testing.T) {
		t.Parallel()
		// Without this the template cannot branch, and the numeric
		// fallback is emitted for every enum regardless of type.
		if got := apiFor(t, "string", stringVariants).Underlying; got != "string" {
			t.Fatalf("API.Underlying = %q, want string", got)
		}
	})

	t.Run("a string variant renders its declared value", func(t *testing.T) {
		t.Parallel()
		api := apiFor(t, "string", stringVariants)
		if got := api.Variants[0].StringValue; got != "us-east" {
			t.Fatalf("StringValue = %q, want us-east (the declared value, not the identifier)", got)
		}
		if got := api.Variants[1].StringValue; got != "eu-west" {
			t.Fatalf("StringValue = %q, want eu-west", got)
		}
	})

	t.Run("the declared value is unquoted", func(t *testing.T) {
		t.Parallel()
		// EnumVariant.Value is go/types' ExactString, so a string
		// constant arrives quoted. Passing it through raw renders
		// `return "\"us-east\""` — compilable and wrong.
		got := apiFor(t, "string", stringVariants).Variants[0].StringValue
		if strings.Contains(got, `"`) {
			t.Fatalf("StringValue = %q still carries its source quotes", got)
		}
	})

	t.Run("an explicit +gen:value still wins over the declared value", func(t *testing.T) {
		t.Parallel()
		api := apiFor(t, "string", func(eb *storefixture.EnumBuilder) {
			eb.Variant("US", `"us-east"`)
			pending := eb.Node().Variants[0]
			pending.DirectiveList = append(pending.DirectiveList, &directive.Directive{
				Name: "value", Args: []string{"americas"},
			})
		})
		if got := api.Variants[0].StringValue; got != "americas" {
			t.Fatalf("StringValue = %q, want the explicit override americas", got)
		}
	})

	t.Run("an unquotable value falls back to the identifier", func(t *testing.T) {
		t.Parallel()
		// A frontend that records something other than a Go literal
		// must not produce a broken string literal in the output.
		api := apiFor(t, "string", func(eb *storefixture.EnumBuilder) {
			eb.Variant("US", "not-a-quoted-literal")
		})
		if got := api.Variants[0].StringValue; got != "US" {
			t.Fatalf("StringValue = %q, want the identifier US", got)
		}
	})

	t.Run("a numeric enum keeps the identifier as its textual form", func(t *testing.T) {
		t.Parallel()
		// Every variant has a Value; for a numeric enum it is `1`,
		// and rendering String() as "1" would be worse than the name.
		api := apiFor(t, "int", func(eb *storefixture.EnumBuilder) {
			eb.Variant("RegionNorth", "0")
		})
		if got := api.Underlying; got != "int" {
			t.Fatalf("API.Underlying = %q, want int", got)
		}
		if got := api.Variants[0].StringValue; got != "North" {
			t.Fatalf("StringValue = %q, want the prefix-stripped identifier North", got)
		}
	})

	t.Run("a rune-valued enum keeps the identifier", func(t *testing.T) {
		t.Parallel()
		// The gate is on the underlying type, not on whether the
		// value happens to unquote. An integer never unquotes, so
		// that case falls through either way — but a rune constant
		// records `'a'`, which strconv.Unquote accepts, and without
		// the gate String() would silently become "a" instead of the
		// identifier.
		api := apiFor(t, "rune", func(eb *storefixture.EnumBuilder) {
			eb.Variant("RegionA", "'a'")
		})
		if got := api.Variants[0].StringValue; got != "A" {
			t.Fatalf("StringValue = %q, want the prefix-stripped identifier A", got)
		}
	})

	t.Run("an enum with no underlying type reports none", func(t *testing.T) {
		t.Parallel()
		// Frontends producing typeless enums leave Underlying nil;
		// the template then takes the numeric branch, as before.
		s := storefixture.New().
			Package("region", "example.com/region").
			Enum("Region", func(eb *storefixture.EnumBuilder) {
				eb.Pos(position.At("region/region.go", 1, 1))
				eb.Directive(storefixture.Directive(enumplugin.DirectiveName))
				eb.Variant("RegionNorth", "0")
			}).
			Build()
		d := diag.Capture()
		if err := enumplugin.New().Generate(&plugin.GeneratorContext{
			Store: s, Reader: store.NewReader(s), Diag: d,
		}); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		for _, slot := range s.Emit().PendingOriginSlots() {
			if api, ok := slot.Item.(*enumplugin.API); ok {
				if api.Underlying != "" {
					t.Fatalf("API.Underlying = %q, want empty", api.Underlying)
				}
				return
			}
		}
		t.Fatalf("plugin queued no API contribution")
	})
}

// renderAPITemplate executes the `enum.api` template against api and
// returns the rendered Go source.
//
// The funcmap entries the template reaches — `renderExpr` and
// `external` — are supplied by the Go backend, which a plugin package
// cannot import. They are stubbed here to something syntactically
// inert: this test is about which branch the template takes, and the
// backend's own tests own what those entries render.
func renderAPITemplate(t *testing.T, api *enumplugin.API) string {
	t.Helper()
	tmplFS, ok := enumplugin.GoTemplates()
	if !ok {
		t.Fatalf("plugin exposes no Go template tree")
	}
	tmpl, err := template.New("enum").Funcs(template.FuncMap{
		"external":   func(pkg, name string) string { return pkg + "." + name },
		"renderExpr": func(v any) string { return fmt.Sprint(v) },
	}).ParseFS(tmplFS, "*.tmpl")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "enum.api", api); err != nil {
		t.Fatalf("execute enum.api: %v", err)
	}
	return buf.String()
}

// TestAPITemplate_FallbackConversion pins the `String` fallback the
// template emits per underlying type.
//
// This is the defect that reached the consumer's build: `int(v)` was
// unconditional, so every generated file for a string-valued enum
// failed to compile with "cannot convert v (variable of string type
// Region) to type int". The data-side tests above cannot see it —
// the conversion is chosen in the template, not in the emit value.
func TestAPITemplate_FallbackConversion(t *testing.T) {
	t.Parallel()

	api := func(underlying string) *enumplugin.API {
		return &enumplugin.API{
			TypeName:     "Region",
			ParseName:    "ParseRegion",
			SentinelName: "ErrUnknownRegion",
			Underlying:   underlying,
			Variants: []enumplugin.Variant{
				{ConstName: "US", StringValue: "us-east"},
			},
		}
	}

	t.Run("a string enum converts with string(v)", func(t *testing.T) {
		t.Parallel()
		got := renderAPITemplate(t, api("string"))
		if !strings.Contains(got, "return string(v)") {
			t.Fatalf("string enum must fall back through string(v); got:\n%s", got)
		}
	})

	t.Run("a string enum never emits the numeric conversion", func(t *testing.T) {
		t.Parallel()
		// The exact expression that did not compile.
		got := renderAPITemplate(t, api("string"))
		if strings.Contains(got, "int(v)") {
			t.Fatalf("string enum must not emit int(v); got:\n%s", got)
		}
	})

	t.Run("a numeric enum keeps the Sprintf fallback", func(t *testing.T) {
		t.Parallel()
		got := renderAPITemplate(t, api("int"))
		if !strings.Contains(got, "int(v)") {
			t.Fatalf("numeric enum must keep int(v); got:\n%s", got)
		}
		if !strings.Contains(got, `"Region(%d)"`) {
			t.Fatalf("numeric enum must keep the Region(%%d) form; got:\n%s", got)
		}
	})

	t.Run("an enum with no underlying type takes the numeric branch", func(t *testing.T) {
		t.Parallel()
		// Historical behaviour for typeless enums; the only form a Go
		// const group without an explicit type can have.
		got := renderAPITemplate(t, api(""))
		if !strings.Contains(got, "int(v)") {
			t.Fatalf("typeless enum must take the numeric branch; got:\n%s", got)
		}
	})

	t.Run("the declared value reaches the switch arm", func(t *testing.T) {
		t.Parallel()
		got := renderAPITemplate(t, api("string"))
		if !strings.Contains(got, `case US:`) || !strings.Contains(got, `return "us-east"`) {
			t.Fatalf("switch arm should map US to its declared value; got:\n%s", got)
		}
	})

	t.Run("both branches parse as Go", func(t *testing.T) {
		t.Parallel()
		// A branch that renders but does not parse is the same class
		// of failure as the original bug, one step earlier.
		for _, u := range []string{"string", "int", ""} {
			src := "package p\n\ntype Region string\n\nconst US Region = \"us-east\"\n\n" +
				renderAPITemplate(t, api(u))
			if _, err := parser.ParseFile(token.NewFileSet(), "p.go", src, 0); err != nil {
				t.Fatalf("underlying %q rendered unparseable Go: %v\n%s", u, err, src)
			}
		}
	})
}
