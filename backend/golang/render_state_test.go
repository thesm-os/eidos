// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"errors"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/backend/golang"
	"go.thesmos.sh/eidos/emit"
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
		if golang.ErrTemplateMissing == nil {
			t.Fatalf("ErrTemplateMissing must be exported and non-nil")
		}
		if !errors.Is(golang.ErrTemplateMissing, golang.ErrTemplateMissing) {
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
		if err := golang.New().Render(ctx); err != nil {
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
	if err := golang.New().Render(ctx); err != nil {
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
