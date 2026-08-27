// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/sdk"
)

// treeAt returns a filesystem holding one template at dir/name, the
// shape a plugin's //go:embed tree has.
func treeAt(dir, name string) fs.FS {
	return fstest.MapFS{dir + "/" + name: &fstest.MapFile{Data: []byte("x")}}
}

// assertPanics fails when fn does not panic, reporting what it
// returned instead.
//
// The builder reports a plugin's own construction mistakes by
// panicking rather than by returning an error, so these are the only
// tests that can observe them at all.
func assertPanics(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("no panic; want one mentioning %q", want)
		}
		if msg, ok := r.(string); ok && want != "" && !strings.Contains(msg, want) {
			t.Fatalf("panic = %q, want it to mention %q", msg, want)
		}
	}()
	fn()
}

// TestBuilder_NeutralDeclarations covers the six methods that carry
// no language, which is every plugin that contributes only into
// other plugins' slots.
func TestBuilder_NeutralDeclarations(t *testing.T) {
	t.Parallel()

	schema := sdk.NewDirective("gen:thing").Build()
	base := sdk.NewPlugin("weaver").
		Version("1.2.3").
		Priority(sdk.GeneratorCrossCutting).
		Provides("cap.a").
		Requires("cap.b").
		Directives(schema).
		Build()

	t.Run("every declaration is answered", func(t *testing.T) {
		t.Parallel()
		if base.Name() != "weaver" || base.Version() != "1.2.3" {
			t.Fatalf("Name/Version = %q/%q", base.Name(), base.Version())
		}
		if base.Priority() != sdk.GeneratorCrossCutting {
			t.Errorf("Priority = %v", base.Priority())
		}
		if got := base.Provides(); !slices.Equal(got, []string{"cap.a"}) {
			t.Errorf("Provides = %v", got)
		}
		if got := base.Requires(); !slices.Equal(got, []string{"cap.b"}) {
			t.Errorf("Requires = %v", got)
		}
		if got := base.Directives(); len(got) != 1 {
			t.Errorf("Directives = %v", got)
		}
	})

	t.Run("a plugin declaring no language answers nothing for one", func(t *testing.T) {
		t.Parallel()
		// The shape a slot contributor has: composition happens at the
		// emit layer, where there is no language yet, so it is
		// language-agnostic by construction rather than by gating.
		if got := base.Outputs("golang"); got != nil {
			t.Errorf("Outputs = %v, want nil", got)
		}
		if _, ok := base.Templates("golang"); ok {
			t.Error("Templates reported a tree for an undeclared language")
		}
		if got := base.Languages(); len(got) != 0 {
			t.Errorf("Languages = %v, want none", got)
		}
	})

	t.Run("slice accessors hand back copies", func(t *testing.T) {
		t.Parallel()
		// The pipeline's render pool reads these concurrently; a
		// caller mutating the answer would race every other reader.
		got := base.Provides()
		got[0] = "mutated"
		if base.Provides()[0] != "cap.a" {
			t.Error("Provides exposed its backing array")
		}
	})
}

// TestBuilder_PerLanguage covers the declarations that vary by
// language — the reason LanguageSupport is a bundle rather than
// fields on the builder.
func TestBuilder_PerLanguage(t *testing.T) {
	t.Parallel()

	goTree := treeAt("templates/golang", "thing.tmpl")
	rsTree := treeAt("templates/rust", "thing.tmpl")
	base := sdk.NewPlugin("dual").
		For("golang", sdk.LanguageSupport{
			Templates: goTree,
			Outputs:   []sdk.Output{{Suffix: "_thing.go"}},
		}).
		For("rust", sdk.LanguageSupport{
			Templates: rsTree,
			Outputs:   []sdk.Output{{Suffix: "_thing.rs"}, {Tag: "mod", Suffix: "_mod.rs"}},
		}).
		Build()

	t.Run("each language answers with its own outputs", func(t *testing.T) {
		t.Parallel()
		if got := base.Outputs("golang"); len(got) != 1 || got[0].Suffix != "_thing.go" {
			t.Errorf("go outputs = %v", got)
		}
		if got := base.Outputs("rust"); len(got) != 2 || got[1].Tag != "mod" {
			t.Errorf("rust outputs = %v", got)
		}
	})

	t.Run("a language the plugin does not target answers nothing", func(t *testing.T) {
		t.Parallel()
		// The backend asks every plugin about its own language; one
		// that does not speak it must be distinguishable from one
		// whose tree failed to embed.
		if got := base.Outputs("typescript"); got != nil {
			t.Errorf("Outputs = %v, want nil", got)
		}
		if _, ok := base.Templates("typescript"); ok {
			t.Error("Templates reported a tree for an untargeted language")
		}
	})

	t.Run("each language resolves its own template subtree", func(t *testing.T) {
		t.Parallel()
		for _, lang := range []string{"golang", "rust"} {
			tree, ok := base.Templates(lang)
			if !ok {
				t.Fatalf("%s: no tree", lang)
			}
			if _, err := fs.Stat(tree, "thing.tmpl"); err != nil {
				t.Errorf("%s: template not at the subtree root: %v", lang, err)
			}
		}
	})

	t.Run("Languages reports what was declared, sorted", func(t *testing.T) {
		t.Parallel()
		// Sorted rather than in declaration order so the answer does
		// not depend on map iteration and two runs agree.
		if got := base.Languages(); !slices.Equal(got, []string{"golang", "rust"}) {
			t.Errorf("Languages = %v", got)
		}
	})

	t.Run("re-declaring a language replaces it", func(t *testing.T) {
		t.Parallel()
		b := sdk.NewPlugin("p").
			For("golang", sdk.LanguageSupport{Outputs: []sdk.Output{{Suffix: ".a"}}, Builtin: true}).
			For("golang", sdk.LanguageSupport{Outputs: []sdk.Output{{Suffix: ".b"}}, Builtin: true}).
			Build()
		if got := b.Outputs("golang"); len(got) != 1 || got[0].Suffix != ".b" {
			t.Errorf("Outputs = %v, want only the later declaration", got)
		}
		if got := b.Languages(); len(got) != 1 {
			t.Errorf("Languages = %v, want one entry", got)
		}
	})
}

// TestBuilder_TemplateDir covers the subtree the tree is rooted at.
func TestBuilder_TemplateDir(t *testing.T) {
	t.Parallel()

	t.Run("defaults to templates/<lang>", func(t *testing.T) {
		t.Parallel()
		if got := sdk.TemplateDirFor("rust"); got != "templates/rust" {
			t.Fatalf("TemplateDirFor = %q", got)
		}
	})

	t.Run("an explicit dir overrides the convention", func(t *testing.T) {
		t.Parallel()
		base := sdk.NewPlugin("p").For("golang", sdk.LanguageSupport{
			Templates:   treeAt("tpl", "a.tmpl"),
			TemplateDir: "tpl",
		}).Build()
		tree, ok := base.Templates("golang")
		if !ok {
			t.Fatal("no tree")
		}
		if _, err := fs.Stat(tree, "a.tmpl"); err != nil {
			t.Errorf("override ignored: %v", err)
		}
	})

	t.Run("a directory absent from the tree yields an empty tree, not a failure", func(t *testing.T) {
		t.Parallel()
		// The backend reports this as a plugin shipping no templates.
		// Failing here instead would turn a mis-specified embed into a
		// panic inside a constructor, where the //go:embed directive
		// has already validated the tree it was given.
		base := sdk.NewPlugin("p").For("golang", sdk.LanguageSupport{
			Templates:   treeAt("templates/golang", "a.tmpl"),
			TemplateDir: "nowhere",
		}).Build()
		tree, ok := base.Templates("golang")
		if !ok {
			t.Fatal("expected a tree value")
		}
		if _, err := fs.Stat(tree, "a.tmpl"); err == nil {
			t.Error("the wrong subtree resolved")
		}
	})
}

// TestBuilder_Funcs covers funcmap composition, which is where a
// language contributes its own bundle and a plugin's helpers get
// their prefix.
func TestBuilder_Funcs(t *testing.T) {
	t.Parallel()

	t.Run("a plugin's helpers register under the names it declared", func(t *testing.T) {
		t.Parallel()
		// A template calls a helper by the name its plugin wrote. An
		// earlier form mangled it with the plugin's name, so the
		// template had to spell a variant no declaration contained.
		base := sdk.NewPlugin("my-gen").For("golang", sdk.LanguageSupport{
			Templates: treeAt("templates/golang", "a.tmpl"),
			Funcs:     template.FuncMap{"defaultsExpr": func() string { return "" }},
		}).Build()
		got := base.TemplateFuncs("golang")
		if _, ok := got["defaultsExpr"]; !ok {
			t.Fatalf("funcs = %v, want a defaultsExpr entry", keys(got))
		}
	})

	t.Run("a builtin-rendering plugin registers no funcs", func(t *testing.T) {
		t.Parallel()
		// It renders through the backend's own kind templates, so a
		// bundle would be registered for templates that never call it.
		base := sdk.NewPlugin("p").For("golang", sdk.LanguageSupport{
			Outputs: []sdk.Output{{Suffix: ".go"}},
			Builtin: true,
			Funcs:   template.FuncMap{"x": func() {}},
		}).Build()
		if got := base.TemplateFuncs("golang"); len(got) != 0 {
			t.Errorf("funcs = %v, want none", keys(got))
		}
	})

	t.Run("overrides pass through untouched", func(t *testing.T) {
		t.Parallel()
		// Unprefixed by design: an override names a canonical backend
		// entry it replaces, so prefixing it would target nothing.
		base := sdk.NewPlugin("p").For("golang", sdk.LanguageSupport{
			Templates: treeAt("templates/golang", "a.tmpl"),
			Overrides: template.FuncMap{"renderType": func() string { return "" }},
		}).Build()
		if _, ok := base.TemplateOverrides("golang")["renderType"]; !ok {
			t.Error("override was renamed or dropped")
		}
	})

	t.Run("funcmap accessors hand back copies", func(t *testing.T) {
		t.Parallel()
		base := sdk.NewPlugin("p").For("golang", sdk.LanguageSupport{
			Templates: treeAt("templates/golang", "a.tmpl"),
			Funcs:     template.FuncMap{"a": func() {}},
		}).Build()
		base.TemplateFuncs("golang")["injected"] = func() {}
		if _, leaked := base.TemplateFuncs("golang")["injected"]; leaked {
			t.Error("TemplateFuncs exposed its backing map")
		}
	})
}

// TestBuilder_RejectsUnservableDeclarations covers the construction
// mistakes the builder refuses.
//
// Each is a mistake in a plugin's own New — not in its input — so it
// fires on the first construction in any test, before a run exists.
// The alternative is a pipeline that starts and emits nothing.
func TestBuilder_RejectsUnservableDeclarations(t *testing.T) {
	t.Parallel()

	tree := treeAt("templates/golang", "a.tmpl")

	t.Run("an empty plugin name", func(t *testing.T) {
		t.Parallel()
		assertPanics(t, "plugin name is empty", func() { sdk.NewPlugin("").Build() })
	})

	t.Run("outputs with no way to render them", func(t *testing.T) {
		t.Parallel()
		assertPanics(t, "no template tree", func() {
			sdk.NewPlugin("p").For("golang", sdk.LanguageSupport{
				Outputs: []sdk.Output{{Suffix: ".go"}},
			}).Build()
		})
	})

	t.Run("a tree and Builtin together", func(t *testing.T) {
		t.Parallel()
		assertPanics(t, "Builtin", func() {
			sdk.NewPlugin("p").For("golang", sdk.LanguageSupport{
				Templates: tree, Builtin: true,
			}).Build()
		})
	})

	t.Run("an output with no suffix", func(t *testing.T) {
		t.Parallel()
		assertPanics(t, "no suffix", func() {
			sdk.NewPlugin("p").For("golang", sdk.LanguageSupport{
				Templates: tree, Outputs: []sdk.Output{{Suffix: ""}},
			}).Build()
		})
	})

	t.Run("two outputs sharing a tag", func(t *testing.T) {
		t.Parallel()
		assertPanics(t, "twice", func() {
			sdk.NewPlugin("p").For("golang", sdk.LanguageSupport{
				Templates: tree,
				Outputs:   []sdk.Output{{Tag: "x", Suffix: ".a"}, {Tag: "x", Suffix: ".b"}},
			}).Build()
		})
	})

	t.Run("the same tag under two languages is legal", func(t *testing.T) {
		t.Parallel()
		// Tags scope to a language: `_thing.go` and `_thing.rs` are one
		// declaration about two targets, and rejecting the pair would
		// force a plugin to invent per-language tag names.
		sdk.NewPlugin("p").
			For("golang", sdk.LanguageSupport{Outputs: []sdk.Output{{Tag: "x", Suffix: ".go"}}, Builtin: true}).
			For("rust", sdk.LanguageSupport{Outputs: []sdk.Output{{Tag: "x", Suffix: ".rs"}}, Builtin: true}).
			Build()
	})
}

// keys returns a funcmap's names, for a readable failure message.
func keys(m template.FuncMap) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// TestLanguageReporter covers the warn-once helper eight plugins had
// each written for themselves.
//
// The failure it exists to prevent is invisible in the output: a run
// over a language nothing reads emits nothing for it and ends green,
// so the file is short rather than wrong and a reader has no line to
// notice.
func TestLanguageReporter(t *testing.T) {
	t.Parallel()

	pkg := func() *sdk.Package { return &sdk.Package{Name: "x", Path: "x"} }
	langs := []string{"golang"}

	t.Run("warns once per language across packages", func(t *testing.T) {
		t.Parallel()
		// Once per language rather than per package: one missing
		// registration is one thing to fix, and a warning per package
		// buries it in a corpus.
		var r sdk.LanguageReporter
		sink := diag.New()
		for range 3 {
			r.Report(sink, pkg(), "gen", "rust", "are not read", langs)
		}
		if got := sink.Diagnostics(); len(got) != 1 {
			t.Fatalf("diagnostics = %d, want 1: %+v", len(got), got)
		}
	})

	t.Run("a second language warns again", func(t *testing.T) {
		t.Parallel()
		var r sdk.LanguageReporter
		sink := diag.New()
		r.Report(sink, pkg(), "gen", "rust", "are not read", langs)
		r.Report(sink, pkg(), "gen", "python", "are not read", langs)
		if got := sink.Diagnostics(); len(got) != 2 {
			t.Fatalf("diagnostics = %d, want one per language", len(got))
		}
	})

	t.Run("an unmarked package is passed over in silence", func(t *testing.T) {
		t.Parallel()
		// The marker names the language a package was written in, so
		// its absence means nothing claimed it. Warning about those
		// would put a diagnostic on every unit test that builds a
		// store by hand, which is where the real warning would then
		// go unread.
		var r sdk.LanguageReporter
		sink := diag.New()
		r.Report(sink, pkg(), "gen", "", "are not read", langs)
		if got := sink.Diagnostics(); len(got) != 0 {
			t.Fatalf("reported %+v over an unmarked package", got)
		}
	})

	t.Run("the message carries the caller's clause and the plugin's languages", func(t *testing.T) {
		t.Parallel()
		// What a generator does not produce is its own to say; a
		// shared sentence would name no output, or one generator's.
		var r sdk.LanguageReporter
		sink := diag.New()
		r.Report(sink, pkg(), "builder", "rust",
			"are not read, so no builder is generated for them", langs)
		got := sink.Diagnostics()[0].Message
		for _, want := range []string{"builder", `"rust"`, "no builder is generated", "golang"} {
			if !strings.Contains(got, want) {
				t.Errorf("message %q does not carry %q", got, want)
			}
		}
	})

	t.Run("the zero value is usable", func(t *testing.T) {
		t.Parallel()
		// Declared as a local var by every caller, so a nil map has to
		// allocate on first use rather than panic.
		var r sdk.LanguageReporter
		sink := diag.New()
		r.Report(sink, pkg(), "gen", "rust", "are not read", langs)
		if len(sink.Diagnostics()) != 1 {
			t.Fatal("the zero value did not report")
		}
	})

	t.Run("a nil sink is survived", func(t *testing.T) {
		t.Parallel()
		var r sdk.LanguageReporter
		r.Report(nil, pkg(), "gen", "rust", "are not read", langs)
	})
}
