// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/priority"
	"go.thesmos.sh/eidos/sink"
	"go.thesmos.sh/eidos/store"
)

func TestPipeline_Run_NoSink(t *testing.T) {
	t.Parallel()

	t.Run("returns ErrNoSink when no sink was configured", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		assertNoError(t, err)
		if got := p.Run(t.Context()); !errors.Is(got, pipeline.ErrNoSink) {
			t.Fatalf("Run without sink should return ErrNoSink; got %v", got)
		}
	})
}

func TestPipeline_Run_FrontendPatterns(t *testing.T) {
	t.Parallel()

	t.Run("each frontend receives every supplied pattern in order", func(t *testing.T) {
		t.Parallel()
		fe := &recFE{name: "fe"}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "pkg/a", "pkg/b"))
		if !slices.Equal(fe.loaded, []string{"pkg/a", "pkg/b"}) {
			t.Fatalf("frontend should see every pattern in order; got %v", fe.loaded)
		}
	})

	t.Run("multiple frontends each see every pattern", func(t *testing.T) {
		t.Parallel()
		fe1 := &recFE{name: "fe1"}
		fe2 := &recFE{name: "fe2"}
		p, err := pipeline.New().
			WithFrontend(fe1).
			WithFrontend(fe2).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if !slices.Equal(fe1.loaded, []string{"x"}) || !slices.Equal(fe2.loaded, []string{"x"}) {
			t.Fatalf("each frontend should receive every pattern; got fe1=%v fe2=%v", fe1.loaded, fe2.loaded)
		}
	})

	t.Run("a frontend Load error becomes an Error diagnostic and Run returns ErrRunHadErrors", func(t *testing.T) {
		t.Parallel()
		fe := &recFE{name: "fe", err: errors.New("load failed")}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		got := p.Run(t.Context(), "pkg")
		if !errors.Is(got, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run should return ErrRunHadErrors after a frontend failure; got %v", got)
		}
		if p.Diag().Count(diag.Error) == 0 {
			t.Fatalf("frontend error should surface as a diag.Error")
		}
	})
}

func TestPipeline_Run_AnnotatorPhase(t *testing.T) {
	t.Parallel()

	t.Run("every annotator runs in plan order against the shared store", func(t *testing.T) {
		t.Parallel()
		var order []string
		mark := func(name string) func(*plugin.AnnotatorContext) {
			return func(*plugin.AnnotatorContext) { order = append(order, name) }
		}
		ann1 := &recAnn{name: "ann1", annotate: mark("ann1")}
		ann2 := &recAnn{name: "ann2", annotate: mark("ann2")}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithAnnotator(ann1).
			WithAnnotator(ann2).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if !slices.Equal(order, []string{"ann1", "ann2"}) {
			t.Fatalf("annotator plan order mismatch: %v", order)
		}
	})

	t.Run("an annotator error becomes an Error diagnostic and execution continues", func(t *testing.T) {
		t.Parallel()
		ann1 := &recAnn{name: "ann1", err: errors.New("first failed")}
		ann2 := &recAnn{name: "ann2"}
		be := &recBE{name: "be", lang: "stub"}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithAnnotator(ann1).
			WithAnnotator(ann2).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		got := p.Run(t.Context())
		if !errors.Is(got, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run should return ErrRunHadErrors; got %v", got)
		}
		if ann2.calls != 1 {
			t.Fatalf("ann2 should still run after ann1 fails; calls=%d", ann2.calls)
		}
		if be.calls != 1 {
			t.Fatalf("backend should still run after ann1 fails; calls=%d", be.calls)
		}
	})
}

func TestPipeline_Run_GeneratorPhase(t *testing.T) {
	t.Parallel()

	t.Run("every generator runs in plan order", func(t *testing.T) {
		t.Parallel()
		var order []string
		mark := func(name string) func(*plugin.GeneratorContext) {
			return func(*plugin.GeneratorContext) { order = append(order, name) }
		}
		g1 := &recGen{name: "g1", generate: mark("g1")}
		g2 := &recGen{name: "g2", generate: mark("g2")}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(g1).
			WithGenerator(g2).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if !slices.Equal(order, []string{"g1", "g2"}) {
			t.Fatalf("generator plan order mismatch: %v", order)
		}
	})

	t.Run("a generator error becomes an Error diagnostic and execution continues", func(t *testing.T) {
		t.Parallel()
		g1 := &recGen{name: "g1", err: errors.New("first failed")}
		g2 := &recGen{name: "g2"}
		be := &recBE{name: "be", lang: "stub"}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(g1).
			WithGenerator(g2).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		_ = p.Run(t.Context())
		if g2.calls != 1 {
			t.Fatalf("g2 should still run after g1 fails; calls=%d", g2.calls)
		}
		if be.calls != 1 {
			t.Fatalf("backend should still run after g1 fails; calls=%d", be.calls)
		}
	})
}

func TestPipeline_Run_BackendPhase(t *testing.T) {
	t.Parallel()

	t.Run("backend receives the configured sink and registered/ordered plugin lists", func(t *testing.T) {
		t.Parallel()
		var seenCtx *plugin.BackendContext
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) { seenCtx = ctx },
		}
		ann := &stubAnn{name: "ann"}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithAnnotator(ann).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if seenCtx == nil {
			t.Fatalf("backend should have been called")
		}
		if seenCtx.Sink == nil {
			t.Fatalf("BackendContext.Sink should be the configured sink")
		}
		if seenCtx.Lang != "stub" {
			t.Fatalf("BackendContext.Lang = %q, want stub", seenCtx.Lang)
		}
		if len(seenCtx.Plugins) == 0 || len(seenCtx.Ordered) == 0 {
			t.Fatalf("Plugins / Ordered should be populated; got %+v", seenCtx)
		}
	})

	t.Run("a backend Render error becomes an Error diagnostic", func(t *testing.T) {
		t.Parallel()
		be := &recBE{name: "be", lang: "stub", err: errors.New("render failed")}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		got := p.Run(t.Context())
		if !errors.Is(got, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run should return ErrRunHadErrors; got %v", got)
		}
	})

	t.Run("WithCommand pins the BackendContext.Command field", func(t *testing.T) {
		t.Parallel()
		var seenCtx *plugin.BackendContext
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) { seenCtx = ctx },
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			WithCommand("(library)").
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if seenCtx.Command != "(library)" {
			t.Fatalf("BackendContext.Command = %q, want %q", seenCtx.Command, "(library)")
		}
	})

	t.Run("WithSourceRoot pins the BackendContext.SourceRoot field", func(t *testing.T) {
		t.Parallel()
		var seenCtx *plugin.BackendContext
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) { seenCtx = ctx },
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			WithSourceRoot("/home/dev/proj").
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if seenCtx.SourceRoot != "/home/dev/proj" {
			t.Fatalf("BackendContext.SourceRoot = %q, want %q", seenCtx.SourceRoot, "/home/dev/proj")
		}
	})

	t.Run("unset WithSourceRoot resolves to os.Getwd at Build time", func(t *testing.T) {
		t.Parallel()
		var seenCtx *plugin.BackendContext
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) { seenCtx = ctx },
		}
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("os.Getwd: %v", err)
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if seenCtx.SourceRoot != wd {
			t.Fatalf(
				"unset SourceRoot should resolve to os.Getwd at Build time; got %q, want %q",
				seenCtx.SourceRoot, wd,
			)
		}
	})

	t.Run("empty WithCommand falls back to commandLine derivation", func(t *testing.T) {
		t.Parallel()
		var seenCtx *plugin.BackendContext
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) { seenCtx = ctx },
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		// Empty WithCommand → pipeline falls back to commandLine().
		// commandLine() returns either the joined os.Args[1:] or
		// "(library)"; either way the value is non-empty so the
		// fallback path is exercised.
		if seenCtx.Command == "" {
			t.Fatalf("Command fallback should produce a non-empty value; got empty string")
		}
	})
}

func TestPipeline_Run_EndToEnd(t *testing.T) {
	t.Parallel()

	t.Run("frontend -> annotator -> generator -> backend pipes data through the store and sink", func(t *testing.T) {
		t.Parallel()
		mem := sink.NewMemory()
		// Frontend populates a node.Package with one struct stamped
		// at a deterministic source position; the Layout phase
		// derives Target.Dir from that position and Target.Package
		// from the package lookup. The emit-side hook below pins
		// the same struct via the OriginNode field so Layout knows
		// where to route its output.
		srcStruct := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/user.go"}},
			Name:     "User", Package: "x",
		}
		fe := &recFE{
			name: "fe",
			loadFn: func(s *store.Store) {
				_ = s.Nodes().AddPackage(&node.Package{
					Name: "x", Path: "x",
					Structs: []*node.Struct{srcStruct},
				})
			},
		}
		// Generator emits one struct whose Origin is the loaded
		// source struct. Target stamping is left to the Layout
		// phase — composeTarget produces
		// {Dir: "x", Filename: "user_gen.go", Package: "x",
		// ImportPath: "x"} from the source position + the
		// generator's "_gen.go" suffix.
		gen := &recGen{
			name: "gen",
			generate: func(ctx *plugin.GeneratorContext) {
				_ = ctx.Store.Emit().AddPackage(&emit.Package{
					Name: "x", Path: "x", Dir: "x",
					Structs: []*emit.Struct{{
						BaseEmit: emit.BaseEmit{OriginNode: srcStruct, SetByName: "gen"},
						Name:     "User", Package: "x",
					}},
				})
			},
		}
		// Backend writes one byte payload per emit struct.
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				ctx.Reader.EmitStructs().Each(func(s *emit.Struct) {
					_ = ctx.Sink.Write(s.Target, []byte("rendered:"+s.Name))
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithGenerator(gen).
			WithBackend(be).
			WithSink(mem).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		got, ok := mem.Get(emit.Target{
			Dir: "x", Filename: "user_gen.go", Package: "x", ImportPath: "x",
		})
		if !ok || string(got) != "rendered:User" {
			t.Fatalf("end-to-end output mismatch: %q ok=%v", got, ok)
		}
	})
}

func TestPipeline_DryRun(t *testing.T) {
	t.Parallel()

	t.Run("returns the resolved Plan without executing any phase", func(t *testing.T) {
		t.Parallel()
		fe := &recFE{name: "fe"}
		ann := &recAnn{name: "ann"}
		gen := &recGen{name: "gen"}
		be := &recBE{name: "be", lang: "stub"}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithAnnotator(ann).
			WithGenerator(gen).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		got := p.DryRun(t.Context())
		if got == nil || got.Backend == nil {
			t.Fatalf("DryRun should return the resolved plan; got %+v", got)
		}
		if len(fe.loaded) != 0 || ann.calls != 0 || gen.calls != 0 || be.calls != 0 {
			t.Fatalf("DryRun must not execute any phase")
		}
	})
}

func TestPipeline_Run_PanicRecovery(t *testing.T) {
	t.Parallel()

	t.Run("a panicking frontend becomes an Error diagnostic and the run continues", func(t *testing.T) {
		t.Parallel()
		be := &recBE{name: "be", lang: "stub"}
		p, err := pipeline.New().
			WithFrontend(&panickyFE{name: "fe", msg: "fe boom"}).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		got := p.Run(t.Context(), "pkg")
		if !errors.Is(got, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run should return ErrRunHadErrors after a panic; got %v", got)
		}
		if be.calls != 1 {
			t.Fatalf("backend should still run after a frontend panic; calls=%d", be.calls)
		}
		if !hasPanicMessage(p.Diag(), "fe boom") {
			t.Fatalf("panic message should be captured in diagnostics")
		}
	})

	t.Run("a panicking annotator becomes an Error diagnostic and subsequent phases run", func(t *testing.T) {
		t.Parallel()
		be := &recBE{name: "be", lang: "stub"}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithAnnotator(&panickyAnn{name: "ann", msg: "ann boom"}).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		_ = p.Run(t.Context())
		if be.calls != 1 {
			t.Fatalf("backend should still run after an annotator panic")
		}
		if !hasPanicMessage(p.Diag(), "ann boom") {
			t.Fatalf("annotator panic message not captured")
		}
	})

	t.Run("a panicking generator becomes an Error diagnostic and the backend still runs", func(t *testing.T) {
		t.Parallel()
		be := &recBE{name: "be", lang: "stub"}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&panickyGen{name: "gen", msg: "gen boom"}).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		_ = p.Run(t.Context())
		if be.calls != 1 {
			t.Fatalf("backend should still run after a generator panic")
		}
		if !hasPanicMessage(p.Diag(), "gen boom") {
			t.Fatalf("generator panic message not captured")
		}
	})

	t.Run("a panicking generator does not abort peers in the same phase", func(t *testing.T) {
		t.Parallel()
		ranA := false
		ranC := false
		be := &recBE{name: "be", lang: "stub"}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&recGen{name: "before", generate: func(*plugin.GeneratorContext) {
				ranA = true
			}}).
			WithGenerator(&panickyGen{name: "boom", msg: "phase peer boom"}).
			WithGenerator(&recGen{name: "after", generate: func(*plugin.GeneratorContext) {
				ranC = true
			}}).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		runErr := p.Run(t.Context())
		if !errors.Is(runErr, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run should return ErrRunHadErrors after a generator panic; got %v", runErr)
		}
		if !ranA {
			t.Fatalf("pre-panic generator should have run")
		}
		if !ranC {
			t.Fatalf("post-panic generator should have run despite peer panic")
		}
		if be.calls != 1 {
			t.Fatalf("backend should still run after a peer panic; calls=%d", be.calls)
		}
		if !hasPanicMessage(p.Diag(), "phase peer boom") {
			t.Fatalf("panic message should appear in diagnostics")
		}
	})

	t.Run("a panicking backend becomes an Error diagnostic", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&panickyBE{name: "be", lang: "stub", msg: "be boom"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		got := p.Run(t.Context())
		if !errors.Is(got, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run should return ErrRunHadErrors after a backend panic; got %v", got)
		}
		if !hasPanicMessage(p.Diag(), "be boom") {
			t.Fatalf("backend panic message not captured")
		}
	})
}

func TestPipeline_Run_FreezeContract(t *testing.T) {
	t.Parallel()

	t.Run("an annotator that mutates the frozen node view produces an Internal diagnostic", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithAnnotator(&frozenAddAnn{name: "bad-ann"}).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		_ = p.Run(t.Context())
		if len(internalDiagsFor(p.Diag())) == 0 {
			t.Fatalf("frozen-store violation should surface as an Internal diagnostic")
		}
	})

	t.Run("a backend that mutates the frozen emit view produces an Internal diagnostic", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&frozenAddBE{name: "bad-be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		_ = p.Run(t.Context())
		if len(internalDiagsFor(p.Diag())) == 0 {
			t.Fatalf("frozen-emit violation should surface as an Internal diagnostic")
		}
	})
}

func TestPipeline_Run_VerbosePhaseLogs(t *testing.T) {
	t.Parallel()

	t.Run("verbose mode emits one phase-boundary Info per phase plus a run summary", func(t *testing.T) {
		t.Parallel()
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			WithDiag(d).
			WithVerbose(true).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if got := phaseLogs(d); len(got) != 5 {
			t.Fatalf("expected 5 phase logs (FE/Ann/Override/Gen/BE); got %d: %v", len(got), got)
		}
		// Plus the run-summary Info; total Info >= 6.
		if d.Count(diag.Info) < 6 {
			t.Fatalf("expected at least 6 Info diagnostics (5 phases + summary); got %d", d.Count(diag.Info))
		}
	})

	t.Run("non-verbose runs emit no Info diagnostics", func(t *testing.T) {
		t.Parallel()
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			WithDiag(d).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if d.Count(diag.Info) != 0 {
			t.Fatalf("non-verbose run should emit no Info diagnostics; got %d", d.Count(diag.Info))
		}
	})
}

func TestPipeline_Run_ParallelFrontends(t *testing.T) {
	t.Parallel()

	t.Run("multiple frontends + patterns dispatch concurrently when PhaseFrontend is opted in", func(t *testing.T) {
		t.Parallel()
		fe1 := &recFE{name: "fe1"}
		fe2 := &recFE{name: "fe2"}
		p, err := pipeline.New().
			WithFrontend(fe1).
			WithFrontend(fe2).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			WithParallel(pipeline.PhaseFrontend).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "a", "b"))
		// 2 frontends × 2 patterns = 4 Load calls total across both.
		if len(fe1.loaded)+len(fe2.loaded) != 4 {
			t.Fatalf("expected 4 Load calls across both frontends; got fe1=%v fe2=%v", fe1.loaded, fe2.loaded)
		}
	})
}

func TestPipeline_Run_ParallelAnnotators(t *testing.T) {
	t.Parallel()

	t.Run("disjoint Provides + WithParallel runs the bucket concurrently", func(t *testing.T) {
		t.Parallel()
		ann1 := &stubAnnCapRec{
			name: "a", priority: priority.AnnotatorShape, provides: []string{"x"},
		}
		ann2 := &stubAnnCapRec{
			name: "b", priority: priority.AnnotatorShape, provides: []string{"y"},
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithAnnotator(ann1).
			WithAnnotator(ann2).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			WithParallel(pipeline.PhaseAnnotator).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if ann1.calls != 1 || ann2.calls != 1 {
			t.Fatalf("both annotators should run; got a=%d b=%d", ann1.calls, ann2.calls)
		}
	})

	t.Run("overlapping Provides forces sequential within the bucket", func(t *testing.T) {
		t.Parallel()
		ann1 := &stubAnnCapRec{
			name: "a", priority: priority.AnnotatorShape, provides: []string{"shared"},
		}
		// Different name → not a duplicate-provider Build error,
		// but Provides overlap → the parallel-safe check rejects.
		ann2 := &stubAnnCapRec{
			name: "b", priority: priority.AnnotatorRefinement, provides: []string{"shared"},
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithAnnotator(ann1).
			WithAnnotator(ann2).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			WithParallel(pipeline.PhaseAnnotator).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if ann1.calls != 1 || ann2.calls != 1 {
			t.Fatalf("both annotators should still run; got a=%d b=%d", ann1.calls, ann2.calls)
		}
	})
}

func TestPipeline_Run_ParallelGenerators(t *testing.T) {
	t.Parallel()

	t.Run("all NodesOnly generators in a bucket run concurrently", func(t *testing.T) {
		t.Parallel()
		g1 := &stubGenNodesOnly{name: "g1", priority: priority.GeneratorFoundation, nodesOnly: true}
		g2 := &stubGenNodesOnly{name: "g2", priority: priority.GeneratorFoundation, nodesOnly: true}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(g1).
			WithGenerator(g2).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			WithParallel(pipeline.PhaseGenerator).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if g1.calls != 1 || g2.calls != 1 {
			t.Fatalf("both generators should run; got g1=%d g2=%d", g1.calls, g2.calls)
		}
	})

	t.Run("a non-NodesOnly generator forces sequential within the bucket", func(t *testing.T) {
		t.Parallel()
		g1 := &stubGenNodesOnly{name: "g1", priority: priority.GeneratorFoundation, nodesOnly: true}
		// g2 doesn't implement NodesOnly → bucket falls back to sequential.
		g2 := &recGen{name: "g2"}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(g1).
			WithGenerator(g2).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			WithParallel(pipeline.PhaseGenerator).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if g1.calls != 1 || g2.calls != 1 {
			t.Fatalf("both generators should still run; got g1=%d g2=%d", g1.calls, g2.calls)
		}
	})
}

func TestPipeline_Run_RecordsCacheKeys(t *testing.T) {
	t.Parallel()

	t.Run("each plugin's ReadSet hash is written to the cache", func(t *testing.T) {
		t.Parallel()
		c := &recordingCache{}
		ann := &recAnn{name: "ann"}
		gen := &recGen{name: "gen"}
		be := &recBE{name: "be", lang: "stub"}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithAnnotator(ann).
			WithGenerator(gen).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			WithCache(c).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		// Three plugins (ann, gen, be) each record a cache key; the
		// frontend phase does not record one because Frontend.Load
		// does not receive a Reader. Each recorded key includes the
		// ReadSet hash plus the routing fingerprint composed from
		// the resolved layout policy and run-wide scope.
		for _, name := range []string{"ann", "gen", "be"} {
			key := c.keyFor(t, name)
			wantSub := "reads:" + emptyReadSetHash()
			if !strings.Contains(key, wantSub) {
				t.Errorf("plugin %q cache key %q missing %q", name, key, wantSub)
			}
		}
	})
}

func TestPipeline_Run_ParallelAnnotatorsWithPlainPlugin(t *testing.T) {
	t.Parallel()

	t.Run("a non-CapabilityProvider annotator still runs in parallel mode (no conflict)", func(t *testing.T) {
		t.Parallel()
		// stubAnn doesn't implement CapabilityProvider so its
		// Provides set is empty; disjoint check accepts.
		plain := &recAnn{name: "plain"}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithAnnotator(plain).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			WithParallel(pipeline.PhaseAnnotator).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if plain.calls != 1 {
			t.Fatalf("plain annotator should run; calls=%d", plain.calls)
		}
	})
}

// TestPipeline_Run_ManifestScopePreserve pins the
// scope-aware manifest merge: a narrow-pattern run must NOT
// wipe prior entries for packages the current run did not
// load. Without the merge, `eidos run ./sub/...` after a
// prior `eidos run ./...` would shrink the manifest to just
// the sub/ entries, orphaning everything else from prune /
// drift tracking.
func TestPipeline_Run_ManifestScopePreserve(t *testing.T) {
	t.Parallel()

	t.Run("narrow re-run preserves out-of-scope entries from prior wide run", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		manifestPath := filepath.Join(tmp, "manifest.json")

		// fePkg builds a frontend that loads a single source
		// package whose Name == Path == pkg, carrying one
		// emit-able struct.
		fePkg := func(pkg string) *recFE {
			return &recFE{
				name: "fe",
				loadFn: func(s *store.Store) {
					_ = s.Nodes().AddPackage(&node.Package{
						Name: pkg, Path: pkg,
						Structs: []*node.Struct{{
							BaseNode: node.BaseNode{SourcePos: position.Pos{File: pkg + "/x.go"}},
							Name:     "X", Package: pkg,
						}},
					})
				},
			}
		}
		// Mirror generator: one emit struct per source struct,
		// stamped with the source ImportPath so the manifest
		// entry carries Target.ImportPath = pkg.
		gen := &recGen{
			name: "gen",
			generate: func(ctx *plugin.GeneratorContext) {
				ctx.Reader.Structs().Each(func(srcStruct *node.Struct) {
					_ = ctx.Store.Emit().AddPackage(&emit.Package{
						Name: srcStruct.Package, Path: srcStruct.Package, Dir: srcStruct.Package,
						Structs: []*emit.Struct{{
							BaseEmit: emit.BaseEmit{OriginNode: srcStruct, SetByName: "gen"},
							Name:     srcStruct.Name, Package: srcStruct.Package,
						}},
					})
				})
			},
		}
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				ctx.Reader.EmitStructs().Each(func(s *emit.Struct) {
					_ = ctx.Sink.Write(s.Target, []byte("rendered:"+s.Name))
				})
			},
		}
		buildPipe := func(fe *recFE) *pipeline.Pipeline {
			p, err := pipeline.New().
				WithFrontend(fe).
				WithGenerator(gen).
				WithBackend(be).
				WithSink(sink.NewMemory()).
				WithManifestPath(manifestPath).
				Build()
			assertNoError(t, err)
			return p
		}

		// First run: load package "a" → manifest gets one entry.
		assertNoError(t, buildPipe(fePkg("a")).Run(t.Context(), "a"))
		// Second run: load package "b" only → without scope-merge
		// this would WIPE the "a" entry; with scope-merge it
		// preserves "a" and adds "b".
		assertNoError(t, buildPipe(fePkg("b")).Run(t.Context(), "b"))

		m, err := manifest.Read(manifestPath)
		assertNoError(t, err)
		importPaths := make([]string, 0, len(m.Outputs))
		for _, o := range m.Outputs {
			importPaths = append(importPaths, o.Target.ImportPath)
		}
		if !slices.Contains(importPaths, "a") {
			t.Errorf("narrow re-run wiped out-of-scope entry; "+
				"manifest ImportPaths = %v, want includes 'a'", importPaths)
		}
		if !slices.Contains(importPaths, "b") {
			t.Errorf("narrow re-run missing current entry; "+
				"manifest ImportPaths = %v, want includes 'b'", importPaths)
		}
	})

	t.Run("two pipelines sharing one manifest coexist (testkit-style multi-pipeline)", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		manifestPath := filepath.Join(tmp, "manifest.json")

		// Same source loaded by both pipelines. Each pipeline
		// produces a different Target (different filename suffix
		// via the rendering closure) and is identified by a
		// distinct WithPipelineID so the merge preserves both.
		fe := func() *recFE {
			return &recFE{
				name: "fe",
				loadFn: func(s *store.Store) {
					_ = s.Nodes().AddPackage(&node.Package{
						Name: "a", Path: "a",
						Structs: []*node.Struct{{
							BaseNode: node.BaseNode{SourcePos: position.Pos{File: "a/x.go"}},
							Name:     "X", Package: "a",
						}},
					})
				},
			}
		}
		genFn := func(name string) *recGen {
			return &recGen{
				name: name,
				generate: func(ctx *plugin.GeneratorContext) {
					ctx.Reader.Structs().Each(func(srcStruct *node.Struct) {
						_ = ctx.Store.Emit().AddPackage(&emit.Package{
							Name: srcStruct.Package,
							Path: srcStruct.Package,
							Dir:  srcStruct.Package,
							Structs: []*emit.Struct{{
								BaseEmit: emit.BaseEmit{OriginNode: srcStruct, SetByName: name},
								Name:     srcStruct.Name + "_" + name, Package: srcStruct.Package,
							}},
						})
					})
				},
			}
		}
		beFn := func() *recBE {
			return &recBE{
				name: "be", lang: "stub",
				render: func(ctx *plugin.BackendContext) {
					ctx.Reader.EmitStructs().Each(func(s *emit.Struct) {
						_ = ctx.Sink.Write(s.Target, []byte("rendered:"+s.Name))
					})
				},
			}
		}
		buildPipe := func(id, genName string) *pipeline.Pipeline {
			p, err := pipeline.New().
				WithFrontend(fe()).
				WithGenerator(genFn(genName)).
				WithBackend(beFn()).
				WithSink(sink.NewMemory()).
				WithManifestPath(manifestPath).
				WithPipelineID(id).
				Build()
			assertNoError(t, err)
			return p
		}

		// Pipeline "bench" runs first, claims its entry.
		assertNoError(t, buildPipe("bench", "bench-gen").Run(t.Context(), "a"))
		// Pipeline "suite" runs second against the SAME source
		// package. Without PipelineID-scoped merge, the bench
		// entry would be wiped (same scope, no plugin-id distinction).
		assertNoError(t, buildPipe("suite", "suite-gen").Run(t.Context(), "a"))

		m, err := manifest.Read(manifestPath)
		assertNoError(t, err)
		var benchCount, suiteCount int
		for _, o := range m.Outputs {
			switch o.PipelineID {
			case "bench":
				benchCount++
			case "suite":
				suiteCount++
			}
		}
		if benchCount == 0 {
			t.Errorf("bench pipeline's entry must survive when suite pipeline re-runs same scope")
		}
		if suiteCount == 0 {
			t.Errorf("suite pipeline's entry must be present after its run")
		}
	})

	t.Run("re-run with identical scope replaces prior entry for same Target", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		manifestPath := filepath.Join(tmp, "manifest.json")

		// Generator emits with a varying name so successive runs
		// produce different hashes for the same Target — proves
		// the merge replaces (rather than duplicates) when scope
		// matches.
		var renderTag string
		fe := &recFE{
			name: "fe",
			loadFn: func(s *store.Store) {
				_ = s.Nodes().AddPackage(&node.Package{
					Name: "a", Path: "a",
					Structs: []*node.Struct{{
						BaseNode: node.BaseNode{SourcePos: position.Pos{File: "a/x.go"}},
						Name:     "X", Package: "a",
					}},
				})
			},
		}
		gen := &recGen{
			name: "gen",
			generate: func(ctx *plugin.GeneratorContext) {
				ctx.Reader.Structs().Each(func(srcStruct *node.Struct) {
					_ = ctx.Store.Emit().AddPackage(&emit.Package{
						Name: srcStruct.Package, Path: srcStruct.Package, Dir: srcStruct.Package,
						Structs: []*emit.Struct{{
							BaseEmit: emit.BaseEmit{OriginNode: srcStruct, SetByName: "gen"},
							Name:     srcStruct.Name, Package: srcStruct.Package,
						}},
					})
				})
			},
		}
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				ctx.Reader.EmitStructs().Each(func(s *emit.Struct) {
					_ = ctx.Sink.Write(s.Target, []byte("rendered:"+s.Name+":"+renderTag))
				})
			},
		}
		buildPipe := func() *pipeline.Pipeline {
			p, err := pipeline.New().
				WithFrontend(fe).
				WithGenerator(gen).
				WithBackend(be).
				WithSink(sink.NewMemory()).
				WithManifestPath(manifestPath).
				Build()
			assertNoError(t, err)
			return p
		}

		renderTag = "first"
		assertNoError(t, buildPipe().Run(t.Context(), "a"))
		renderTag = "second"
		assertNoError(t, buildPipe().Run(t.Context(), "a"))

		m, err := manifest.Read(manifestPath)
		assertNoError(t, err)
		count := 0
		var hash string
		for _, o := range m.Outputs {
			if o.Target.ImportPath == "a" {
				count++
				hash = o.Hash
			}
		}
		if count != 1 {
			t.Errorf("same-scope re-run must replace, not duplicate; got %d entries for ImportPath 'a'", count)
		}
		if hash == "" {
			t.Errorf("expected a hash on the merged entry; got empty")
		}
	})
}

// test reads os.Args or asserts on BackendContext.Command, so
// parallel siblings stay safe across the mutation window.
//
//nolint:paralleltest // mutates os.Args; must run serial. No sibling
func TestPipeline_Run_LibraryCommandLine(t *testing.T) {
	t.Run("commandLine returns the library marker when os.Args carries no positional arguments", func(t *testing.T) {
		orig := os.Args
		t.Cleanup(func() { os.Args = orig })
		os.Args = []string{"some-binary"}

		var captured string
		be := &recBE{
			name: "be",
			lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				captured = ctx.Command
			},
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		if captured != "(library)" {
			t.Fatalf("BackendContext.Command = %q, want %q", captured, "(library)")
		}
	})
}

// TestRun_HonoursCancellation pins that a cancelled context stops
// the run at a phase boundary instead of executing every phase to
// completion.
//
// Run took a context and discarded it, so a caller wiring
// signal.NotifyContext or a per-request timeout got a pipeline that
// ran through the frontend load, every annotator and generator,
// layout, render and the manifest write regardless. The signature
// promised something the body did not do.
func TestRun_HonoursCancellation(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T) (*sink.Memory, error) {
		t.Helper()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		mem := sink.NewMemory()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(mem).
			Build()
		assertNoError(t, err)
		return mem, p.Run(ctx, "x")
	}

	t.Run("the error wraps the context's own cause", func(t *testing.T) {
		t.Parallel()
		// Callers classify with errors.Is; returning a bare
		// ErrRunHadErrors would make a cancellation indistinguishable
		// from a plugin failure.
		if _, err := run(t); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v, want it to wrap context.Canceled", err)
		}
	})

	t.Run("no phase executes", func(t *testing.T) {
		t.Parallel()
		// Returning the right error while still rendering every file
		// would satisfy the assertion above and miss the point.
		mem, _ := run(t)
		if got := mem.Len(); got != 0 {
			t.Fatalf("sink received %d writes on a cancelled run, want 0", got)
		}
	})
}

// TestRun_TwiceOnOnePipelineIsClean pins that per-run derived state
// does not leak between invocations of the same [pipeline.Pipeline].
//
// resolvedLayouts was allocated lazily and never cleared, so a
// second Run compared its routing against the first run's entries
// and reported ordinary cross-run variation as an Internal
// diagnostic — which counts toward HasErrors, so the second run
// failed. Most callers build a fresh Pipeline per run and never saw
// it; library embedders and long-lived processes did.
func TestRun_TwiceOnOnePipelineIsClean(t *testing.T) {
	t.Parallel()

	runTwice := func(t *testing.T) (*diag.Sink, error) {
		t.Helper()
		d := diag.New()
		p, err := pipeline.New().
			WithDiag(d).
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		return d, p.Run(t.Context(), "x")
	}

	t.Run("the second run succeeds", func(t *testing.T) {
		t.Parallel()
		if d, err := runTwice(t); err != nil {
			t.Fatalf("second Run failed: %v; diags=%+v", err, d.Diagnostics())
		}
	})

	t.Run("the second run reports no Internal diagnostic", func(t *testing.T) {
		t.Parallel()
		// Internal severity means "framework bug". Leaked per-run
		// state reported ordinary cross-run variation at that level,
		// which is both wrong and unactionable for the user.
		d, _ := runTwice(t)
		for _, g := range d.Diagnostics() {
			if g.Severity == diag.Internal {
				t.Errorf("second run reported an Internal diagnostic: %s", g.Message)
			}
		}
	})
}

// TestRun_ResetsResolvedLayoutsBetweenRuns pins the reset directly.
//
// The end-to-end assertion above cannot reach it: two runs over
// identical input resolve identical routing, so the divergence
// comparison agrees and nothing fires. Reproducing the failure
// end-to-end needs two runs that resolve the *same* emit.Target from
// *different* precedence layers, which no fixture in this package
// constructs. Asserting the reset itself is the honest substitute —
// it pins the mechanism the leak depended on, and says plainly that
// it is not an end-to-end reproduction.
func TestRun_ResetsResolvedLayoutsBetweenRuns(t *testing.T) {
	t.Parallel()

	seeded := func(t *testing.T) *pipeline.Pipeline {
		t.Helper()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		p.RecordResolvedLayoutForTest(
			emit.Target{Dir: "d", Filename: "f.go"},
			manifest.ResolvedLayout{Package: "one"},
		)
		return p
	}

	t.Run("a recorded layout is observable", func(t *testing.T) {
		t.Parallel()
		// Without this the clearing assertion below would pass
		// against a fixture that never recorded anything.
		if !seeded(t).HasLayoutActivityForTest() {
			t.Fatalf("fixture recorded no layout; the reset assertion would be vacuous")
		}
	})

	t.Run("a subsequent run clears it", func(t *testing.T) {
		t.Parallel()
		p := seeded(t)
		assertNoError(t, p.Run(t.Context(), "x"))
		if p.HasLayoutActivityForTest() {
			t.Fatalf("resolvedLayouts survived a Run; a later run inherits the previous run's routing")
		}
	})
}

// versionedGen is a generator whose declared version is settable, so
// a test can observe the cache key move when it changes.
type versionedGen struct {
	name    string
	version string
}

// Name returns the configured plugin identifier.
func (g *versionedGen) Name() string { return g.name }

// Version reports the configured version via [plugin.Versioned].
func (g *versionedGen) Version() string { return g.version }

// Generate emits nothing; the cache key is what this fixture is for.
func (*versionedGen) Generate(*plugin.GeneratorContext) error { return nil }

// TestRun_PluginVersionEntersCacheKey pins that a plugin's declared
// version composes into its cache key.
//
// plugin.Versioned, its sdk alias, the cache package doc and the
// README all state that a version bump invalidates that plugin's
// cached entries and no other's. Nothing read the value, so a bump
// produced an identical key — the documented invalidation never
// happened.
func TestRun_PluginVersionEntersCacheKey(t *testing.T) {
	t.Parallel()

	keysFor := func(t *testing.T, version string) []string {
		t.Helper()
		c := &keyRecordingCache{}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&versionedGen{name: "vg", version: version}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithCache(c).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		return c.keys
	}

	t.Run("the run records at least one key", func(t *testing.T) {
		t.Parallel()
		// Guards every comparison below against passing vacuously on
		// an empty slice.
		if got := keysFor(t, "v1"); len(got) == 0 {
			t.Fatalf("no cache entries recorded")
		}
	})

	t.Run("a version bump changes the key", func(t *testing.T) {
		t.Parallel()
		if before, after := keysFor(t, "v1"), keysFor(t, "v2"); slices.Equal(before, after) {
			t.Fatalf("cache keys unchanged across a version bump: %v", before)
		}
	})

	t.Run("an unchanged version keeps the key stable", func(t *testing.T) {
		t.Parallel()
		// A key that moved on every run would also satisfy the bump
		// assertion while making the cache useless.
		if first, second := keysFor(t, "v1"), keysFor(t, "v1"); !slices.Equal(first, second) {
			t.Fatalf("cache keys differ across identical runs: %v vs %v", first, second)
		}
	})
}

// keyRecordingCache is a [cache.Cache] that records every key
// written, so a test can compare the composed key across runs
// without reaching into the pipeline.
type keyRecordingCache struct {
	keys []string
}

// Get always misses; the pipeline's marker is write-only and this
// fixture only observes what it writes.
func (*keyRecordingCache) Get(string) ([]byte, bool) { return nil, false }

// Put records the key and discards the body.
func (c *keyRecordingCache) Put(key string, _ []byte) error {
	c.keys = append(c.keys, key)
	return nil
}
