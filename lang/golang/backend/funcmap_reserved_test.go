// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang/backend"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// TestSlotFuncmap covers the `slot` funcmap entry — the canonical
// helper plugin templates use to read a host's slot generically.
// The helper accepts any [emit.SlotHost]; non-host inputs surface
// as [backend.ErrSlotHostUnsupported].
func TestSlotFuncmap(t *testing.T) {
	t.Parallel()

	t.Run("ErrSlotHostUnsupported is exported and satisfies errors.Is", func(t *testing.T) {
		t.Parallel()
		if backend.ErrSlotHostUnsupported == nil {
			t.Fatalf("ErrSlotHostUnsupported must be exported and non-nil")
		}
		if !errors.Is(backend.ErrSlotHostUnsupported, backend.ErrSlotHostUnsupported) {
			t.Fatalf("ErrSlotHostUnsupported must satisfy errors.Is reflexivity")
		}
	})

	t.Run("slot funcmap reads a named slot on a SlotHost", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		provider := &stubTemplateProvider{
			name: "slotreader",
			tmplFS: fstest.MapFS{
				"templates/golang/override.tmpl": &fstest.MapFile{Data: []byte(
					`{{ define "emit.struct" -}}
type {{ .Name }} struct {
}
// fields-slot-count: {{ (slot . "fields").Len }}
{{- end -}}`,
				)},
			},
		}
		ctx.Plugins = []plugin.Plugin{provider}
		ctx.Ordered = []plugin.Plugin{provider}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target, fieldSpec{name: "F", builtin: "int"},
		)))
		body := string(assertRenderSucceeds(t, ctx, mem, d, target))
		if !strings.Contains(body, "// fields-slot-count: 0") {
			t.Fatalf("expected fields-slot-count comment; got:\n%s", body)
		}
	})

	t.Run("slot funcmap rejects non-SlotHost input with ErrSlotHostUnsupported", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		provider := &stubTemplateProvider{
			name: "slotmisuser",
			tmplFS: fstest.MapFS{
				"templates/golang/override.tmpl": &fstest.MapFile{Data: []byte(
					`{{ define "emit.constant" -}}
{{ slot . "audit" }}
{{- end -}}`,
				)},
			},
		}
		ctx.Plugins = []plugin.Plugin{provider}
		ctx.Ordered = []plugin.Plugin{provider}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Constants: []*emit.Constant{{
				Name: "K", Package: "x", Target: target,
				Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "1"},
			}},
		})
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("slot misuse must suppress sink writes")
		}
		// Confirm the diagnostic carries the ErrSlotHostUnsupported
		// sentinel's message fragment.
		var found bool
		for _, dg := range d.Diagnostics() {
			if strings.Contains(dg.Message, "value does not own slots") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected ErrSlotHostUnsupported diagnostic; got %+v", d.Diagnostics())
		}
	})
}

// TestExternalFuncmap covers the `external` funcmap entry — the
// canonical helper plugin templates use to construct a polymorphic
// package-qualified symbol reference at render time. The helper
// takes (pkg, name) and returns a [*emit.Expr] that downstream
// `renderExpr` resolves to `<alias>.<name>` and registers the
// import on the host file's import set, identically to refs
// constructed in Go code via [emit.NewExternal].
//
// Each subtest exercises one concrete (pkg, name) pair so the
// dispatch through `renderExpr` is observed end-to-end and the
// rendered text confirms the alias-qualified spelling lands in
// the output. Import registration itself is `renderExpr`'s
// contract, covered in its own tests.
func TestExternalFuncmap(t *testing.T) {
	t.Parallel()

	t.Run("renderExpr (external pkg name) emits qualified reference", func(t *testing.T) {
		t.Parallel()
		body := renderWithPluginTemplate(t,
			`{{ define "emit.constant" -}}
// Call: {{ renderExpr (external "fmt" "Sprintf") }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`)
		if !strings.Contains(body, "// Call: fmt.Sprintf") {
			t.Fatalf("expected fmt.Sprintf qualified reference; got:\n%s", body)
		}
	})

	t.Run("multi-segment import path qualifies on last path element", func(t *testing.T) {
		t.Parallel()
		body := renderWithPluginTemplate(t,
			`{{ define "emit.constant" -}}
// Call: {{ renderExpr (external "encoding/json" "Marshal") }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`)
		if !strings.Contains(body, "// Call: json.Marshal") {
			t.Fatalf("expected json.Marshal qualified reference; got:\n%s", body)
		}
	})
}

// TestProvenanceFuncmap covers the `provenance` funcmap entry —
// the canonical attribution helper plugin templates use to surface
// "where did this come from" markers in generated comments.
func TestProvenanceFuncmap(t *testing.T) {
	t.Parallel()

	t.Run("synthetic entity renders as kind + (synthetic)", func(t *testing.T) {
		t.Parallel()
		body := renderWithPluginTemplate(t,
			`{{ define "emit.constant" -}}
// {{ provenance . }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`)
		if !strings.Contains(body, "// emit.constant (synthetic)") {
			t.Fatalf("expected provenance line; got:\n%s", body)
		}
	})

	t.Run("nil host renders as (nil)", func(t *testing.T) {
		t.Parallel()
		body := renderWithPluginTemplate(t,
			`{{ define "emit.constant" -}}
// {{ provenance nil }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`)
		if !strings.Contains(body, "// (nil)") {
			t.Fatalf("expected provenance nil line; got:\n%s", body)
		}
	})

	t.Run("origin with empty file falls back to (synthetic)", func(t *testing.T) {
		t.Parallel()
		body := renderWithPluginTemplateUsing(t,
			`{{ define "emit.constant" -}}
// {{ provenance . }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`,
			func(c *emit.Constant) {
				c.OriginNode = &node.Constant{
					BaseNode: node.BaseNode{SourcePos: position.Pos{Line: 1}},
				}
			})
		if !strings.Contains(body, "// emit.constant (synthetic)") {
			t.Fatalf("expected synthetic fallback; got:\n%s", body)
		}
	})

	t.Run("origin with file and line renders as kind from file:line", func(t *testing.T) {
		t.Parallel()
		body := renderWithPluginTemplateUsing(t,
			`{{ define "emit.constant" -}}
// {{ provenance . }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`,
			func(c *emit.Constant) {
				c.OriginNode = &node.Constant{
					BaseNode: node.BaseNode{
						SourcePos: position.Pos{File: "pkg/source.go", Line: 12},
					},
				}
			})
		if !strings.Contains(body, "// emit.constant from pkg/source.go:12") {
			t.Fatalf("expected file:line provenance; got:\n%s", body)
		}
	})

	t.Run("origin with file only drops the line suffix", func(t *testing.T) {
		t.Parallel()
		body := renderWithPluginTemplateUsing(t,
			`{{ define "emit.constant" -}}
// {{ provenance . }}
const {{ .Name }} = {{ renderExpr .Value }}
{{- end -}}`,
			func(c *emit.Constant) {
				c.OriginNode = &node.Constant{
					BaseNode: node.BaseNode{
						SourcePos: position.Pos{File: "pkg/source.go"},
					},
				}
			})
		if !strings.Contains(body, "// emit.constant from pkg/source.go\n") {
			t.Fatalf("expected file-only provenance; got:\n%s", body)
		}
	})
}
