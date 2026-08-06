// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"errors"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/backend/golang"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// TestFileCompose_SharedTargetTwoPlugins covers the happy-path of
// multi-generator file composition: two plugins each call
// AddPackage with one Struct routed to the same Target. The
// backend composes both decls into a single rendered file. Both
// Structs must appear, in plugin registration order (which
// corresponds to AddPackage call order since the store records
// insertion order for ByTarget).
func TestFileCompose_SharedTargetTwoPlugins(t *testing.T) {
	t.Parallel()

	t.Run("two AddPackage calls share a Target; both decls render", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "users"},
			stubPluginVersion{name: "events"},
		}
		target := emit.Target{Dir: "domain", Filename: "core.go", Package: "domain"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "domain.users", Path: "domain/users",
			Structs: []*emit.Struct{{
				Name: "User", Package: "domain/users", Target: target,
				Fields: []*emit.Field{{Name: "ID", Type: emit.Builtin("int")}},
			}},
		})
		addEmitPackage(t, ctx, &emit.Package{
			Name: "domain.events", Path: "domain/events",
			Structs: []*emit.Struct{{
				Name: "Event", Package: "domain/events", Target: target,
				Fields: []*emit.Field{{Name: "Kind", Type: emit.Builtin("string")}},
			}},
		})
		body := string(assertRenderSucceeds(t, ctx, mem, d, target))
		mustOrderedSubstrings(t, body, "type User struct {", "type Event struct {")
	})
}

// TestFileCompose_InitBlockFromThreePlugins covers init composition
// — three plugins each contribute one statement to the file's
// [emit.File.Init] slot; the backend composes them into a single
// `func init() { … }` block, topo-ordered.
func TestFileCompose_InitBlockFromThreePlugins(t *testing.T) {
	t.Parallel()

	t.Run("three plugins contribute init stmts; single init block emitted", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "logger"},
			stubPluginVersion{name: "metrics"},
			stubPluginVersion{name: "tracer"},
		}
		target := emit.Target{Dir: "boot", Filename: "boot.go", Package: "boot"}
		f, err := ctx.Store.Emit().FileFor(target)
		if err != nil {
			t.Fatalf("FileFor: %v", err)
		}
		appendInit := func(setBy, raw string) {
			if err := f.Init().Append(emit.NewRawStmt(raw), emit.Provenance{SetBy: setBy}); err != nil {
				t.Fatalf("append %s init: %v", setBy, err)
			}
		}
		// Out of topo order — render verifies re-sorting.
		appendInit("tracer", `_ = "tracer-init"`)
		appendInit("metrics", `_ = "metrics-init"`)
		appendInit("logger", `_ = "logger-init"`)
		// FileFor inserted the File with a zero-package; mirror the
		// Target so byTarget routing picks it up.
		f.Package = target.Package
		f.Name = target.Filename
		f.Dir = target.Dir
		body := string(assertRenderSucceeds(t, ctx, mem, d, target))
		mustOrderedSubstrings(
			t, body,
			"func init() {",
			`_ = "logger-init"`,
			`_ = "metrics-init"`,
			`_ = "tracer-init"`,
		)
		if strings.Count(body, "func init()") != 1 {
			t.Fatalf("expected exactly one init block; got:\n%s", body)
		}
	})

	t.Run("empty Init slot emits no init block", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target, fieldSpec{name: "F", builtin: "int"},
		)))
		body := string(assertRenderSucceeds(t, ctx, mem, d, target))
		if strings.Contains(body, "func init()") {
			t.Fatalf("file with empty Init slot must not emit func init(); got:\n%s", body)
		}
	})
}

// TestFileCompose_TopBottomSlots covers the file-level slot
// placement — the file's Top slot renders above the free-floating
// decls, Bottom below them, each in plugin-topo order.
func TestFileCompose_TopBottomSlots(t *testing.T) {
	t.Parallel()

	t.Run("top-only target renders header content above package decls", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{stubPluginVersion{name: "introgen"}}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		f := bindFile(t, ctx, target)
		topConst := &emit.Constant{
			Name: "Banner", Package: "x", Target: emit.Target{},
			Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitString, RawText: "hello"},
		}
		if err := f.Top().Append(topConst, emit.Provenance{SetBy: "introgen"}); err != nil {
			t.Fatalf("append top: %v", err)
		}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target, fieldSpec{name: "F", builtin: "int"},
		)))
		body := string(assertRenderSucceeds(t, ctx, mem, d, target))
		mustOrderedSubstrings(t, body, `const Banner = "hello"`, "type X struct {")
	})

	t.Run("bottom-only contribution renders below decls", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{stubPluginVersion{name: "footergen"}}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		f := bindFile(t, ctx, target)
		bot := &emit.Variable{
			Name: "Sentinel", Package: "x", Target: emit.Target{},
			Init: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitBool, RawText: "true"},
		}
		if err := f.Bottom().Append(bot, emit.Provenance{SetBy: "footergen"}); err != nil {
			t.Fatalf("append bottom: %v", err)
		}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target, fieldSpec{name: "F", builtin: "int"},
		)))
		body := string(assertRenderSucceeds(t, ctx, mem, d, target))
		mustOrderedSubstrings(t, body, "type X struct {", "var Sentinel = true")
	})
}

// TestFileCompose_EmptyTargetFilter covers the empty-target rule:
// a Target with no decls, no File slots populated produces no
// sink output. Confirms the filter applies through the public
// render path.
func TestFileCompose_EmptyTargetFilter(t *testing.T) {
	t.Parallel()

	t.Run("File with no decls and empty slots produces no output", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		// Create the File entity but contribute nothing to it; the
		// File itself routes through byTarget but carries no decls
		// or slot items, so the empty-target filter fires.
		bindFile(t, ctx, target)
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if d.HasErrors() {
			t.Fatalf("unexpected error diagnostics: %+v", d.Diagnostics())
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("empty Target must not produce a sink write")
		}
	})

	t.Run("ErrEmptyTarget is exported and satisfies errors.Is", func(t *testing.T) {
		t.Parallel()
		if golang.ErrEmptyTarget == nil {
			t.Fatalf("ErrEmptyTarget must be exported and non-nil")
		}
		if !errors.Is(golang.ErrEmptyTarget, golang.ErrEmptyTarget) {
			t.Fatalf("ErrEmptyTarget must satisfy errors.Is reflexivity")
		}
	})
}

// TestFileCompose_DuplicateQNameAcrossPlugins covers the cross-
// plugin duplicate-entity surface: two plugins emit decls with the
// same QName routed to the same Target. Today's store-level dedup
// catches the collision at AddPackage time with
// [store.ErrDuplicateQName]; this test pins that surface so the
// duplicate-entity discipline is observable from the public
// emit-view API.
func TestFileCompose_DuplicateQNameAcrossPlugins(t *testing.T) {
	t.Parallel()

	t.Run("two AddPackage calls for the same struct QName collide", func(t *testing.T) {
		t.Parallel()
		ctx, _, _ := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "Duplicate", target, fieldSpec{name: "F", builtin: "int"},
		)))
		err := ctx.Store.Emit().AddPackage(emitPackage("x", emitStructWithFields(
			"x", "Duplicate", target, fieldSpec{name: "G", builtin: "string"},
		)))
		if !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("expected ErrDuplicateQName from second AddPackage; got %v", err)
		}
	})
}

// TestFileCompose_ImportsSlotRegistersBeforeBody covers the pre-
// render imports pass: imports contributed via
// [emit.File.ImportsSlot] register with the writer's
// [writer.ImportSet] before any template fires, so the final
// import block carries the plugin-staged path even when no decl
// references it.
func TestFileCompose_ImportsSlotRegistersBeforeBody(t *testing.T) {
	t.Parallel()

	t.Run("plugin-staged blank import surfaces in the rendered import block", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{stubPluginVersion{name: "sideeffectgen"}}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		f := bindFile(t, ctx, target)
		// Side-effect-only imports (Go's `import _ "…"` form) are
		// the canonical reason ImportsSlot exists — they reference
		// no symbol, so the body has no `imp` call that would
		// otherwise pull them in.
		if err := f.ImportsSlot().Append(
			&emit.Import{Path: "embed", Alias: "_"},
			emit.Provenance{SetBy: "sideeffectgen"},
		); err != nil {
			t.Fatalf("append import: %v", err)
		}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target, fieldSpec{name: "F", builtin: "int"},
		)))
		body := string(assertRenderSucceeds(t, ctx, mem, d, target))
		if !strings.Contains(body, `_ "embed"`) {
			t.Fatalf("expected staged blank import `_ \"embed\"` in body; got:\n%s", body)
		}
	})
}

// TestFileCompose_DirectFileImports covers the
// directly-attached-imports branch of preRenderImports — entries
// on [emit.File.Imports] (as opposed to ImportsSlot) register
// through the writer's [writer.ImportSet] and surface in the
// rendered file. Uses a side-effect import (`_`) so goimports
// keeps the path even though no decl references it.
func TestFileCompose_DirectFileImports(t *testing.T) {
	t.Parallel()

	t.Run("file.Imports staged blank import surfaces in the rendered import block", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		f := bindFile(t, ctx, target)
		f.Imports = append(
			f.Imports,
			&emit.Import{Path: "embed", Alias: "_"},
		)
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target, fieldSpec{name: "F", builtin: "int"},
		)))
		body := string(assertRenderSucceeds(t, ctx, mem, d, target))
		if !strings.Contains(body, `_ "embed"`) {
			t.Fatalf("expected staged blank import `_ \"embed\"` in body; got:\n%s", body)
		}
	})
}

// TestFileCompose_Goldens pins canonical multi-plugin output for
// the two flagship file-composition scenarios — a shared Target
// with two plugin contributions and a three-plugin init block —
// so byte-level drift in file composition is caught at PR time.
func TestFileCompose_Goldens(t *testing.T) {
	t.Parallel()

	t.Run("file_compose_shared_target — two plugins, one file", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "users"},
			stubPluginVersion{name: "events"},
		}
		target := emit.Target{Dir: "domain", Filename: "core.go", Package: "domain"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "domain.users", Path: "domain/users",
			Structs: []*emit.Struct{{
				BaseEmit: emit.BaseEmit{DocLines: []string{"User is the canonical user record."}},
				Name:     "User", Package: "domain/users", Target: target,
				Fields: []*emit.Field{{Name: "ID", Type: emit.Builtin("int")}},
			}},
		})
		addEmitPackage(t, ctx, &emit.Package{
			Name: "domain.events", Path: "domain/events",
			Structs: []*emit.Struct{{
				BaseEmit: emit.BaseEmit{DocLines: []string{"Event is one entry on the timeline."}},
				Name:     "Event", Package: "domain/events", Target: target,
				Fields: []*emit.Field{{Name: "Kind", Type: emit.Builtin("string")}},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "file_compose_shared_target.go.golden"))
	})

	t.Run("file_compose_init_block — three-plugin init composition", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "logger"},
			stubPluginVersion{name: "metrics"},
			stubPluginVersion{name: "tracer"},
		}
		target := emit.Target{Dir: "boot", Filename: "boot.go", Package: "boot"}
		f := bindFile(t, ctx, target)
		appendInit := func(setBy, raw string) {
			if err := f.Init().Append(emit.NewRawStmt(raw), emit.Provenance{SetBy: setBy}); err != nil {
				t.Fatalf("append %s init: %v", setBy, err)
			}
		}
		appendInit("tracer", `_ = "tracer-init"`)
		appendInit("metrics", `_ = "metrics-init"`)
		appendInit("logger", `_ = "logger-init"`)
		addEmitPackage(t, ctx, emitPackage("boot", emitStructWithFields(
			"boot", "Boot", target, fieldSpec{name: "Ready", builtin: "bool"},
		)))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "file_compose_init_block.go.golden"))
	})
}

// renderWithImports renders one struct into target with the supplied
// File-level imports declared, and returns the rendered file plus
// the run's diagnostics. The struct's single field type is supplied
// by the caller so a test can decide whether the body references an
// imported package or not.
func renderWithImports(
	t *testing.T,
	target emit.Target,
	fieldType emit.Ref,
	imports ...*emit.Import,
) (string, *diag.Sink) {
	t.Helper()
	ctx, mem, d := newBackendContext(t)
	f := bindFile(t, ctx, target)
	f.Imports = append(f.Imports, imports...)
	addEmitPackage(t, ctx, emitPackage(target.Package, &emit.Struct{
		Name: "Holder", Package: target.Package, Target: target,
		Fields: []*emit.Field{{Name: "F", Type: fieldType}},
	}))
	if err := mustNew(t).Render(ctx); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if d.HasErrors() {
		t.Fatalf("unexpected error diagnostics: %+v", d.Diagnostics())
	}
	body, ok := mem.Get(target)
	if !ok {
		t.Fatalf("no output for %s", target.JoinPath())
	}
	return string(body), d
}

// TestRenderFile_PrunesUnreferencedImports pins the deletion the
// goimports resolve pass used to perform. An unused import is a
// compile error in Go, so an [emit.File.Imports] entry no template
// references must not reach the block — registration is
// unconditional and completes before a body byte is written, so
// nothing else ties an entry to a reference.
func TestRenderFile_PrunesUnreferencedImports(t *testing.T) {
	t.Parallel()

	target := emit.Target{Dir: "p", Filename: "x.go", Package: "p"}

	t.Run("an unreferenced import is absent from the block", func(t *testing.T) {
		t.Parallel()
		body, _ := renderWithImports(t, target, emit.Builtin("int"),
			&emit.Import{Path: "strings"})
		if strings.Contains(body, `"strings"`) {
			t.Fatalf("unreferenced import reached the block:\n%s", body)
		}
	})

	t.Run("a referenced import survives", func(t *testing.T) {
		t.Parallel()
		// The field type drives an ExternalRef, which registers the
		// import and emits the qualifier in the same breath.
		body, _ := renderWithImports(t, target,
			emit.External("example.com/kept", "Thing"))
		if !strings.Contains(body, `"example.com/kept"`) {
			t.Fatalf("referenced import was pruned:\n%s", body)
		}
	})

	t.Run("declaring an unused import changes no byte", func(t *testing.T) {
		t.Parallel()
		// The equivalence that matters: an entry nothing references
		// must leave the output exactly as if it had never been
		// declared. True today because goimports deletes it, true
		// afterwards because the prune does.
		with, _ := renderWithImports(t, target, emit.Builtin("int"),
			&emit.Import{Path: "strings"})
		without, _ := renderWithImports(t, target, emit.Builtin("int"))
		if with != without {
			t.Fatalf("unused import perturbed the output:\nwith:\n%s\nwithout:\n%s", with, without)
		}
	})
}

// TestRenderFile_KeepsSideEffectImports pins the side-effect shape.
// No body text can name a `_` or `.` import, so absence from the
// qualifier set carries no information and pruning on it would
// delete exactly the imports whose entire purpose is to be
// unreferenced.
func TestRenderFile_KeepsSideEffectImports(t *testing.T) {
	t.Parallel()

	target := emit.Target{Dir: "p", Filename: "x.go", Package: "p"}

	t.Run("two blank imports both survive", func(t *testing.T) {
		t.Parallel()
		// Two, not one: the second is the case the collision loop
		// used to rename to `_2`, which goimports then deleted as an
		// unused named import — silently dropping a side effect.
		body, _ := renderWithImports(t, target, emit.Builtin("int"),
			&emit.Import{Path: "embed", Alias: "_"},
			&emit.Import{Path: "net/http/pprof", Alias: "_"})
		for _, want := range []string{`_ "embed"`, `_ "net/http/pprof"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected %s in:\n%s", want, body)
			}
		}
	})

	t.Run("neither blank import is renamed", func(t *testing.T) {
		t.Parallel()
		body, _ := renderWithImports(t, target, emit.Builtin("int"),
			&emit.Import{Path: "embed", Alias: "_"},
			&emit.Import{Path: "net/http/pprof", Alias: "_"})
		if strings.Contains(body, "_2") {
			t.Fatalf("a blank import was suffixed:\n%s", body)
		}
	})

	t.Run("a dot import survives", func(t *testing.T) {
		t.Parallel()
		body, _ := renderWithImports(t, target, emit.Builtin("int"),
			&emit.Import{Path: "example.com/dot", Alias: "."})
		if !strings.Contains(body, `. "example.com/dot"`) {
			t.Fatalf("dot import was pruned:\n%s", body)
		}
	})
}

// TestRenderFile_ShadowedImportIsKeptAndReported pins the one case
// the rule cannot judge. A local sharing an import's alias produces
// selectors of its own, so an unreferenced import reads as
// referenced and survives.
//
// Keeping it is deliberate — dropping an import on an ambiguous
// reading is the direction that changes what compiling code does.
// The Warn is what stops that from being silent: goimports used to
// resolve this correctly and delete the import, so without a
// diagnostic the replacement would trade a repair for an
// unattributed build failure.
func TestRenderFile_ShadowedImportIsKeptAndReported(t *testing.T) {
	t.Parallel()

	target := emit.Target{Dir: "p", Filename: "x.go", Package: "p"}

	// A method body binding a local named `strings` and selecting on
	// it. The import is declared, never genuinely used, and kept.
	build := func(t *testing.T) (string, *diag.Sink) {
		t.Helper()
		ctx, mem, d := newBackendContext(t)
		f := bindFile(t, ctx, target)
		f.Imports = append(f.Imports, &emit.Import{Path: "strings"})
		addEmitPackage(t, ctx, &emit.Package{
			Name: "p", Path: "p",
			Methods: []*emit.Method{{
				Name: "Shadow", Package: "p", Target: target,
				Receiver: emit.Builtin("Holder"), ReceiverName: "h",
				Body: []*emit.Stmt{
					emit.NewRawStmt("strings := newThing()"),
					emit.NewRawStmt("_ = strings.Field"),
				},
			}},
			Structs: []*emit.Struct{{Name: "Holder", Package: "p", Target: target}},
		})
		// Top-level methods index under an empty Target until the
		// routing layer has stamped one; the backend reads
		// ByTarget, so without the rebuild the method never renders.
		ctx.Store.Emit().RebuildByTarget()
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		body, ok := mem.Get(target)
		if !ok {
			t.Fatalf("no output for %s", target.JoinPath())
		}
		return string(body), d
	}

	t.Run("the shadowed import survives into the output", func(t *testing.T) {
		t.Parallel()
		// The resolve pass used to resolve the shadowing properly and
		// delete the import. Nothing does now, and keeping it is the
		// deliberate choice: dropping an import on an ambiguous
		// reading is the direction that changes what compiling code
		// does. The file fails to build with "imported and not used",
		// and the Warn below is what points at why.
		body, _ := build(t)
		if !strings.Contains(body, `"strings"`) {
			t.Fatalf("shadowed import was dropped:\n%s", body)
		}
	})

	t.Run("a Warn names the import and the shadowing name", func(t *testing.T) {
		t.Parallel()
		_, d := build(t)
		if !diagnosticsContain(d, diag.Warn, `import "strings" is unused but kept`) {
			t.Fatalf("expected a shadowed-import Warn; got %+v", d.Diagnostics())
		}
	})
}
