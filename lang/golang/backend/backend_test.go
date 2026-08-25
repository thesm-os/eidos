// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend_test

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang/backend"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
	"go.thesmos.sh/eidos/store"
)

// TestBackend_Name covers the stable plugin identifier.
func TestBackend_Name(t *testing.T) {
	t.Parallel()

	t.Run("Name returns the documented constant", func(t *testing.T) {
		t.Parallel()
		if got := mustNew(t).Name(); got != backend.Name {
			t.Fatalf("Name = %q, want %q", got, backend.Name)
		}
	})

	t.Run("Name is namespaced for collision avoidance", func(t *testing.T) {
		t.Parallel()
		if !strings.Contains(backend.Name, ".") {
			t.Fatalf("Name %q should be namespaced (contain '.')", backend.Name)
		}
	})
}

// TestBackend_Language covers the target-language identifier.
func TestBackend_Language(t *testing.T) {
	t.Parallel()

	t.Run("Language returns the documented constant", func(t *testing.T) {
		t.Parallel()
		if got := mustNew(t).Language(); got != backend.Language {
			t.Fatalf("Language = %q, want %q", got, backend.Language)
		}
	})
}

// TestBackend_Render covers the per-Target render orchestration.
func TestBackend_Render(t *testing.T) {
	t.Parallel()

	t.Run("empty store produces no sink writes and no diagnostics", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render on empty store: %v", err)
		}
		if mem.Len() != 0 {
			t.Fatalf("expected no sink writes; got %d", mem.Len())
		}
		if len(d.Diagnostics()) != 0 {
			t.Fatalf("expected no diagnostics; got %+v", d.Diagnostics())
		}
	})

	t.Run("writes one gofmt-clean file per distinct Target", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		t1 := emit.Target{Dir: "out/users", Filename: "user.go", Package: "users"}
		t2 := emit.Target{Dir: "out/orders", Filename: "order.go", Package: "orders"}
		addEmitPackage(t, ctx, emitPackage("users", emitStructWithFields(
			"users", "User", t1,
			fieldSpec{name: "ID", builtin: "int"},
			fieldSpec{name: "Name", builtin: "string"},
		)))
		addEmitPackage(t, ctx, emitPackage("orders", emitStructWithFields(
			"orders", "Order", t2,
			fieldSpec{name: "Total", builtin: "int"},
		)))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if d.HasErrors() {
			t.Fatalf("unexpected error diagnostics: %+v", d.Diagnostics())
		}
		if mem.Len() != 2 {
			t.Fatalf("expected 2 sink writes; got %d (files=%v)", mem.Len(), mem.Files())
		}
		// Spot-check each file's package line and decl line.
		for _, c := range []struct {
			target  emit.Target
			pkgLine string
			decl    string
		}{
			{t1, "package users", "type User struct {"},
			{t2, "package orders", "type Order struct {"},
		} {
			body, ok := mem.Get(c.target)
			if !ok {
				t.Fatalf("no output for %v", c.target)
			}
			if !strings.Contains(string(body), c.pkgLine) {
				t.Fatalf("%v: body should contain %q; got:\n%s", c.target, c.pkgLine, body)
			}
			if !strings.Contains(string(body), c.decl) {
				t.Fatalf("%v: body should contain %q; got:\n%s", c.target, c.decl, body)
			}
		}
	})

	t.Run("zero-valued Target never reaches the sink", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		// Zero Target — store's by-target index drops it on insert, so
		// nothing reaches the render loop. Verifies the upstream
		// filter via the public render path.
		addEmitPackage(t, ctx, emitPackage(
			"x",
			emitStructWithFields("x", "X", emit.Target{}, fieldSpec{name: "F", builtin: "int"}),
		))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if mem.Len() != 0 {
			t.Fatalf("zero-target struct should not produce sink output; got %d files", mem.Len())
		}
		if d.HasErrors() {
			t.Fatalf("unexpected error diagnostics: %+v", d.Diagnostics())
		}
	})

	t.Run("sink failure surfaces as a wrapped error from Render", func(t *testing.T) {
		t.Parallel()
		ctx, _, _ := newBackendContext(t)
		ctx.Sink = &failingSink{}
		target := emit.Target{Dir: "out", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target,
			fieldSpec{name: "F", builtin: "int"},
		)))
		err := mustNew(t).Render(ctx)
		if err == nil {
			t.Fatalf("expected an error when sink fails")
		}
		if !errors.Is(err, errSinkBoom) {
			t.Fatalf("err should wrap errSinkBoom; got %v", err)
		}
		msg := err.Error()
		if !strings.Contains(msg, backend.Name) {
			t.Fatalf("error %q should mention backend Name %q", msg, backend.Name)
		}
		if !strings.Contains(msg, target.JoinPath()) {
			t.Fatalf("error %q should mention target path %q", msg, target.JoinPath())
		}
	})
}

// TestBackend_Golden pins canonical output for every shipped
// rendering fixture so byte-level drift in templates, funcmap,
// format.Source, or goimports is caught at PR time. Each subtest
// covers a representative shape — envelope variations, struct
// shapes, generic forms, enums, methods, and file composition.
func TestBackend_Golden(t *testing.T) {
	t.Parallel()

	t.Run("envelope_full — Source / Plugins / Command + hash footer", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Command = "eidos run --config example.yaml"
		ctx.SourcesOverride = []string{"./internal/users/user.go", "./internal/users/types.go"}
		ctx.Plugins = []plugin.Plugin{
			stubPluginVersion{name: "repogen", version: "1.2.3"},
			stubPluginVersion{name: "mockgen", version: "0.5.0"},
		}
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		addEmitPackage(t, ctx, emitPackage("users", emitStructWithFields(
			"users", "User", target,
			fieldSpec{name: "ID", builtin: "int"},
			fieldSpec{name: "Name", builtin: "string"},
		)))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "envelope_full.go.golden"))
	})

	t.Run("envelope_branded — Brand substitutes header marker and footer EOGC", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Brand = "acmegen"
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target,
			fieldSpec{name: "F", builtin: "int"},
		)))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "envelope_branded.go.golden"))
	})

	t.Run("envelope_customised — HeaderPrefix/Suffix + FooterSuffix", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.HeaderPrefix = []string{
			"//go:build linux",
			"",
			"// Copyright 2026 Acme Industries.",
			"// SPDX-License-Identifier: MIT",
		}
		ctx.HeaderSuffix = []string{"// Reviewed-By: platform-team"}
		ctx.FooterSuffix = []string{"// Signed-Off-By: release-bot"}
		ctx.Command = "eidos run"
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		addEmitPackage(t, ctx, emitPackage("users", emitStructWithFields(
			"users", "User", target,
			fieldSpec{name: "ID", builtin: "int"},
		)))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "envelope_customised.go.golden"))
	})

	t.Run("envelope_minimal — bare DO NOT EDIT + body + hash", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		// No Command, no SourcesOverride, no Plugins. The header
		// collapses to the DO NOT EDIT line alone.
		ctx.Plugins = nil
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target,
			fieldSpec{name: "F", builtin: "int"},
		)))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "envelope_minimal.go.golden"))
	})

	t.Run("struct_simple — no imports", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		addEmitPackage(t, ctx, emitPackage("users", emitStructWithFields(
			"users", "User", target,
			fieldSpec{name: "ID", builtin: "int"},
			fieldSpec{name: "Name", builtin: "string"},
		)))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "struct_simple.go.golden"))
	})

	t.Run("struct_stdlib_import — context.Context field", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		addEmitPackage(t, ctx, emitPackage("users", &emit.Struct{
			Name: "Request", Package: "users", Target: target,
			Fields: []*emit.Field{{Name: "Ctx", Type: emit.External("context", "Context")}},
		}))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "struct_stdlib_import.go.golden"))
	})

	t.Run("struct_external_import — third-party type", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		addEmitPackage(t, ctx, emitPackage("users", &emit.Struct{
			Name: "Wrapper", Package: "users", Target: target,
			Fields: []*emit.Field{{Name: "Inner", Type: emit.External("github.com/example/lib", "Item")}},
		}))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "struct_external_import.go.golden"))
	})

	t.Run("struct_multi_import — stdlib + external regrouped", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		addEmitPackage(t, ctx, emitPackage("users", &emit.Struct{
			Name: "Request", Package: "users", Target: target,
			Fields: []*emit.Field{
				{Name: "Ctx", Type: emit.External("context", "Context")},
				{Name: "Err", Type: emit.External("errors", "Is")},
				{Name: "Item", Type: emit.External("github.com/example/lib", "Item")},
			},
		}))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "struct_multi_import.go.golden"))
	})

	t.Run("struct_with_docs — DocLines render as // above the decl", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		addEmitPackage(t, ctx, emitPackage("users", &emit.Struct{
			BaseEmit: emit.BaseEmit{
				DocLines: []string{
					"User is the canonical user record.",
					"",
					"Fields hold the immutable identifier and the display",
					"name used in UI surfaces.",
				},
			},
			Name: "User", Package: "users", Target: target,
			Fields: []*emit.Field{
				{Name: "ID", Type: emit.Builtin("int")},
				{Name: "Name", Type: emit.Builtin("string")},
			},
		}))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "struct_with_docs.go.golden"))
	})

	t.Run("struct_with_directive_in_docs — directive lines ride DocLines, rendered verbatim", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		// Generators put `//nolint:foo` and similar suppressions at
		// the END of DocLines per Go convention. `renderDocs` detects
		// the leading "//" and renders the line verbatim; regular
		// doc lines get the "// " prefix applied.
		addEmitPackage(t, ctx, emitPackage("users", &emit.Struct{
			BaseEmit: emit.BaseEmit{
				DocLines: []string{
					"Legacy is kept around for backwards compatibility.",
					"//nolint:revive",
				},
			},
			Name: "Legacy", Package: "users", Target: target,
			Fields: []*emit.Field{{Name: "ID", Type: emit.Builtin("int")}},
		}))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "struct_with_directive_in_docs.go.golden"))
	})

	t.Run("package_with_docs — emit.Package.DocLines surface above package decl", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		// Package-level docs ride on emit.Package.DocLines; the
		// backend applies them as the file's package doc when no
		// per-Target emit.File overrides them.
		pkg := &emit.Package{
			BaseEmit: emit.BaseEmit{
				DocLines: []string{
					"Package users models the canonical user record and",
					"the operations performed on it across the platform.",
				},
			},
			Name: "users", Path: "users",
			Structs: []*emit.Struct{
				{
					Name: "User", Package: "users", Target: target,
					Fields: []*emit.Field{{Name: "ID", Type: emit.Builtin("int")}},
				},
			},
		}
		addEmitPackage(t, ctx, pkg)
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "package_with_docs.go.golden"))
	})

	t.Run("package docs land in exactly one file of a multi-file package", func(t *testing.T) {
		t.Parallel()
		// Go allows one package comment. A package emitting N files
		// used to get N copies: it compiles, vet is silent, and godoc
		// takes whichever file sorts first — so nothing downstream
		// catches a file declaring a package comment it does not own.
		ctx, mem, d := newBackendContext(t)
		first := emit.Target{Dir: "users", Filename: "a_builder.go", Package: "users"}
		last := emit.Target{Dir: "users", Filename: "z_suite.go", Package: "users"}
		docs := []string{"Package users models the canonical user record."}
		addEmitPackage(t, ctx, &emit.Package{
			BaseEmit: emit.BaseEmit{DocLines: docs},
			Name:     "users", Path: "users",
			Structs: []*emit.Struct{
				{Name: "Builder", Package: "users", Target: first},
				{Name: "Suite", Package: "users", Target: last},
			},
		})
		firstBody := assertRenderSucceeds(t, ctx, mem, d, first)
		lastBody := assertRenderSucceeds(t, ctx, mem, d, last)
		if !strings.Contains(string(firstBody), docs[0]) {
			t.Fatalf("the lowest-named file should carry the package doc; got:\n%s", firstBody)
		}
		if strings.Contains(string(lastBody), docs[0]) {
			t.Fatalf("a second copy of the package doc was rendered; got:\n%s", lastBody)
		}
	})

	t.Run("an external test package keeps its own doc beside its subject", func(t *testing.T) {
		t.Parallel()
		// One directory, two packages: `users` and `users_test` each
		// own a package comment. Grouping on the directory alone
		// would silence whichever sorted second.
		ctx, mem, d := newBackendContext(t)
		subject := emit.Target{Dir: "users", Filename: "a_user.go", Package: "users"}
		external := emit.Target{Dir: "users", Filename: "z_user_test.go", Package: "users_test"}
		addEmitPackage(t, ctx, &emit.Package{
			BaseEmit: emit.BaseEmit{DocLines: []string{"Package users is the subject."}},
			Name:     "users", Path: "users",
			Structs: []*emit.Struct{{Name: "User", Package: "users", Target: subject}},
		})
		addEmitPackage(t, ctx, &emit.Package{
			BaseEmit: emit.BaseEmit{DocLines: []string{"Package users_test exercises the subject."}},
			Name:     "users_test", Path: "users_test",
			Structs: []*emit.Struct{{Name: "Case", Package: "users_test", Target: external}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, external)
		if !strings.Contains(string(body), "Package users_test exercises the subject.") {
			t.Fatalf("the external test package lost its own doc; got:\n%s", body)
		}
	})

	t.Run("struct_with_field_annotations — field docs, tags, line comments", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		addEmitPackage(t, ctx, emitPackage("users", &emit.Struct{
			BaseEmit: emit.BaseEmit{
				DocLines: []string{"User is the canonical user record."},
			},
			Name: "User", Package: "users", Target: target,
			Fields: []*emit.Field{
				{
					BaseEmit: emit.BaseEmit{
						DocLines: []string{
							"ID is the immutable primary key.",
							"Stored as the database row identifier.",
						},
					},
					Name: "ID",
					Type: emit.Builtin("int"),
					Tag:  `json:"id"`,
				},
				{
					BaseEmit: emit.BaseEmit{
						DocLines: []string{"Name is the display name shown in the UI."},
					},
					Name:        "Name",
					Type:        emit.Builtin("string"),
					Tag:         `json:"name"`,
					LineComment: "max 64 chars per product spec",
				},
				{
					Name:        "Internal",
					Type:        emit.Builtin("bool"),
					LineComment: "set by middleware; not exposed externally",
				},
			},
		}))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "struct_with_field_annotations.go.golden"))
	})

	t.Run("struct_alias_collision — suffix-2 deterministic alias", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", &emit.Struct{
			Name: "X", Package: "x", Target: target,
			Fields: []*emit.Field{
				{Name: "A", Type: emit.External("github.com/example/users", "User")},
				{Name: "B", Type: emit.External("github.com/other/users", "User")},
			},
		}))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "struct_alias_collision.go.golden"))
	})

	t.Run("interface_simple — methods + embeds", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "iox", Filename: "iox.go", Package: "iox"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "iox", Path: "iox",
			Interfaces: []*emit.Interface{{
				BaseEmit: emit.BaseEmit{DocLines: []string{"Reader is a minimal byte reader."}},
				Name:     "Reader", Package: "iox", Target: target,
				Embeds: []*emit.Embed{{Type: emit.External("io", "Closer")}},
				Methods: []*emit.Method{
					{
						Name:    "Read",
						Params:  []*emit.Param{{Name: "p", Type: emit.Builtin("byte")}},
						Returns: emit.AnonReturns(emit.Builtin("int"), emit.Builtin("error")),
					},
					{Name: "Reset"},
				},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "interface_simple.go.golden"))
	})

	t.Run("alias_definition — type X int", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "users", Filename: "id.go", Package: "users"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "users", Path: "users",
			Aliases: []*emit.Alias{{
				BaseEmit: emit.BaseEmit{DocLines: []string{"UserID is the canonical primary key."}},
				Name:     "UserID", Package: "users", File: target,
				Target: emit.Builtin("int"),
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "alias_definition.go.golden"))
	})

	t.Run("alias_alias — type X = Y", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Aliases: []*emit.Alias{{
				Name: "Bytes", Package: "x", File: target,
				Target:  emit.Builtin("byte"),
				IsAlias: true,
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "alias_alias.go.golden"))
	})

	t.Run("variable_combinations — typed/inferred/no-init", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "cfg", Filename: "cfg.go", Package: "cfg"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "cfg", Path: "cfg",
			Variables: []*emit.Variable{
				{
					BaseEmit: emit.BaseEmit{DocLines: []string{"Counter tracks invocations."}},
					Name:     "Counter", Package: "cfg", Target: target,
					Type: emit.Builtin("int"),
				},
				{
					Name: "Greeting", Package: "cfg", Target: target,
					Type: emit.Builtin("string"),
					Init: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitString, RawText: "hello"},
				},
				{
					Name: "MaxRetries", Package: "cfg", Target: target,
					Init: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "3"},
				},
			},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "variable_combinations.go.golden"))
	})

	t.Run("constant_combinations — untyped/typed/iota", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "cfg", Filename: "cfg.go", Package: "cfg"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "cfg", Path: "cfg",
			Constants: []*emit.Constant{
				{
					BaseEmit: emit.BaseEmit{DocLines: []string{"Pi is a mathematical constant."}},
					Name:     "Pi", Package: "cfg", Target: target,
					Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitFloat, RawText: "3.14"},
				},
				{
					Name: "Limit", Package: "cfg", Target: target,
					Type:  emit.Builtin("int"),
					Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "100"},
				},
				{
					Name: "Enabled", Package: "cfg", Target: target,
					Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitBool, RawText: "true"},
				},
			},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "constant_combinations.go.golden"))
	})

	t.Run("struct_embeds — adjacent embeds + fields", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "iox", Filename: "wrapper.go", Package: "iox"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "iox", Path: "iox",
			Structs: []*emit.Struct{{
				BaseEmit: emit.BaseEmit{DocLines: []string{"Wrapper composes Reader and Closer."}},
				Name:     "Wrapper", Package: "iox", Target: target,
				Embeds: []*emit.Embed{
					{Type: emit.External("io", "Reader")},
					{Type: emit.External("io", "Closer")},
				},
				Fields: []*emit.Field{{Name: "Closed", Type: emit.Builtin("bool")}},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "struct_embeds.go.golden"))
	})

	t.Run("generic_struct — single-term constraint", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "containers", Filename: "box.go", Package: "containers"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "containers", Path: "containers",
			Structs: []*emit.Struct{{
				BaseEmit: emit.BaseEmit{DocLines: []string{"Box holds a single comparable value."}},
				Name:     "Box", Package: "containers", Target: target,
				TypeParams: []*emit.TypeParam{{
					Name:       "T",
					Constraint: &emit.Constraint{Embedded: []emit.Ref{emit.Builtin("comparable")}},
				}},
				Fields: []*emit.Field{{Name: "V", Type: emit.Builtin("int")}},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "generic_struct.go.golden"))
	})

	t.Run("generic_union — type-set constraint with approx terms", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "math", Filename: "ord.go", Package: "math"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "math", Path: "math",
			Structs: []*emit.Struct{{
				Name: "Ordered", Package: "math", Target: target,
				TypeParams: []*emit.TypeParam{{
					Name: "T",
					Constraint: &emit.Constraint{
						Embedded: []emit.Ref{
							emit.Union(
								emit.UnionTerm{Type: emit.Builtin("int"), Approx: true},
								emit.UnionTerm{Type: emit.Builtin("float64"), Approx: true},
								emit.UnionTerm{Type: emit.Builtin("string")},
							),
						},
					},
				}},
				Fields: []*emit.Field{{Name: "V", Type: emit.Builtin("int")}},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "generic_union.go.golden"))
	})

	t.Run("field_tag_aggregation — base + slot contributors", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		idField := &emit.Field{Name: "ID", Type: emit.Builtin("int"), Tag: `json:"id"`}
		if err := idField.Tags().Append(&emit.Tag{Key: "db", Value: "user_id"}, emit.Provenance{}); err != nil {
			t.Fatalf("Append db tag: %v", err)
		}
		if err := idField.Tags().Append(&emit.Tag{Key: "yaml", Value: "id"}, emit.Provenance{}); err != nil {
			t.Fatalf("Append yaml tag: %v", err)
		}
		nameField := &emit.Field{Name: "Name", Type: emit.Builtin("string")}
		validateTag := &emit.Tag{Key: "validate", Value: "required,max=64"}
		if err := nameField.Tags().Append(validateTag, emit.Provenance{}); err != nil {
			t.Fatalf("Append validate tag: %v", err)
		}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "users", Path: "users",
			Structs: []*emit.Struct{{
				BaseEmit: emit.BaseEmit{DocLines: []string{"User carries the canonical user record."}},
				Name:     "User", Package: "users", Target: target,
				Fields: []*emit.Field{idField, nameField},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "field_tag_aggregation.go.golden"))
	})

	t.Run("var_with_funclit_init — Stmt + Expr through ExprFuncLit", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "handlers", Filename: "h.go", Package: "handlers"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "handlers", Path: "handlers",
			Variables: []*emit.Variable{{
				BaseEmit: emit.BaseEmit{
					DocLines: []string{"Handler bumps the counter and returns the new value."},
				},
				Name: "Handler", Package: "handlers", Target: target,
				Init: &emit.Expr{
					ExprKind:    emit.ExprFuncLit,
					FuncParams:  []*emit.Param{{Name: "n", Type: emit.Builtin("int")}},
					FuncReturns: []emit.Ref{emit.Builtin("int")},
					FuncBody: []*emit.Stmt{
						emit.NewIf(
							&emit.Expr{
								ExprKind: emit.ExprBinary, Op: "<",
								Left:  &emit.Expr{ExprKind: emit.ExprIdent, Name: "n"},
								Right: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "0"},
							},
							[]*emit.Stmt{emit.NewReturn(&emit.Expr{
								ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "0",
							})},
						),
						emit.NewReturn(&emit.Expr{
							ExprKind: emit.ExprBinary, Op: "+",
							Left:  &emit.Expr{ExprKind: emit.ExprIdent, Name: "n"},
							Right: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "1"},
						}),
					},
				},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "var_with_funclit_init.go.golden"))
	})

	t.Run("function_simple — params, returns, body", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "mathx", Filename: "math.go", Package: "mathx"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "mathx", Path: "mathx",
			Functions: []*emit.Function{{
				BaseEmit: emit.BaseEmit{DocLines: []string{"Add returns the sum of a and b."}},
				Name:     "Add", Package: "mathx", Target: target,
				Params: []*emit.Param{
					{Name: "a", Type: emit.Builtin("int")},
					{Name: "b", Type: emit.Builtin("int")},
				},
				Returns: emit.AnonReturns(emit.Builtin("int")),
				Body: []*emit.Stmt{emit.NewReturn(&emit.Expr{
					ExprKind: emit.ExprBinary, Op: "+",
					Left:  &emit.Expr{ExprKind: emit.ExprIdent, Name: "a"},
					Right: &emit.Expr{ExprKind: emit.ExprIdent, Name: "b"},
				})},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "function_simple.go.golden"))
	})

	t.Run("method_on_struct — struct + pointer-receiver method", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "counter", Filename: "counter.go", Package: "counter"}
		host := &emit.Struct{
			BaseEmit: emit.BaseEmit{DocLines: []string{"Counter accumulates a monotonic count."}},
			Name:     "Counter", Package: "counter", Target: target,
			Fields: []*emit.Field{{Name: "n", Type: emit.Builtin("int")}},
		}
		host.Methods = []*emit.Method{{
			BaseEmit:     emit.BaseEmit{DocLines: []string{"Inc bumps the counter by one."}},
			Name:         "Inc",
			Receiver:     emit.Ptr(emit.Internal(host)),
			ReceiverName: "c",
			Body: []*emit.Stmt{emit.NewAssign(
				[]*emit.Expr{
					{ExprKind: emit.ExprField, Receiver: &emit.Expr{ExprKind: emit.ExprIdent, Name: "c"}, Name: "n"},
				},
				"+=",
				[]*emit.Expr{{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "1"}},
			)},
		}}
		addEmitPackage(t, ctx, emitPackage("counter", host))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "method_on_struct.go.golden"))
	})

	t.Run("enum_typed_iota — typed iota promotion with docs", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "status", Filename: "status.go", Package: "status"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "status", Path: "status",
			Enums: []*emit.Enum{{
				BaseEmit: emit.BaseEmit{
					DocLines: []string{"Phase is the position of a job in its lifecycle."},
				},
				Name: "Phase", Package: "status", Target: target,
				Underlying: emit.Builtin("int"),
				Variants: []*emit.EnumVariant{
					{
						BaseEmit: emit.BaseEmit{
							DocLines: []string{"Pending is the initial state before any work begins."},
						},
						Name:  "Pending",
						Value: &emit.Expr{ExprKind: emit.ExprIdent, Name: "iota"},
					},
					{Name: "Active"},
					{
						BaseEmit: emit.BaseEmit{
							DocLines: []string{"Closed is the terminal state after work completes."},
						},
						Name: "Closed",
					},
				},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "enum_typed_iota.go.golden"))
	})

	t.Run("struct_composite_fields — pointer/slice/array/map/func", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Structs: []*emit.Struct{{
				Name: "Composite", Package: "x", Target: target,
				Fields: []*emit.Field{
					{Name: "Ptr", Type: emit.Ptr(emit.Builtin("int"))},
					{Name: "Slice", Type: emit.SliceOf(emit.Builtin("byte"))},
					{Name: "Array", Type: emit.ArrayOf(emit.Builtin("byte"), 32)},
					{Name: "Map", Type: emit.MapOf(emit.Builtin("string"), emit.Builtin("int"))},
					{Name: "Fn", Type: emit.FuncOf(
						[]emit.Ref{emit.Builtin("int")},
						[]emit.Ref{emit.Builtin("int"), emit.Builtin("error")},
					)},
				},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "struct_composite_fields.go.golden"))
	})
}

// assertRenderSucceeds drives the backend over ctx, asserting no
// errors and a non-empty sink output for target, then returns the
// rendered bytes for golden comparison. Centralised so each golden
// subtest stays at the "build fixture, assert golden" altitude.
func assertRenderSucceeds(
	t *testing.T,
	ctx *plugin.BackendContext,
	mem *sink.Memory,
	d *diag.Sink,
	target emit.Target,
) []byte {
	t.Helper()
	if err := mustNew(t).Render(ctx); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if d.HasErrors() {
		t.Fatalf("unexpected error diagnostics: %+v", d.Diagnostics())
	}
	body, ok := mem.Get(target)
	if !ok {
		t.Fatalf("no output for %v", target)
	}
	return body
}

// TestConformance runs the framework's plugin-conformance suite
// against this package's plugin. The suite pins the standard
// framework contracts (stable Name, role-interface compliance,
// deterministic capability ordering, unique directive schema
// names, non-empty Versioned version) so a regression on any
// of them surfaces here before downstream tests trip over it.
//
// The per-role [plugintest.RunBackendSuite] adds byte-stability
// and diagnostic-discipline checks against representative emit
// fixtures: the backend's deterministic-render contract is the
// foundation of byte-identical CI rebuilds and the pipeline's
// manifest provenance hashing.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, backend.New())
	})

	t.Run("backend contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunBackendSuite(
			t,
			backend.New(),
			[]plugintest.BackendFixture{
				{
					Name: "single struct in one package",
					BuildEmitPackages: func(t *testing.T) []*emit.Package {
						t.Helper()
						return []*emit.Package{{
							Name: "demo",
							Path: "example.com/demo",
							Structs: []*emit.Struct{{
								Name:    "User",
								Package: "demo",
								Target: emit.Target{
									Dir:      "demo",
									Filename: "user_gen.go",
									Package:  "demo",
								},
							}},
						}}
					},
					Command: "test-fixture",
				},
				{
					Name: "two structs in one package",
					BuildEmitPackages: func(t *testing.T) []*emit.Package {
						t.Helper()
						return []*emit.Package{{
							Name: "demo",
							Path: "example.com/demo",
							Structs: []*emit.Struct{
								{
									Name:    "User",
									Package: "demo",
									Target: emit.Target{
										Dir:      "demo",
										Filename: "user_gen.go",
										Package:  "demo",
									},
								},
								{
									Name:    "Order",
									Package: "demo",
									Target: emit.Target{
										Dir:      "demo",
										Filename: "order_gen.go",
										Package:  "demo",
									},
								},
							},
						}}
					},
					Command: "test-fixture",
				},
			},
		)
	})
}

// BenchmarkBackend_Render measures one full render pass over a
// multi-target emit graph — template execution, gofmt, the goimports
// pass, and the sink write for every target.
//
// The scope is deliberately the whole pass rather than a single
// stage: Render's cost is dominated by per-target work that only
// exists in composition, and the open question the number answers is
// whether the per-target loop is worth parallelising. A benchmark
// over one target in isolation could not answer that.
//
// Setup builds the store once, outside the timed region. The sink is
// reused across iterations rather than reallocated per pass, so the
// measurement is Render's own allocation rather than the fixture's;
// the in-memory sink overwrites by target, so it does not grow.
func BenchmarkBackend_Render(b *testing.B) {
	b.ReportAllocs()

	ctx, _, _ := newBenchmarkContext(b, 24, sharedPackage)
	be := backend.New()

	for b.Loop() {
		if err := be.Render(ctx); err != nil {
			b.Fatalf("Render: %v", err)
		}
	}
}

// BenchmarkBackend_Render_Targets measures one full render pass per
// operation over emit graphs of increasing target count.
//
// Render walks its targets sequentially, and each iteration of that
// loop builds an independent [renderState] and writes to a distinct
// sink key — there is no shared mutable state between targets. The
// standing question is whether the loop is worth parallelising, and
// that is a question about the shape of this curve: a per-target
// cost that stays flat as the count grows means the sequential loop
// is a pure multiplier and the work is embarrassingly parallel,
// while a per-target cost that climbs would mean the win is
// elsewhere.
//
// Pair the ns/op here with BenchmarkFinaliseBody, which separates
// the format-and-goimports tax from template execution. Together
// they say both how much of a target's cost is parallelisable and
// how much of the total that is.
//
// Store construction is hoisted per size, outside the timed region;
// the in-memory sink is reused across iterations and overwrites by
// target, so it does not grow with b.N.
//
// The Backend is hoisted too. It used to be constructed inside the
// loop, defended as a real part of what a caller pays — but
// [backend.New] parses the whole embedded template set, the type doc
// states Render holds no state across calls, and the pipeline builds
// exactly one Backend at Build time. It is paid once per process,
// not once per Render. Inside the loop it was 47% of the objects at
// one target and 1.25% at a thousand, so a fixed cost varying 40×
// across the sweep flattened the very per-target slope this
// benchmark exists to measure. [BenchmarkBackend_New] measures the
// construction on its own.
//
// The importPaths axis exists because a single shared import path
// hides [applySelfAliases], which rescans every output package of
// the run for every target. With one path that rescan is O(targets);
// with distinct paths it is O(targets × packages), which is the
// shape a real multi-package run has.
func BenchmarkBackend_Render_Targets(b *testing.B) {
	b.ReportAllocs()

	for _, paths := range []benchImportPaths{sharedPackage, distinctPackages, renamedPackages} {
		b.Run(string(paths), func(b *testing.B) {
			b.ReportAllocs()
			for _, targets := range []int{1, 10, 100, 1000} {
				b.Run(strconv.Itoa(targets), func(b *testing.B) {
					b.ReportAllocs()
					ctx, mem, _ := newBenchmarkContext(b, targets, paths)
					be := backend.New()
					for b.Loop() {
						if err := be.Render(ctx); err != nil {
							b.Fatalf("Render: %v", err)
						}
					}
					// A render that silently produced nothing would
					// report a flattering number for no work; the
					// sink must hold one file per target.
					if got := mem.Len(); got != targets {
						b.Fatalf("sink holds %d files, want %d", got, targets)
					}
				})
			}
		})
	}
}

// BenchmarkBackend_New measures what constructing a Backend costs:
// parsing the embedded template set, once per process.
//
// It has its own benchmark because it used to be smeared across
// BenchmarkBackend_Render_Targets' scaling curve, where it was a
// fixed cost pretending to be a per-target one.
func BenchmarkBackend_New(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if backend.New() == nil {
			b.Fatal("New returned nil")
		}
	}
}

// benchImportPaths selects the fixture's output-package shape.
//
// The three arms exercise different things and none subsumes another.
// sharedPackage holds package count at one while target count varies,
// which is the axis the sweep had for a long time — and which hides
// every per-package term in the render loop.
// distinctPackages is the ordinary multi-package run: one output
// package per target, each declaring the name its path derives to.
// renamedPackages is the `pkg=`-override shape, where a package's
// declared name diverges from its directory and every referring file
// needs an explicit alias registered. It is the case
// applySelfAliases exists for, and the only one where its work is
// not discarded.
type benchImportPaths string

const (
	sharedPackage    benchImportPaths = "shared_package"
	distinctPackages benchImportPaths = "distinct_packages"
	renamedPackages  benchImportPaths = "renamed_packages"
)

// newBenchmarkContext builds a [plugin.BackendContext] whose store
// holds targets structs, each routed to its own file so the render
// loop iterates a realistic number of targets.
func newBenchmarkContext(
	b *testing.B,
	targets int,
	paths benchImportPaths,
) (*plugin.BackendContext, *sink.Memory, *diag.Sink) {
	b.Helper()
	s := store.New()
	mem := sink.NewMemory()
	ctx := &plugin.BackendContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   diag.New(),
		Sink:   mem,
		Lang:   "golang",
	}
	pkg := &emit.Package{Name: "bench", Path: "example.com/bench"}
	for i := range targets {
		name := fmt.Sprintf("Entity%d", i)
		dir, importPath, pkgName := "bench", "example.com/bench", "bench"
		switch paths {
		case distinctPackages:
			dir = fmt.Sprintf("bench%d", i)
			importPath = fmt.Sprintf("example.com/bench%d", i)
			// Declared name matches the path's last segment, which
			// is what a package without a pkg= override looks like.
			pkgName = dir
		case renamedPackages:
			dir = fmt.Sprintf("bench%d", i)
			importPath = fmt.Sprintf("example.com/bench%d", i)
			// Diverges from the derived alias, so every referring
			// file needs an explicit registration.
			pkgName = "renamed"
		case sharedPackage:
		}
		pkg.Structs = append(pkg.Structs, &emit.Struct{
			Name:    name,
			Package: importPath,
			Target: emit.Target{
				Dir:        dir,
				Filename:   fmt.Sprintf("entity%d.go", i),
				Package:    pkgName,
				ImportPath: importPath,
			},
			Fields: []*emit.Field{
				{Name: "ID", Type: emit.Builtin("string")},
				{Name: "Count", Type: emit.Builtin("int")},
				{Name: "Ctx", Type: emit.External("context", "Context")},
			},
		})
	}
	if err := s.Emit().AddPackage(pkg); err != nil {
		b.Fatalf("AddPackage: %v", err)
	}
	s.Emit().RebuildByTarget()
	return ctx, mem, diag.New()
}

// TestRender_ConcurrencyPreservesOrder pins that parallel rendering
// changes nothing observable.
//
// Render dispatches one goroutine per CPU across targets, so the
// order work *completes* in is nondeterministic. Everything a
// consumer sees must not be: sink writes reach the sink in
// ByTarget().Keys() order, and diagnostics are buffered per target
// and replayed in that same order rather than written straight
// through. Without the buffering, `-diag-format json` would emit a
// different sequence on each run.
func TestRender_ConcurrencyPreservesOrder(t *testing.T) {
	t.Parallel()

	// A target whose entities render nothing produces an Error
	// diagnostic, so a spread of failing and succeeding targets
	// exercises both the write path and the diagnostic path.
	render := func(t *testing.T) (map[emit.Target][]byte, []string) {
		t.Helper()
		ctx, mem, d := newBackendContext(t)
		pkg := &emit.Package{Name: "conc", Path: "example.com/conc"}
		for i := range 40 {
			pkg.Structs = append(pkg.Structs, &emit.Struct{
				Name:    fmt.Sprintf("Entity%d", i),
				Package: "example.com/conc",
				Target: emit.Target{
					Dir:        "conc",
					Filename:   fmt.Sprintf("entity%d.go", i),
					Package:    "conc",
					ImportPath: "example.com/conc",
				},
				Fields: []*emit.Field{{Name: "ID", Type: emit.Builtin("string")}},
			})
		}
		addEmitPackage(t, ctx, pkg)
		ctx.Store.Emit().RebuildByTarget()
		if err := backend.New().Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		msgs := make([]string, 0, len(d.Diagnostics()))
		for _, g := range d.Diagnostics() {
			msgs = append(msgs, g.Message)
		}
		return mem.Files(), msgs
	}

	t.Run("every target reaches the sink", func(t *testing.T) {
		t.Parallel()
		// A concurrent pass that dropped work would still look
		// deterministic across runs, so count is asserted separately
		// from stability.
		if got, _ := render(t); len(got) != 40 {
			t.Fatalf("sink holds %d files, want 40", len(got))
		}
	})

	t.Run("two runs produce byte-identical output", func(t *testing.T) {
		t.Parallel()
		first, _ := render(t)
		second, _ := render(t)
		if len(first) != len(second) {
			t.Fatalf("run sizes differ: %d vs %d", len(first), len(second))
		}
		for target, body := range first {
			if !bytes.Equal(body, second[target]) {
				t.Fatalf("%s differed between runs", target.JoinPath())
			}
		}
	})

	t.Run("diagnostic order is stable across runs", func(t *testing.T) {
		t.Parallel()
		_, first := render(t)
		_, second := render(t)
		if !slices.Equal(first, second) {
			t.Fatalf("diagnostic order differed:\n%v\nvs\n%v", first, second)
		}
	})
}

// TestRender_UnresolvedQualifierWarns covers the diagnostic that
// replaces the resolver's repair.
//
// The fixture is the one shape that reaches it through core render
// paths: a BuiltinRef whose name is a qualified type. renderType
// emits it verbatim without calling `imp`, so the body names `time`
// and nothing binds it. Today goimports guesses an import from the
// developer's module cache and reports the guess after the fact;
// the Warn names the qualifier a template actually wrote, which is
// what a reader needs, and fires whether or not a guess is
// available.
func TestRender_UnresolvedQualifierWarns(t *testing.T) {
	t.Parallel()

	target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}

	build := func(t *testing.T) (*sink.Memory, *diag.Sink) {
		t.Helper()
		ctx, mem, d := newBackendContext(t)
		addEmitPackage(t, ctx, emitPackage("x", &emit.Struct{
			Name: "X", Package: "x", Target: target,
			Fields: []*emit.Field{{Name: "When", Type: emit.Builtin("time.Time")}},
		}))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		return mem, d
	}

	t.Run("the Warn names the qualifier, not a guessed path", func(t *testing.T) {
		t.Parallel()
		_, d := build(t)
		if !diagnosticsContain(d, diag.Warn, `unresolved qualifier "time"`) {
			t.Fatalf("expected an unresolved-qualifier Warn; got %+v", d.Diagnostics())
		}
	})

	t.Run("the file is still written", func(t *testing.T) {
		t.Parallel()
		// Matching the runGoFormat precedent: broken output reaches
		// the sink so it can be read. Withholding it would leave the
		// reader with a diagnostic and no way to see what produced it.
		mem, _ := build(t)
		if _, ok := mem.Get(target); !ok {
			t.Fatalf("file must reach the sink even with an unresolved qualifier")
		}
	})

	t.Run("the run does not fail", func(t *testing.T) {
		t.Parallel()
		// Warn, not Error: the check cannot prove its verdict, and
		// the failure it describes is attributed downstream by
		// `go build` at the exact line.
		_, d := build(t)
		if d.HasErrors() {
			t.Fatalf("unresolved qualifier must not fail the run; got %+v", d.Diagnostics())
		}
	})
}

// TestRender_SiblingTargetSatisfiesQualifier is the assertion the
// whole design exists for. Two targets rendering into one package,
// one declaring a package-scope name and the other selecting on it,
// is correct Go that needs no import — and it is exactly what
// multi-output routing produces.
//
// This fails first if the package union is dropped or computed per
// target, which is the mistake the per-target prune invites.
func TestRender_SiblingTargetSatisfiesQualifier(t *testing.T) {
	t.Parallel()

	t.Run("a name a sibling declares raises no diagnostic", func(t *testing.T) {
		t.Parallel()
		ctx, _, d := newBackendContext(t)
		decl := emit.Target{Dir: "x", Filename: "decl.go", Package: "x"}
		use := emit.Target{Dir: "x", Filename: "use.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			// Registry is package scope in decl.go; use.go selects on
			// it. Nothing imports anything.
			Variables: []*emit.Variable{{
				Name: "Registry", Package: "x", Target: decl,
				Init: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "0"},
			}},
			Functions: []*emit.Function{{
				Name: "Use", Package: "x", Target: use,
				Body: []*emit.Stmt{emit.NewRawStmt("_ = Registry.Field")},
			}},
		})
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if diagnosticsContain(d, diag.Warn, "unresolved qualifier") {
			t.Fatalf("a sibling-declared name must not be reported; got %+v", d.Diagnostics())
		}
	})

	t.Run("a name declared in a different package is still reported", func(t *testing.T) {
		t.Parallel()
		// The union must be keyed by package, not merged across the
		// run. A name declared in package y is not in scope in
		// package x, so collapsing the two would silently suppress a
		// genuine report — the failure direction a per-target check
		// cannot see and a run-wide one invites.
		ctx, _, d := newBackendContext(t)
		elsewhere := emit.Target{Dir: "y", Filename: "decl.go", Package: "y"}
		use := emit.Target{Dir: "x", Filename: "use.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Variables: []*emit.Variable{{
				Name: "Registry", Package: "y", Target: elsewhere,
				Init: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "0"},
			}},
			Functions: []*emit.Function{{
				Name: "Use", Package: "x", Target: use,
				Body: []*emit.Stmt{emit.NewRawStmt("_ = Registry.Field")},
			}},
		})
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if !diagnosticsContain(d, diag.Warn, `unresolved qualifier "Registry"`) {
			t.Fatalf("a name from another package must still be reported; got %+v", d.Diagnostics())
		}
	})

	t.Run("a name no sibling declares is still reported", func(t *testing.T) {
		t.Parallel()
		// The negative's control: without it, a union that swallowed
		// everything would pass the test above for the wrong reason.
		ctx, _, d := newBackendContext(t)
		use := emit.Target{Dir: "x", Filename: "use.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{{
				Name: "Use", Package: "x", Target: use,
				Body: []*emit.Stmt{emit.NewRawStmt("_ = Missing.Field")},
			}},
		})
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if !diagnosticsContain(d, diag.Warn, `unresolved qualifier "Missing"`) {
			t.Fatalf("expected a report for an undeclared name; got %+v", d.Diagnostics())
		}
	})
}

// TestRender_UnresolvedQualifierOrderIsStable pins the emission
// order. The candidates come out of a map, and `-diag-format json`
// makes the sequence observable, so an unsorted emission would make
// two runs of the same input produce different output.
func TestRender_UnresolvedQualifierOrderIsStable(t *testing.T) {
	t.Parallel()

	messages := func(t *testing.T) []string {
		t.Helper()
		ctx, _, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{{
				Name: "Use", Package: "x", Target: target,
				Body: []*emit.Stmt{
					emit.NewRawStmt("_ = zulu.A"),
					emit.NewRawStmt("_ = alpha.B"),
					emit.NewRawStmt("_ = mike.C"),
				},
			}},
		})
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		var out []string
		for _, dg := range d.Diagnostics() {
			if dg.Severity == diag.Warn && strings.Contains(dg.Message, "unresolved qualifier") {
				out = append(out, dg.Message)
			}
		}
		return out
	}

	t.Run("every unresolved qualifier is reported once", func(t *testing.T) {
		t.Parallel()
		if got := messages(t); len(got) != 3 {
			t.Fatalf("expected 3 reports; got %d: %v", len(got), got)
		}
	})

	t.Run("reports arrive sorted, not in body order", func(t *testing.T) {
		t.Parallel()
		got := messages(t)
		if !slices.IsSortedFunc(got, strings.Compare) {
			t.Fatalf("expected sorted reports; got %v", got)
		}
	})

	t.Run("two runs of the same input agree", func(t *testing.T) {
		t.Parallel()
		if first, second := messages(t), messages(t); !slices.Equal(first, second) {
			t.Fatalf("report order differed between runs:\n%v\n%v", first, second)
		}
	})
}

// TestRender_ShadowedNameSuppressesReport pins the documented false
// negative. A local sharing a package's name suppresses a genuine
// report elsewhere in the file, because `declared` is scope-blind.
//
// It is written down so a later "improvement" to the declared set
// has to argue with a test rather than with prose. The asymmetry is
// the point: a missed report costs a `go build` error the developer
// already gets, an invented one costs a maintainer an afternoon.
func TestRender_ShadowedNameSuppressesReport(t *testing.T) {
	t.Parallel()

	t.Run("a local named like a package suppresses its report", func(t *testing.T) {
		t.Parallel()
		ctx, _, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{{
				Name: "Use", Package: "x", Target: target,
				Body: []*emit.Stmt{
					emit.NewRawStmt("time := 1"),
					emit.NewRawStmt("_ = time"),
					emit.NewRawStmt("_ = time.Now()"),
				},
			}},
		})
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if diagnosticsContain(d, diag.Warn, `unresolved qualifier "time"`) {
			t.Fatalf("expected the shadowed name to suppress the report; got %+v", d.Diagnostics())
		}
	})
}
