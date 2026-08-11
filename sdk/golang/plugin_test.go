// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"embed"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"
	"text/template"

	langgo "go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/sdk"
	sdkgo "go.thesmos.sh/eidos/sdk/golang"
)

//go:embed testdata/templates/golang/*.tmpl
var goTemplates embed.FS

// out is the output set a generator with a primary and a companion
// declares.
func out() []sdk.Output {
	return []sdk.Output{{Suffix: ".gen.go"}, {Tag: "test", Suffix: "_test.gen.go"}}
}

// built returns a fully declared base, the shape a real plugin's New
// produces.
// fixtureDir is where the embedded test tree keeps its templates. A
// real plugin's tree is rooted at its own package, so its templates
// sit at [sdkgo.DefaultTemplateDir]; a test fixture has to live under
// testdata, which is one directory deeper.
const fixtureDir = "testdata/" + sdkgo.DefaultTemplateDir

func built() *sdkgo.Base {
	return sdkgo.NewGenerator("stub", goTemplates, out()...).
		TemplateDir(fixtureDir).
		Version("1.0.0").
		Priority(sdk.GeneratorFoundation).
		Provides("stub").
		Requires("shape").
		Directives(sdk.NewDirective("stub").On(node.KindInterface).Build()).
		Build()
}

// mustPanic runs fn and returns the panic message, failing when it
// did not panic.
func mustPanic(t *testing.T, fn func()) string {
	t.Helper()
	var msg string
	func() {
		defer func() {
			if r := recover(); r != nil {
				msg, _ = r.(string)
			}
		}()
		fn()
		t.Fatalf("Build did not panic")
	}()
	return msg
}

func TestBaseSatisfiesExactly(t *testing.T) {
	t.Parallel()

	// Pins the provider set a plugin gets for free. Embedding the base
	// means adding a method to any of these interfaces no longer stops
	// a plugin compiling, so this is what fails loudly instead — the
	// one guard standing between a shared base and a silent default
	// nobody chose. See the package docs.
	var b any = &sdkgo.Base{}

	t.Run("satisfies every declaration provider", func(t *testing.T) {
		t.Parallel()
		if _, ok := b.(sdk.Plugin); !ok {
			t.Error("Base does not satisfy sdk.Plugin")
		}
		if _, ok := b.(sdk.Versioned); !ok {
			t.Error("Base does not satisfy sdk.Versioned")
		}
		if _, ok := b.(sdk.CapabilityProvider); !ok {
			t.Error("Base does not satisfy sdk.CapabilityProvider")
		}
		if _, ok := b.(sdk.DirectiveProvider); !ok {
			t.Error("Base does not satisfy sdk.DirectiveProvider")
		}
		if _, ok := b.(sdk.FilenameProvider); !ok {
			t.Error("Base does not satisfy sdk.FilenameProvider")
		}
		if _, ok := b.(sdk.TemplateProvider); !ok {
			t.Error("Base does not satisfy sdk.TemplateProvider")
		}
	})

	t.Run("answers no behaviour the plugin owns", func(t *testing.T) {
		t.Parallel()
		// A plugin embedding the base still writes the whole of what
		// makes it that plugin. A base that satisfied Generator would
		// let one ship with no generation pass and no compile error.
		if _, ok := b.(sdk.Generator); ok {
			t.Error("Base satisfies sdk.Generator; generation is the plugin's own")
		}
		if _, ok := b.(sdk.Annotator); ok {
			t.Error("Base satisfies sdk.Annotator; annotation is the plugin's own")
		}
		if _, ok := b.(sdk.OptionsProvider); ok {
			t.Error("Base satisfies sdk.OptionsProvider; options bind to the plugin's struct")
		}
	})
}

func TestBaseDeclarations(t *testing.T) {
	t.Parallel()

	b := built()

	t.Run("reports what the builder declared", func(t *testing.T) {
		t.Parallel()
		if b.Name() != "stub" || b.Version() != "1.0.0" {
			t.Fatalf("Name/Version = %q/%q", b.Name(), b.Version())
		}
		if b.Priority() != sdk.GeneratorFoundation {
			t.Fatalf("Priority = %v", b.Priority())
		}
		if !slices.Equal(b.Provides(), []string{"stub"}) {
			t.Fatalf("Provides = %v", b.Provides())
		}
		if !slices.Equal(b.Requires(), []string{"shape"}) {
			t.Fatalf("Requires = %v", b.Requires())
		}
		if len(b.Directives()) != 1 {
			t.Fatalf("Directives = %d, want 1", len(b.Directives()))
		}
	})

	t.Run("an unset priority is the bucket a plugin already occupies", func(t *testing.T) {
		t.Parallel()
		// Not a guess: DefaultPriority is documented as where a plugin
		// implementing no capability sits, so the base answers what its
		// absence already meant.
		if got := sdkgo.NewPlugin("x").Build().Priority(); got != sdk.DefaultPriority {
			t.Fatalf("Priority = %v, want DefaultPriority", got)
		}
	})

	t.Run("unset declarations are nil rather than empty", func(t *testing.T) {
		t.Parallel()
		bare := sdkgo.NewPlugin("x").Build()
		if bare.Provides() != nil || bare.Requires() != nil || bare.Directives() != nil {
			t.Fatalf("bare base = %v/%v/%v",
				bare.Provides(), bare.Requires(), bare.Directives())
		}
	})

	t.Run("the caller cannot rewrite the declaration", func(t *testing.T) {
		t.Parallel()
		// Every accessor hands out a declaration the framework may sort
		// or filter; the first caller to do it in place would rewrite
		// what every later one sees.
		got := b.Provides()
		got[0] = "mutated"
		if b.Provides()[0] != "stub" {
			t.Fatalf("Provides = %v, want the declaration intact", b.Provides())
		}
	})
}

func TestBaseLanguageDispatch(t *testing.T) {
	t.Parallel()

	b := built()

	t.Run("answers for Go", func(t *testing.T) {
		t.Parallel()
		if got := b.Outputs(langgo.Language); len(got) != 2 || got[0].Suffix != ".gen.go" {
			t.Fatalf("Outputs = %+v", got)
		}
		tree, ok := b.Templates(langgo.Language)
		if !ok || tree == nil {
			t.Fatalf("Templates = %v, %v", tree, ok)
		}
		if len(b.TemplateFuncs(langgo.Language)) == 0 {
			t.Fatalf("TemplateFuncs is empty")
		}
	})

	t.Run("returns nil for another language", func(t *testing.T) {
		t.Parallel()
		// Nil rather than empty is what makes Layout report a missing
		// provider instead of composing Go-shaped filenames for a
		// backend that is not Go.
		if got := b.Outputs("rust"); got != nil {
			t.Fatalf("Outputs(rust) = %+v, want nil", got)
		}
		if tree, ok := b.Templates("rust"); ok || tree != nil {
			t.Fatalf("Templates(rust) = %v, %v", tree, ok)
		}
		if got := b.TemplateFuncs("rust"); got != nil {
			t.Fatalf("TemplateFuncs(rust) = %v, want nil", got)
		}
		if got := b.TemplateOverrides("rust"); got != nil {
			t.Fatalf("TemplateOverrides(rust) = %v, want nil", got)
		}
	})

	t.Run("a plugin shipping no templates reports so", func(t *testing.T) {
		t.Parallel()
		if tree, ok := sdkgo.NewPlugin("x").Build().Templates(langgo.Language); ok {
			t.Fatalf("Templates = %v, %v, want no tree", tree, ok)
		}
	})

	t.Run("the tree is rooted at the template directory", func(t *testing.T) {
		t.Parallel()
		// The backend registers templates by base filename, so the
		// directory must already be stripped.
		tree, _ := b.Templates(langgo.Language)
		if _, err := fs.Stat(tree, "stub.tmpl"); err != nil {
			t.Fatalf("stub.tmpl not at the tree root: %v", err)
		}
	})

	t.Run("the default directory is the convention plugins follow", func(t *testing.T) {
		t.Parallel()
		// The rooting mechanism is one line shared with the override
		// above; what the default has to get right is the value, which
		// is a convention between plugins rather than a choice any one
		// of them makes.
		if sdkgo.DefaultTemplateDir != "templates/golang" {
			t.Fatalf("DefaultTemplateDir = %q", sdkgo.DefaultTemplateDir)
		}
		if got := sdkgo.NewPlugin("x").Templates(goTemplates).Build(); got == nil {
			t.Fatalf("Build with the default directory returned nil")
		}
	})

	t.Run("an overridden directory is honoured", func(t *testing.T) {
		t.Parallel()
		tree, ok := sdkgo.NewPlugin("x").
			Templates(goTemplates).
			TemplateDir("testdata").
			Build().
			Templates(langgo.Language)
		if !ok {
			t.Fatalf("Templates reported no tree")
		}
		if _, err := fs.Stat(tree, "templates/golang/stub.tmpl"); err != nil {
			t.Fatalf("tree not rooted at testdata: %v", err)
		}
	})

	t.Run("the caller cannot rewrite the output set", func(t *testing.T) {
		t.Parallel()
		// Layout composes the primary filename from index 0.
		got := b.Outputs(langgo.Language)
		got[0].Suffix = ".mutated"
		if b.Outputs(langgo.Language)[0].Suffix != ".gen.go" {
			t.Fatalf("Outputs = %+v, want the declaration intact", b.Outputs(langgo.Language))
		}
	})
}

func TestBaseTemplateFuncs(t *testing.T) {
	t.Parallel()

	t.Run("the shared bundle carries the plugin's prefix", func(t *testing.T) {
		t.Parallel()
		// The backend rejects two plugins registering the same extension
		// name, so an unprefixed bundle fails every run rather than one
		// output.
		got := built().TemplateFuncs(langgo.Language)
		if _, ok := got["stub_args"]; !ok {
			t.Fatalf("funcs = %v, want a stub_-prefixed bundle", slices.Sorted(maps.Keys(got)))
		}
	})

	t.Run("a plugin's own helpers are prefixed identically", func(t *testing.T) {
		t.Parallel()
		// An author never writes the prefix, so it can never disagree
		// with the one the backend attributes their templates to.
		got := sdkgo.NewPlugin("stub").
			Funcs(template.FuncMap{"fields": func() string { return "" }}).
			Build().
			TemplateFuncs(langgo.Language)
		if _, ok := got["stub_fields"]; !ok {
			t.Fatalf("funcs = %v, want stub_fields", slices.Sorted(maps.Keys(got)))
		}
	})

	t.Run("a plugin's helper replaces a shared one of the same name", func(t *testing.T) {
		t.Parallel()
		// Specialising one helper should not cost the other eleven.
		got := sdkgo.NewPlugin("stub").
			Funcs(template.FuncMap{"args": func() string { return "mine" }}).
			Build().
			TemplateFuncs(langgo.Language)
		fn, ok := got["stub_args"].(func() string)
		if !ok || fn() != "mine" {
			t.Fatalf("stub_args = %T, want the plugin's own", got["stub_args"])
		}
		if _, still := got["stub_locals"]; !still {
			t.Fatalf("replacing one helper dropped the rest")
		}
	})

	// text/template panics inside Funcs on a name that is not a Go
	// identifier, and the backend calls Funcs for every plugin that
	// ships templates. A hyphen in a plugin name is ordinary —
	// `debug-weaver`, `if-match`, `leader-election` — so composing the
	// prefix verbatim made every one of them abort the whole render.
	t.Run("a name text/template would reject is folded to an identifier", func(t *testing.T) {
		t.Parallel()
		got := sdkgo.NewPlugin("debug-weaver").Build().TemplateFuncs(langgo.Language)
		if _, ok := got["debug_weaver_args"]; !ok {
			t.Fatalf("funcs = %v, want a debug_weaver_-prefixed bundle",
				slices.Sorted(maps.Keys(got)))
		}
		// The claim is not about spelling: it is that the backend can
		// register the bundle at all.
		if _, err := template.New("probe").Funcs(got).Parse(""); err != nil {
			t.Fatalf("registering the bundle: %v", err)
		}
	})

	t.Run("overrides are not prefixed", func(t *testing.T) {
		t.Parallel()
		// An override is identified by the builtin it stands in for.
		got := sdkgo.NewPlugin("stub").
			Overrides(template.FuncMap{"renderType": func() string { return "" }}).
			Build().
			TemplateOverrides(langgo.Language)
		if _, ok := got["renderType"]; !ok {
			t.Fatalf("overrides = %v, want the builtin's own name", slices.Sorted(maps.Keys(got)))
		}
	})

	t.Run("a plugin overriding nothing returns nil", func(t *testing.T) {
		t.Parallel()
		// The method exists because the capability is all-or-nothing,
		// not because every plugin has something to say.
		if got := built().TemplateOverrides(langgo.Language); got != nil {
			t.Fatalf("TemplateOverrides = %v, want nil", got)
		}
	})

	t.Run("the caller cannot rewrite the bundle", func(t *testing.T) {
		t.Parallel()
		b := built()
		got := b.TemplateFuncs(langgo.Language)
		delete(got, "stub_args")
		if _, still := b.TemplateFuncs(langgo.Language)["stub_args"]; !still {
			t.Fatalf("TemplateFuncs handed out its own map")
		}
	})
}

func TestBuildRefusesAMalformedDeclaration(t *testing.T) {
	t.Parallel()

	t.Run("an empty name", func(t *testing.T) {
		t.Parallel()
		// The pipeline keys registration, provenance and directive
		// scoping on it.
		msg := mustPanic(t, func() { sdkgo.NewPlugin("").Build() })
		if !strings.Contains(msg, "name is empty") {
			t.Fatalf("panic = %q", msg)
		}
	})

	t.Run("outputs with no template tree", func(t *testing.T) {
		t.Parallel()
		// The backend resolves a template per emit kind and would find
		// none — the failure roadmap item 5 catches one layer later.
		msg := mustPanic(t, func() {
			sdkgo.NewPlugin("stub").Outputs(sdk.Output{Suffix: ".gen.go"}).Build()
		})
		if !strings.Contains(msg, "no template tree") {
			t.Fatalf("panic = %q", msg)
		}
	})

	t.Run("an output with no suffix", func(t *testing.T) {
		t.Parallel()
		// Layout composes every filename from one.
		msg := mustPanic(t, func() {
			sdkgo.NewGenerator("stub", goTemplates, sdk.Output{Tag: "test"}).Build()
		})
		if !strings.Contains(msg, "no suffix") {
			t.Fatalf("panic = %q", msg)
		}
	})

	t.Run("a duplicate output tag", func(t *testing.T) {
		t.Parallel()
		// Directive scoping and CLI overrides address an output by tag,
		// so two of a name makes one of them unaddressable.
		msg := mustPanic(t, func() {
			sdkgo.NewGenerator("stub", goTemplates,
				sdk.Output{Tag: "test", Suffix: "_a.go"},
				sdk.Output{Tag: "test", Suffix: "_b.go"},
			).Build()
		})
		if !strings.Contains(msg, "twice") {
			t.Fatalf("panic = %q", msg)
		}
	})

	t.Run("a template tree with no outputs is legal", func(t *testing.T) {
		t.Parallel()
		// A cross-cutting generator renders into other plugins' files
		// and declares no file of its own.
		if got := sdkgo.NewPlugin("audit").Templates(goTemplates).Build(); got == nil {
			t.Fatalf("Build returned nil")
		}
	})

	t.Run("a tree declared alongside BuiltinTemplates", func(t *testing.T) {
		t.Parallel()
		// The two say opposite things about whether a tree exists, and
		// silently honouring one would register templates the plugin
		// declared it had none of.
		msg := mustPanic(t, func() {
			sdkgo.NewGenerator("stub", goTemplates, out()...).BuiltinTemplates().Build()
		})
		if !strings.Contains(msg, "BuiltinTemplates") {
			t.Fatalf("panic = %q", msg)
		}
	})
}

func TestBuiltinTemplatesServesATemplateFreeGenerator(t *testing.T) {
	t.Parallel()

	// The shape mockgen and repogen have: they own a file, and every
	// decl they emit is a standard one the backend already renders. A
	// template tree would have nothing in it.
	build := func() *sdkgo.Base {
		return sdkgo.NewPlugin("mock").
			Outputs(sdk.Output{Suffix: "_mock_test.go"}).
			BuiltinTemplates().
			Build()
	}

	t.Run("outputs without a tree are accepted", func(t *testing.T) {
		t.Parallel()
		got := build()
		if len(got.Outputs(langgo.Language)) != 1 {
			t.Fatalf("Outputs = %v", got.Outputs(langgo.Language))
		}
	})

	t.Run("it reports shipping no templates", func(t *testing.T) {
		t.Parallel()
		// The supported answer for "no templates for this language",
		// and what the backend keys its per-plugin parse on.
		if _, ok := build().Templates(langgo.Language); ok {
			t.Fatalf("Templates reported a tree for a plugin declaring none")
		}
	})

	t.Run("no helper bundle is registered", func(t *testing.T) {
		t.Parallel()
		// A plugin's helpers bind only to its own templates at parse
		// time, so a bundle without a tree is unreachable by
		// construction — registering one would put sixty names into the
		// backend's registry that nothing can ever call.
		if got := build().TemplateFuncs(langgo.Language); len(got) != 0 {
			t.Fatalf("TemplateFuncs registered %d entries for a template-free plugin", len(got))
		}
	})
}

func TestBuilderFreezesTheDeclaration(t *testing.T) {
	t.Parallel()

	t.Run("a builder reused after Build does not reach the base", func(t *testing.T) {
		t.Parallel()
		// Two types rather than one: a single mutable type would leave
		// a plugin's outputs writable for the life of the process, from
		// any goroutine holding the plugin.
		b := sdkgo.NewGenerator("stub", goTemplates, out()...)
		frozen := b.Build()
		b.Provides("late").Version("9.9.9")
		if frozen.Version() != "" || len(frozen.Provides()) != 0 {
			t.Fatalf("the frozen base saw a later declaration: %q/%v",
				frozen.Version(), frozen.Provides())
		}
	})

	t.Run("the source slice cannot rewrite the declaration", func(t *testing.T) {
		t.Parallel()
		caps := []string{"stub"}
		frozen := sdkgo.NewPlugin("stub").Provides(caps...).Build()
		caps[0] = "mutated"
		if frozen.Provides()[0] != "stub" {
			t.Fatalf("Provides = %v, want the value at declaration time", frozen.Provides())
		}
	})
}

// TestLanguageMatchesTheAdapter pins the re-export against the
// language package. A plugin comparing its dispatch argument to a
// constant that has drifted returns nothing for every call, emits no
// output, and reports no error — the silent failure the constant
// exists to prevent.
func TestLanguageMatchesTheAdapter(t *testing.T) {
	t.Parallel()

	t.Run("the re-export is the adapter's identifier", func(t *testing.T) {
		t.Parallel()
		if sdkgo.Language != langgo.Language {
			t.Errorf("sdkgo.Language = %q, want %q", sdkgo.Language, langgo.Language)
		}
	})

	t.Run("Base answers the language it names", func(t *testing.T) {
		t.Parallel()
		// The constant is only useful if the builder dispatches on the
		// same string a hand-written plugin would compare against.
		base := sdkgo.NewGenerator("langcheck", goTemplates,
			sdk.Output{Suffix: ".gen.go"}).Build()
		if got := base.Outputs(sdkgo.Language); len(got) == 0 {
			t.Errorf("Outputs(%q) returned nothing", sdkgo.Language)
		}
	})
}
