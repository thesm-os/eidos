// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend_test

import (
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang/backend"
	"go.thesmos.sh/eidos/lang/golang/backend/testdata/pluginfixture"
	"go.thesmos.sh/eidos/plugin"
)

// TestMerge_FuncCollision covers the funcmap-extension pass: a
// plugin registering a name that collides with another plugin's
// extension surfaces [backend.ErrTemplateFuncCollision] with both
// contributors named.
func TestMerge_FuncCollision(t *testing.T) {
	t.Parallel()

	t.Run("ErrTemplateFuncCollision is exported and satisfies errors.Is", func(t *testing.T) {
		t.Parallel()
		if backend.ErrTemplateFuncCollision == nil {
			t.Fatalf("ErrTemplateFuncCollision must be exported and non-nil")
		}
		if !errors.Is(backend.ErrTemplateFuncCollision, backend.ErrTemplateFuncCollision) {
			t.Fatalf("ErrTemplateFuncCollision must satisfy errors.Is reflexivity")
		}
	})

	t.Run("two plugins registering same extension name collide", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Plugins = []plugin.Plugin{
			&stubTemplateProvider{
				name:  "alpha",
				funcs: template.FuncMap{"shout": strings.ToUpper},
			},
			&stubTemplateProvider{
				name:  "beta",
				funcs: template.FuncMap{"shout": func(s string) string { return s + "!" }},
			},
		}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target, fieldSpec{name: "F", builtin: "int"},
		)))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("collision must suppress sink writes")
		}
		if !diagnosticsContain(d, diag.Error, "template func collision") {
			t.Fatalf("expected ErrTemplateFuncCollision diagnostic; got %+v", d.Diagnostics())
		}
		if !diagnosticsContain(d, diag.Error, "alpha") || !diagnosticsContain(d, diag.Error, "beta") {
			t.Fatalf("diagnostic should name both plugins; got %+v", d.Diagnostics())
		}
	})

	t.Run("extension that collides with core canonical entry surfaces collision", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Plugins = []plugin.Plugin{
			&stubTemplateProvider{
				name:  "intruder",
				funcs: template.FuncMap{"imp": func(string) (string, error) { return "", nil }},
			},
		}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target, fieldSpec{name: "F", builtin: "int"},
		)))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("collision must suppress sink writes")
		}
		if !diagnosticsContain(d, diag.Error, "template func collision") {
			t.Fatalf("expected ErrTemplateFuncCollision diagnostic; got %+v", d.Diagnostics())
		}
		if !diagnosticsContain(d, diag.Error, "imp") || !diagnosticsContain(d, diag.Error, "core") {
			t.Fatalf("diagnostic should name the core entry and the offending plugin; got %+v", d.Diagnostics())
		}
	})
}

// TestMerge_ReservedOverride covers the funcmap-override pass:
// targeting a reserved canonical entry surfaces
// [backend.ErrReservedFuncName] with the offending plugin and
// entry named.
func TestMerge_ReservedOverride(t *testing.T) {
	t.Parallel()

	t.Run("ErrReservedFuncName is exported and satisfies errors.Is", func(t *testing.T) {
		t.Parallel()
		if backend.ErrReservedFuncName == nil {
			t.Fatalf("ErrReservedFuncName must be exported and non-nil")
		}
		if !errors.Is(backend.ErrReservedFuncName, backend.ErrReservedFuncName) {
			t.Fatalf("ErrReservedFuncName must satisfy errors.Is reflexivity")
		}
	})

	t.Run("override targeting a reserved name fails Build", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		offender := &stubTemplateProvider{
			name:      "intruder",
			overrides: template.FuncMap{"render": func(emit.Node) (string, error) { return "", nil }},
		}
		ctx.Plugins = []plugin.Plugin{offender}
		ctx.Ordered = []plugin.Plugin{offender}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target, fieldSpec{name: "F", builtin: "int"},
		)))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("reserved-override must suppress sink writes")
		}
		if !diagnosticsContain(d, diag.Error, "reserved funcmap entry") {
			t.Fatalf("expected ErrReservedFuncName diagnostic; got %+v", d.Diagnostics())
		}
		if !diagnosticsContain(d, diag.Error, "intruder") || !diagnosticsContain(d, diag.Error, "render") {
			t.Fatalf("diagnostic should name plugin + entry; got %+v", d.Diagnostics())
		}
	})
}

// TestMerge_OverrideEmitsInfo covers the override-success path:
// when a plugin overrides a non-reserved name, an Info diagnostic
// of the form `plugin <id> overrode <name> (was: <previous>)` is
// emitted so the override trail is auditable.
func TestMerge_OverrideEmitsInfo(t *testing.T) {
	t.Parallel()

	t.Run("override on a non-reserved name emits Info diagnostic", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		// Stage an extension first so the override has a "previous
		// owner" to name. Then a second plugin overrides it.
		alpha := &stubTemplateProvider{
			name:  "alpha",
			funcs: template.FuncMap{"shout": strings.ToUpper},
		}
		beta := &stubTemplateProvider{
			name:      "beta",
			overrides: template.FuncMap{"shout": func(s string) string { return s + "!" }},
		}
		ctx.Plugins = []plugin.Plugin{alpha, beta}
		ctx.Ordered = []plugin.Plugin{alpha, beta}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target, fieldSpec{name: "F", builtin: "int"},
		)))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !diagnosticsContain(d, diag.Info, "plugin beta overrode shout (was: alpha)") {
			t.Fatalf("expected override Info diagnostic; got %+v", d.Diagnostics())
		}
		// And the override actually took effect — body still renders
		// successfully (no core templates use `shout`, but the
		// merge step must not break the render path).
		if !strings.Contains(string(body), "type X struct {") {
			t.Fatalf("expected struct decl in rendered body; got:\n%s", body)
		}
	})

	t.Run("first-time override of a non-reserved name names core as previous owner", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		// `shout` isn't a core entry, so an override on it should
		// still emit an Info — but the "previous owner" surface for
		// a never-registered name is "core" (no prior contributor).
		// Plugins overriding nothing-at-all is a weird use case;
		// the diagnostic still fires because the override succeeded.
		intruder := &stubTemplateProvider{
			name:      "intruder",
			overrides: template.FuncMap{"shout": func(s string) string { return s }},
		}
		ctx.Plugins = []plugin.Plugin{intruder}
		ctx.Ordered = []plugin.Plugin{intruder}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target, fieldSpec{name: "F", builtin: "int"},
		)))
		assertRenderSucceeds(t, ctx, mem, d, target)
		if !diagnosticsContain(d, diag.Info, "plugin intruder overrode shout (was: core)") {
			t.Fatalf("expected override Info naming core as previous owner; got %+v", d.Diagnostics())
		}
	})
}

// TestMerge_PluginTemplate covers the plugin-template parse path:
// a [plugin.TemplateProvider] ships a template under a custom
// kind name, an emit kind with that Kind() reaches the backend
// via the store, and the kind's template renders inline via the
// merged template tree.
func TestMerge_PluginTemplate(t *testing.T) {
	t.Parallel()

	t.Run("plugin-defined kind dispatches through the merged template", func(t *testing.T) {
		t.Parallel()
		// The cleanest plugin-template fixture is an override of an
		// existing kind template: we ship a template named
		// "emit.constant" via the plugin filesystem, and verify the
		// dispatcher uses it (replacing core's default). text/template's
		// Parse semantics define that the latest parse of a given
		// name wins, so the plugin template overrides core's
		// constant rendering.
		ctx, mem, d := newBackendContext(t)
		ctx.Plugins = []plugin.Plugin{&stubTemplateProvider{
			name: "constgen",
			tmplFS: fstest.MapFS{
				"templates/golang/constant.tmpl": &fstest.MapFile{
					Data: []byte(`{{ define "emit.constant" -}}
// plugin-rendered: {{ .Name }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`),
				},
			},
		}}
		ctx.Ordered = ctx.Plugins
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Constants: []*emit.Constant{{
				Name: "K", Package: "x", Target: target,
				Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "42"},
			}},
		})
		body := string(assertRenderSucceeds(t, ctx, mem, d, target))
		if !strings.Contains(body, "// plugin-rendered: K") {
			t.Fatalf("expected plugin-rendered constant marker; got:\n%s", body)
		}
		if !strings.Contains(body, "const K = 42") {
			t.Fatalf("expected const decl from plugin template; got:\n%s", body)
		}
		// Plugin-overrides-core templates emit an Info diagnostic
		// symmetric with the funcmap-override surface so the
		// override trail is auditable end-to-end.
		if !diagnosticsContain(d, diag.Info, "plugin constgen overrode template emit.constant (was: core)") {
			t.Fatalf("expected template-override Info diagnostic; got %+v", d.Diagnostics())
		}
	})
}

// TestMerge_PluginDefinedKindRoundTrip covers the end-to-end plugin
// extension story: a [pluginfixture.Plugin] ships a custom emit
// kind ([pluginfixture.Banner]) plus the matching template, the
// banner rides into a Target's [emit.File.Top] slot alongside a
// core Struct, and the backend renders both into one file.
//
// The acceptance assertion has three parts: the banner content
// appears in the body, the core Struct appears in the body, and a
// golden fixture pins the canonical byte-level output.
func TestMerge_PluginDefinedKindRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("plugin-defined kind renders alongside a core kind", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		fx := pluginfixture.Plugin{}
		ctx.Plugins = []plugin.Plugin{fx}
		ctx.Ordered = []plugin.Plugin{fx}
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		f := bindFile(t, ctx, target)
		banner := &pluginfixture.Banner{
			Title:   "USERS",
			Message: "User record + supporting types for the users package.",
			Target:  target,
		}
		if err := f.Top().Append(banner, emit.Provenance{SetBy: pluginfixture.Name}); err != nil {
			t.Fatalf("append banner to File.Top: %v", err)
		}
		addEmitPackage(t, ctx, emitPackage("users", &emit.Struct{
			BaseEmit: emit.BaseEmit{DocLines: []string{"User is the canonical user record."}},
			Name:     "User", Package: "users", Target: target,
			Fields: []*emit.Field{
				{Name: "ID", Type: emit.Builtin("int")},
				{Name: "Name", Type: emit.Builtin("string")},
			},
		}))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "// === USERS ===") {
			t.Fatalf("expected plugin-rendered banner header in body; got:\n%s", body)
		}
		if !strings.Contains(string(body), "// User record + supporting types") {
			t.Fatalf("expected plugin-rendered banner message in body; got:\n%s", body)
		}
		if !strings.Contains(string(body), "type User struct {") {
			t.Fatalf("expected core Struct decl in body; got:\n%s", body)
		}
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "plugin_kind_round_trip.go.golden"))
	})
}

// TestMerge_TemplateNameCollision covers the cross-plugin
// template-collision rule: two plugins both shipping a template
// under the same name surface [backend.ErrTemplateNameCollision]
// with both contributors named.
func TestMerge_TemplateNameCollision(t *testing.T) {
	t.Parallel()

	t.Run("ErrTemplateNameCollision is exported and satisfies errors.Is", func(t *testing.T) {
		t.Parallel()
		if backend.ErrTemplateNameCollision == nil {
			t.Fatalf("ErrTemplateNameCollision must be exported and non-nil")
		}
		if !errors.Is(backend.ErrTemplateNameCollision, backend.ErrTemplateNameCollision) {
			t.Fatalf("ErrTemplateNameCollision must satisfy errors.Is reflexivity")
		}
	})

	t.Run("two plugins defining the same template name collide", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		mk := func(name, body string) plugin.Plugin {
			return &stubTemplateProvider{
				name: name,
				tmplFS: fstest.MapFS{
					"templates/golang/custom.tmpl": &fstest.MapFile{
						Data: []byte(body),
					},
				},
			}
		}
		ctx.Plugins = []plugin.Plugin{
			mk("alpha", `{{ define "plugin.custom" }}from alpha{{ end }}`),
			mk("beta", `{{ define "plugin.custom" }}from beta{{ end }}`),
		}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target, fieldSpec{name: "F", builtin: "int"},
		)))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("template-name collision must suppress sink writes")
		}
		if !diagnosticsContain(d, diag.Error, "template name collision") {
			t.Fatalf("expected ErrTemplateNameCollision diagnostic; got %+v", d.Diagnostics())
		}
		if !diagnosticsContain(d, diag.Error, "alpha") || !diagnosticsContain(d, diag.Error, "beta") {
			t.Fatalf("diagnostic should name both plugins; got %+v", d.Diagnostics())
		}
	})
}

// TestMerge_ReservedTemplatePrefix covers the parse-time guard that
// rejects plugin-defined templates whose name starts with a
// reserved prefix (currently `fragment.`). Reserved prefixes back
// the backend's shared partials; plugin definitions there would
// silently shadow core helpers.
func TestMerge_ReservedTemplatePrefix(t *testing.T) {
	t.Parallel()

	t.Run("ErrReservedTemplatePrefix is exported and satisfies errors.Is", func(t *testing.T) {
		t.Parallel()
		if backend.ErrReservedTemplatePrefix == nil {
			t.Fatalf("ErrReservedTemplatePrefix must be exported and non-nil")
		}
		if !errors.Is(backend.ErrReservedTemplatePrefix, backend.ErrReservedTemplatePrefix) {
			t.Fatalf("ErrReservedTemplatePrefix must satisfy errors.Is reflexivity")
		}
	})

	t.Run("plugin defining a fragment.* template surfaces the sentinel", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Plugins = []plugin.Plugin{&stubTemplateProvider{
			name: "intruder",
			tmplFS: fstest.MapFS{
				"templates/golang/fragment.tmpl": &fstest.MapFile{
					Data: []byte(`{{ define "fragment.intruder" }}shadow{{ end }}`),
				},
			},
		}}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target, fieldSpec{name: "F", builtin: "int"},
		)))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("reserved-prefix violation must suppress sink writes")
		}
		if !diagnosticsContain(d, diag.Error, "reserved template-name prefix") {
			t.Fatalf("expected reserved-prefix diagnostic; got %+v", d.Diagnostics())
		}
	})
}

// importAwareFixture is a plugin whose helper spells a reference into
// another package.
//
// The shape a consumer replacing a generator's vocabulary takes: the
// helper names a symbol the generated file has to import, which is
// exactly what a static funcmap entry cannot arrange.
type importAwareFixture struct{ pluginfixture.Plugin }

// Templates shadows the embedded fixture's tree with one whose banner
// calls the helper, so the assertion runs through the path a real
// consumer uses — a template naming a symbol in another package.
func (importAwareFixture) Templates(string) (fs.FS, bool) {
	return fstest.MapFS{
		"templates/golang/banner.tmpl": &fstest.MapFile{Data: []byte(
			`{{- define "bannergen.banner" -}}` + "\n" +
				`// === {{ .Title }} ===` + "\n" +
				`var _ = {{ bannerRule }}` + "\n" +
				`{{- end -}}`,
		)},
	}, true
}

func (importAwareFixture) TemplateFuncsFor(
	_ string, reg plugin.ImportRegistrar,
) template.FuncMap {
	return template.FuncMap{
		"bannerRule": func() (string, error) {
			// Asking for the qualifier is what registers the import,
			// so the returned text and the file's import block cannot
			// disagree.
			local, err := reg.Import("strings")
			if err != nil {
				return "", err
			}
			return local + ".Repeat(\"=\", 3)", nil
		},
	}
}

// TestMerge_ImportAwareFuncRegistersItsImport pins the capability a
// static funcmap entry does not have.
//
// A plugin helper returning `strings.Repeat(…)` used to render
// cleanly into a file that never imported strings — output that looks
// right and fails the consumer's build, which is the worst place to
// find out. The helper now receives the rendering file's import set,
// so naming the package and importing it are one step.
func TestMerge_ImportAwareFuncRegistersItsImport(t *testing.T) {
	t.Parallel()

	ctx, mem, d := newBackendContext(t)
	fx := importAwareFixture{}
	ctx.Plugins = []plugin.Plugin{fx}
	ctx.Ordered = []plugin.Plugin{fx}
	target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
	f := bindFile(t, ctx, target)
	banner := &pluginfixture.Banner{
		Title:   "USERS",
		Message: "unused",
		Target:  target,
	}
	if err := f.Top().Append(banner, emit.Provenance{SetBy: pluginfixture.Name}); err != nil {
		t.Fatalf("append banner: %v", err)
	}
	addEmitPackage(t, ctx, emitPackage("users", &emit.Struct{
		Name: "User", Package: "users", Target: target,
		Fields: []*emit.Field{{Name: "ID", Type: emit.Builtin("int")}},
	}))

	body := string(assertRenderSucceeds(t, ctx, mem, d, target))

	t.Run("the helper is callable from a plugin template", func(t *testing.T) {
		t.Parallel()
		if !strings.Contains(body, "strings.Repeat") {
			t.Errorf("the helper did not render; got:\n%s", body)
		}
	})

	t.Run("the package it names is imported", func(t *testing.T) {
		t.Parallel()
		if !strings.Contains(body, `"strings"`) {
			t.Errorf("the reference rendered without its import, which is a file "+
				"that does not compile; got:\n%s", body)
		}
	})
}

// replacerFixture ships a banner template declaring that it replaces
// the one another plugin already defines.
type replacerFixture struct{ pluginfixture.Plugin }

func (replacerFixture) Name() string { return "replacer" }

func (replacerFixture) ReplacesTemplates(string) []string {
	return []string{"bannergen.banner"}
}

func (replacerFixture) Templates(string) (fs.FS, bool) {
	return fstest.MapFS{
		"templates/golang/banner.tmpl": &fstest.MapFile{Data: []byte(
			`{{- define "bannergen.banner" -}}` + "\n" +
				`// replaced: {{ .Title }}` + "\n" +
				`{{- end -}}`,
		)},
	}, true
}

// TestMerge_DeclaredTemplateReplacement pins re-use over forking.
//
// Two plugins defining one template name is ordinarily a collision,
// and that rule is right for the accident: whichever won would be
// decided by registration order and nothing would say so. It is wrong
// for the intent, which is a consumer changing how a generator's
// output reads without forking the generator to do it.
func TestMerge_DeclaredTemplateReplacement(t *testing.T) {
	t.Parallel()

	t.Run("a declared replacement supersedes the original", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		// The replacer is registered *first*, so passing means the
		// ordering rule did the work rather than luck.
		fx, rep := pluginfixture.Plugin{}, replacerFixture{}
		ctx.Plugins = []plugin.Plugin{rep, fx}
		ctx.Ordered = []plugin.Plugin{rep, fx}
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		f := bindFile(t, ctx, target)
		banner := &pluginfixture.Banner{Title: "USERS", Message: "m", Target: target}
		if err := f.Top().Append(banner, emit.Provenance{SetBy: pluginfixture.Name}); err != nil {
			t.Fatalf("append banner: %v", err)
		}
		addEmitPackage(t, ctx, emitPackage("users", &emit.Struct{
			Name: "User", Package: "users", Target: target,
			Fields: []*emit.Field{{Name: "ID", Type: emit.Builtin("int")}},
		}))

		body := string(assertRenderSucceeds(t, ctx, mem, d, target))
		if !strings.Contains(body, "// replaced: USERS") {
			t.Errorf("the declared replacement did not win; got:\n%s", body)
		}
		if strings.Contains(body, "// === USERS ===") {
			t.Errorf("the superseded template still rendered; got:\n%s", body)
		}
	})

	t.Run("an undeclared collision is still refused", func(t *testing.T) {
		t.Parallel()
		// The rule the declaration opts out of has to still be there,
		// or every accidental claim of a dispatch name becomes a silent
		// race decided by plugin order.
		ctx, _, d := newBackendContext(t)
		fx, other := pluginfixture.Plugin{}, silentReplacer{}
		ctx.Plugins = []plugin.Plugin{fx, other}
		ctx.Ordered = []plugin.Plugin{fx, other}
		addEmitPackage(t, ctx, emitPackage("users", &emit.Struct{
			Name: "User", Package: "users",
			Target: emit.Target{Dir: "users", Filename: "user.go", Package: "users"},
		}))
		_ = backend.New().Render(ctx)
		// Reported rather than returned: the backend records the
		// refusal against the run so every offending plugin surfaces,
		// instead of stopping at the first.
		collided := func(x diag.Diag) bool {
			return strings.Contains(x.Message, "template name collision")
		}
		if !slices.ContainsFunc(d.Diagnostics(), collided) {
			t.Errorf("no collision reported; got %v", d.Diagnostics())
		}
	})
}

// silentReplacer claims a template name without declaring that it
// means to.
type silentReplacer struct{ pluginfixture.Plugin }

func (silentReplacer) Name() string { return "silent" }

func (silentReplacer) Templates(string) (fs.FS, bool) {
	return fstest.MapFS{
		"templates/golang/banner.tmpl": &fstest.MapFile{Data: []byte(
			`{{- define "bannergen.banner" -}}x{{- end -}}`,
		)},
	}, true
}
