// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// testMetaKindKey / testMetaExportedKey are package-level metadata
// keys the meta-funcmap test seeds onto a fixture entity.
// Declared at package scope so the global key registry sees each
// key exactly once — meta.NewKey panics on duplicate names, so
// repeated test runs (e.g. `go test -count=2`) need a single
// registration site.
//
//nolint:gochecknoglobals // shared across subtest invocations; registration is one-shot by design.
var (
	testMetaKindKey     = meta.NewKey("test-kind", meta.StringParser)
	testMetaExportedKey = meta.NewKey("test-exported", meta.BoolParser)
)

// TestFuncmapExtras_Naming covers the Naming-category funcmap
// entries (pascal, camel, snake, screaming, exported) — they
// delegate to `core/naming` so the assertion is on the round-trip
// rendered text rather than the conversion semantics themselves.
func TestFuncmapExtras_Naming(t *testing.T) {
	t.Parallel()

	body := renderWithPluginTemplate(t,
		`{{ define "emit.constant" -}}
// pascal:    {{ pascal "user_id" }}
// camel:     {{ camel "user_id" }}
// snake:     {{ snake "UserID" }}
// screaming: {{ screaming "user_id" }}
// exported:  {{ exported "userID" }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`)
	for _, want := range []string{
		"// pascal:    UserID",
		"// camel:     userID",
		"// snake:     user_id",
		"// screaming: USER_ID",
		"// exported:  UserID",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body; got:\n%s", want, body)
		}
	}
}

// TestFuncmapExtras_String covers the String-category funcmap
// entries (join, title, upper, lower, trim, split, default,
// coalesce).
func TestFuncmapExtras_String(t *testing.T) {
	t.Parallel()

	body := renderWithPluginTemplate(t,
		`{{ define "emit.constant" -}}
// join:     {{ join (split "a,b,c" ",") " | " }}
// upper:    {{ upper "hello" }}
// lower:    {{ lower "WORLD" }}
// trim:     {{ trim "  spaced  " }}
// default:  {{ default "" "fallback" }}
// coalesce: {{ coalesce "" "" "third" }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`)
	for _, want := range []string{
		"// join:     a | b | c",
		"// upper:    HELLO",
		"// lower:    world",
		"// trim:     spaced",
		"// default:  fallback",
		"// coalesce: third",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body; got:\n%s", want, body)
		}
	}
}

// TestFuncmapExtras_Meta covers the Meta-read category — meta,
// metaBool, metaStr, hasMeta, metaEq — by populating an entity's
// metadata bag and asserting the rendered template surfaces the
// values correctly.
func TestFuncmapExtras_Meta(t *testing.T) {
	t.Parallel()

	body := renderWithPluginTemplateUsing(t,
		`{{ define "emit.constant" -}}
// hasMeta:  {{ hasMeta . "test-kind" }}
// metaStr:  {{ metaStr . "test-kind" }}
// metaBool: {{ metaBool . "test-exported" }}
// metaEq:   {{ metaEq . "test-kind" "primary" }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`,
		func(c *emit.Constant) {
			testMetaKindKey.SetManual(c.EnsureMeta(), "primary", "extrasprobe")
			testMetaExportedKey.SetManual(c.EnsureMeta(), true, "extrasprobe")
		})
	for _, want := range []string{
		"// hasMeta:  true",
		"// metaStr:  primary",
		"// metaBool: true",
		"// metaEq:   true",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body; got:\n%s", want, body)
		}
	}
}

// TestFuncmapExtras_Debug covers the Debug category — origin and
// explain — by rendering an entity through a plugin template that
// prints both helpers. The entity is synthetic (no Origin) so the
// helpers surface their no-provenance forms.
func TestFuncmapExtras_Debug(t *testing.T) {
	t.Parallel()

	body := renderWithPluginTemplate(t,
		`{{ define "emit.constant" -}}
// origin:  {{ origin . }}
// explain: {{ explain . }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`)
	// gofmt strips trailing whitespace from comments, so the
	// empty-origin form lands as a bare `// origin:` line.
	if !strings.Contains(body, "// origin:\n") {
		t.Fatalf("expected origin line; got:\n%s", body)
	}
	if !strings.Contains(body, "// explain: emit.constant") {
		t.Fatalf("expected explain line carrying the kind; got:\n%s", body)
	}
}

// TestFuncmapExtras_FallbackPaths exercises the branches of the
// Naming/String funcmap entries that the happy-path tests skip:
// `exported` with empty input, `default` returning the original
// (non-zero) value, and `coalesce` returning nil when every
// argument is zero. Each helper renders a sentinel substring the
// assertion locates verbatim.
func TestFuncmapExtras_FallbackPaths(t *testing.T) {
	t.Parallel()

	body := renderWithPluginTemplate(t,
		`{{ define "emit.constant" -}}
// exportedEmpty: [{{ exported "" }}]
// defaultReal:   [{{ default "real" "fallback" }}]
// defaultNil:    [{{ default nil "fallback" }}]
// coalesceAll:   [{{ coalesce "" "" }}]
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`)
	for _, want := range []string{
		"// exportedEmpty: []",
		"// defaultReal:   [real]",
		// `default nil` reaches isZero's nil-interface guard.
		"// defaultNil:    [fallback]",
		// coalesce returns nil when every value is zero;
		// text/template renders an unset interface as "<no value>".
		"// coalesceAll:   [<no value>]",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body; got:\n%s", want, body)
		}
	}
}

// TestFuncmapExtras_MetaMismatch covers the type-mismatch and
// unset-key branches of the Meta-read helpers: metaBool reading a
// string-valued key returns false, metaStr reading a bool-valued
// key returns the empty string, and meta/hasMeta on an unset name
// return nil/false respectively.
func TestFuncmapExtras_MetaMismatch(t *testing.T) {
	t.Parallel()

	body := renderWithPluginTemplateUsing(t,
		`{{ define "emit.constant" -}}
// metaBoolMismatch: {{ metaBool . "test-kind" }}
// metaStrMismatch:  [{{ metaStr . "test-exported" }}]
// metaUnset:        [{{ meta . "test-missing" }}]
// hasMetaUnset:     {{ hasMeta . "test-missing" }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`,
		func(c *emit.Constant) {
			testMetaKindKey.SetManual(c.EnsureMeta(), "primary", "extrasprobe")
			testMetaExportedKey.SetManual(c.EnsureMeta(), true, "extrasprobe")
		})
	for _, want := range []string{
		"// metaBoolMismatch: false",
		"// metaStrMismatch:  []",
		"// metaUnset:        [<no value>]",
		"// hasMetaUnset:     false",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body; got:\n%s", want, body)
		}
	}
}

// TestFuncmapExtras_NilHost covers the nil-host guard in each
// Meta-read helper (metaValue, metaBool, metaStr, hasMeta) by
// passing the template's `nil` literal — text/template binds it to
// the interface parameter as the typed-nil [emit.Node].
func TestFuncmapExtras_NilHost(t *testing.T) {
	t.Parallel()

	body := renderWithPluginTemplate(t,
		`{{ define "emit.constant" -}}
// metaNil:     [{{ meta nil "k" }}]
// metaBoolNil: {{ metaBool nil "k" }}
// metaStrNil:  [{{ metaStr nil "k" }}]
// hasMetaNil:  {{ hasMeta nil "k" }}
// originNil:   [{{ origin nil }}]
// explainNil:  {{ explain nil }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`)
	for _, want := range []string{
		"// metaNil:     [<no value>]",
		"// metaBoolNil: false",
		"// metaStrNil:  []",
		"// hasMetaNil:  false",
		"// originNil:   []",
		"// explainNil:  (nil)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body; got:\n%s", want, body)
		}
	}
}

// TestFuncmapExtras_OriginVariants exercises the three positive
// branches of `origin`: a complete file+line pair renders as
// "file:line", a file-only origin (Line == 0) drops the suffix,
// and a synthetic origin (File == "") returns the empty string.
func TestFuncmapExtras_OriginVariants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		pos  position.Pos
		want string
	}{
		{
			name: "file and line",
			pos:  position.Pos{File: "pkg/source.go", Line: 42},
			want: "// origin: [pkg/source.go:42]",
		},
		{
			name: "file only",
			pos:  position.Pos{File: "pkg/source.go"},
			want: "// origin: [pkg/source.go]",
		},
		{
			name: "empty file",
			pos:  position.Pos{},
			want: "// origin: []",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := renderWithPluginTemplateUsing(t,
				`{{ define "emit.constant" -}}
// origin: [{{ origin . }}]
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`,
				func(c *emit.Constant) {
					c.OriginNode = &node.Constant{
						BaseNode: node.BaseNode{SourcePos: tc.pos},
					}
				})
			if !strings.Contains(body, tc.want) {
				t.Fatalf("expected %q in body; got:\n%s", tc.want, body)
			}
		})
	}
}

// TestFuncmapExtras_ExplainRich covers the directive-count and
// meta-count append branches of `explain` by seeding the host with
// both a directive list and a metadata entry. The assertion
// inspects the rendered explain line for both counters.
func TestFuncmapExtras_ExplainRich(t *testing.T) {
	t.Parallel()

	body := renderWithPluginTemplateUsing(t,
		`{{ define "emit.constant" -}}
// explain: {{ explain . }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`,
		func(c *emit.Constant) {
			c.OriginNode = &node.Constant{
				BaseNode: node.BaseNode{
					SourcePos: position.Pos{File: "pkg/source.go", Line: 7},
				},
			}
			c.DirectiveList = []*directive.Directive{
				{Name: directive.Name("gen:probe")},
			}
			testMetaKindKey.SetManual(c.EnsureMeta(), "primary", "extrasprobe")
		})
	for _, want := range []string{
		"from=pkg/source.go:7",
		"directives=1",
		"meta=1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body; got:\n%s", want, body)
		}
	}
}

// renderWithPluginTemplate runs the backend against a single-
// constant fixture rendered through a plugin-supplied
// `emit.constant` template body. Centralised so each per-category
// test stays at the assertion altitude.
func renderWithPluginTemplate(t *testing.T, body string) string {
	t.Helper()
	return renderWithPluginTemplateUsing(t, body, nil)
}

// renderWithPluginTemplateUsing extends renderWithPluginTemplate
// with a configure hook the caller uses to seed metadata or
// other per-entity state before render.
func renderWithPluginTemplateUsing(t *testing.T, body string, configure func(*emit.Constant)) string {
	t.Helper()
	ctx, mem, d := newBackendContext(t)
	provider := &stubTemplateProvider{
		name: "extrasprobe",
		tmplFS: fstest.MapFS{
			"templates/golang/override.tmpl": &fstest.MapFile{Data: []byte(body)},
		},
	}
	ctx.Plugins = []plugin.Plugin{provider}
	ctx.Ordered = []plugin.Plugin{provider}
	target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
	k := &emit.Constant{
		Name: "K", Package: "x", Target: target,
		Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "1"},
	}
	if configure != nil {
		configure(k)
	}
	addEmitPackage(t, ctx, &emit.Package{
		Name: "x", Path: "x",
		Constants: []*emit.Constant{k},
	})
	return string(assertRenderSucceeds(t, ctx, mem, d, target))
}
