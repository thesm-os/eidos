// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang/backend"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
)

// TestErrTemplateMissing covers the sentinel surfaced when the
// render funcmap can't find a template for an entity's Kind. The
// sentinel itself is small but used by every render-dispatch
// branch — pinning its exported shape catches accidental rename
// or removal.
func TestErrTemplateMissing(t *testing.T) {
	t.Parallel()

	t.Run("ErrTemplateMissing is exported and satisfies errors.Is", func(t *testing.T) {
		t.Parallel()
		if backend.ErrTemplateMissing == nil {
			t.Fatalf("ErrTemplateMissing must be exported and non-nil")
		}
		if !errors.Is(backend.ErrTemplateMissing, backend.ErrTemplateMissing) {
			t.Fatalf("ErrTemplateMissing must satisfy errors.Is reflexivity")
		}
	})
}

// TestNewRenderState_LifecycleViaPublicPath verifies the
// clone-per-Target pattern works through the public render path:
// constructing a Backend and calling Render successfully exercises
// newRenderState's clone + funcmap binding once per Target. Any
// regression in the clone mechanism (e.g., shared mutable state
// across renders) would surface as cross-target leakage during a
// multi-target render.
func TestNewRenderState_LifecycleViaPublicPath(t *testing.T) {
	t.Parallel()

	t.Run("renders two distinct targets without cross-target leakage", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		t1 := emit.Target{Dir: "a", Filename: "foo.go", Package: "a"}
		t2 := emit.Target{Dir: "b", Filename: "foo.go", Package: "b"}
		addEmitPackage(t, ctx, emitPackage("a", emitStructWithFields(
			"a", "A", t1, fieldSpec{name: "F", builtin: "int"},
		)))
		addEmitPackage(t, ctx, emitPackage("b", emitStructWithFields(
			"b", "B", t2, fieldSpec{name: "G", builtin: "string"},
		)))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if d.HasErrors() {
			t.Fatalf("unexpected diagnostics: %+v", d.Diagnostics())
		}
		if mem.Len() != 2 {
			t.Fatalf("expected 2 sink writes; got %d", mem.Len())
		}
	})
}

// TestRender_ImpTracksAgainstTheLiveImportSet pins the invariant
// behind the `imp` funcmap entry — the documented way a plugin
// template declares an import.
//
// `imp` is the only funcmap entry that reaches the import set rather
// than the render state, and the import set is the one field
// [renderTarget] replaces per target. An entry bound as a method
// value on the field (`s.imports.Imp`) captures the pointer at
// funcmap-construction time and keeps writing into the object the
// state has since moved on from, so nothing the template declares
// reaches the file it is building.
//
// The two assertions are separate because they fail at different
// times. Only the tracking one catches a stale binding today:
// goimports repairs the output, so the rendered bytes are correct
// either way — until the resolve pass goes and the bytes stop
// compiling.
func TestRender_ImpTracksAgainstTheLiveImportSet(t *testing.T) {
	t.Parallel()

	// Two targets rather than one: the render state is constructed
	// once per worker and reset once per target, so a single-target
	// render is the weakest possible exercise of the reset.
	first := emit.Target{Dir: "a", Filename: "x.go", Package: "a"}
	second := emit.Target{Dir: "b", Filename: "x.go", Package: "b"}

	render := func(t *testing.T) (*sink.Memory, *diag.Sink) {
		t.Helper()
		ctx, mem, d := newBackendContext(t)
		ctx.Plugins = []plugin.Plugin{&stubTemplateProvider{
			name: "impgen",
			tmplFS: fstest.MapFS{
				"templates/golang/struct.tmpl": &fstest.MapFile{
					Data: []byte(`{{ define "emit.struct" -}}
type {{ .Name }} struct {
	Ctx {{ imp "context" }}.Context
}
{{- end -}}`),
				},
			},
		}}
		ctx.Ordered = ctx.Plugins
		addEmitPackage(t, ctx, emitPackage("a", emitStructWithFields("a", "A", first)))
		addEmitPackage(t, ctx, emitPackage("b", emitStructWithFields("b", "B", second)))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if d.HasErrors() {
			t.Fatalf("unexpected error diagnostics: %+v", d.Diagnostics())
		}
		return mem, d
	}

	t.Run("the backend tracks the import the template declared", func(t *testing.T) {
		t.Parallel()
		// goimports reporting that it added the import is the tell
		// that the backend never knew about it.
		_, d := render(t)
		if diagnosticsContain(d, diag.Warn, "goimports added untracked import") {
			t.Fatalf("imp wrote to an import set the file was not built from; got %+v", d.Diagnostics())
		}
	})

	t.Run("every rendered target carries the declared import", func(t *testing.T) {
		t.Parallel()
		mem, _ := render(t)
		for _, target := range []emit.Target{first, second} {
			body, ok := mem.Get(target)
			if !ok {
				t.Fatalf("no output for %s", target.JoinPath())
			}
			if !strings.Contains(string(body), `"context"`) {
				t.Fatalf("%s: declared import missing from:\n%s", target.JoinPath(), body)
			}
		}
	})
}

// TestRender_SelfPackageAliases pins import aliasing for the run's
// own outputs.
//
// Go binds an unaliased import to the package's *declared* name, not
// to the last segment of its path. The writer can only derive an
// alias from the path, so the two diverge exactly when a `pkg=`
// override renames an output away from its directory — the shape
// `+gen:stub out=testkit/ pkg=storetest` produces. Without an
// explicit alias the referring file renders `testkit.Thing` against
// an import that actually bound `storetest`, and does not compile.
func TestRender_SelfPackageAliases(t *testing.T) {
	t.Parallel()

	// renamed is the output whose directory (`testkit`) and package
	// clause (`storetest`) disagree. referrer lives elsewhere and
	// holds a field typed by a reference into it.
	build := func(t *testing.T) string {
		t.Helper()
		ctx, mem, _ := newBackendContext(t)

		renamed := emit.Target{
			Dir: "testkit", Filename: "thing.go",
			Package: "storetest", ImportPath: "example.com/p/testkit",
		}
		referrer := emit.Target{
			Dir: "app", Filename: "use.go",
			Package: "app", ImportPath: "example.com/p/app",
		}
		user := &emit.Struct{
			Name: "User", Package: "example.com/p/app", Target: referrer,
			Fields: []*emit.Field{{
				Name: "Thing",
				Type: emit.External("example.com/p/testkit", "Thing"),
			}},
		}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "p", Path: "example.com/p",
			Structs: []*emit.Struct{
				{Name: "Thing", Package: "example.com/p/testkit", Target: renamed},
				user,
			},
		})
		ctx.Store.Emit().RebuildByTarget()
		if err := backend.New().Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		got, ok := mem.Get(referrer)
		if !ok {
			t.Fatalf("no output for %s", referrer.JoinPath())
		}
		return string(got)
	}

	t.Run("the import carries the declared package name as an alias", func(t *testing.T) {
		t.Parallel()
		// Without the alias the import line is bare and Go silently
		// binds `storetest`, leaving the qualifier below undefined.
		if got := build(t); !strings.Contains(got, `storetest "example.com/p/testkit"`) {
			t.Fatalf("aliased import missing from:\n%s", got)
		}
	})

	t.Run("the reference qualifies with that same alias", func(t *testing.T) {
		t.Parallel()
		// The alias and the qualifier have to agree; asserting only
		// the import line would pass against a renderer that emitted
		// the alias and then qualified with the path segment.
		if got := build(t); !strings.Contains(got, "storetest.Thing") {
			t.Fatalf("qualifier not rendered against the alias:\n%s", got)
		}
	})
}

// TestRender_SelfPackageAliases_LeavesAgreeingPathsBare pins the
// negative: an output whose directory and package name agree must
// not acquire a redundant alias, or every generated import block in
// the common case grows noise gofmt will not remove.
func TestRender_SelfPackageAliases_LeavesAgreeingPathsBare(t *testing.T) {
	t.Parallel()

	ctx, mem, _ := newBackendContext(t)
	other := emit.Target{
		Dir: "other", Filename: "thing.go",
		Package: "other", ImportPath: "example.com/p/other",
	}
	referrer := emit.Target{
		Dir: "app", Filename: "use.go",
		Package: "app", ImportPath: "example.com/p/app",
	}
	addEmitPackage(t, ctx, &emit.Package{
		Name: "p", Path: "example.com/p",
		Structs: []*emit.Struct{
			{Name: "Thing", Package: "example.com/p/other", Target: other},
			{
				Name: "User", Package: "example.com/p/app", Target: referrer,
				Fields: []*emit.Field{{
					Name: "Thing",
					Type: emit.External("example.com/p/other", "Thing"),
				}},
			},
		},
	})
	ctx.Store.Emit().RebuildByTarget()
	if err := backend.New().Render(ctx); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got, ok := mem.Get(referrer)
	if !ok {
		t.Fatalf("no output for %s", referrer.JoinPath())
	}
	if strings.Contains(string(got), `other "example.com/p/other"`) {
		t.Fatalf("redundant alias emitted:\n%s", got)
	}
}
