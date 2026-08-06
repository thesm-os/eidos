// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipelinetest_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/emit"
)

// stubBackendWith returns a stubBackend pre-seeded with the supplied
// target→body map.
func stubBackendWith(writes map[emit.Target][]byte) *stubBackend {
	return &stubBackend{name: "stub-be", lang: "stub", writes: writes}
}

// buildPipelineWithOutputs builds a pipelinetest.Pipeline whose stub
// backend writes the supplied target→body pairs on Render. Bound to
// the supplied TB so assertion failures route to the caller's choice
// of testing handle.
func buildPipelineWithOutputs(tb testing.TB, writes map[emit.Target][]byte) *pipelinetest.Pipeline {
	tb.Helper()
	return pipelinetest.New(tb).
		WithFrontend(pipelinetest.FromNodes()).
		WithBackend(stubBackendWith(writes)).
		Build().
		Run()
}

func TestPipeline_Run(t *testing.T) {
	t.Parallel()

	t.Run("returns the pipeline for chaining", func(t *testing.T) {
		t.Parallel()
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(stubBackendWith(nil)).
			Build()
		if got := p.Run(); got != p {
			t.Fatalf("Run should return its receiver for chaining")
		}
	})

	t.Run("plugin error diagnostics do not fatal the test", func(t *testing.T) {
		t.Parallel()
		fe := &erroringFrontend{name: "broken-fe", err: errors.New("boom")}
		p := pipelinetest.New(t).
			WithFrontend(fe).
			WithBackend(stubBackendWith(nil)).
			Build().
			Run()
		if !p.Diagnostics().HasErrors() {
			t.Fatalf("expected error diagnostic for failing frontend")
		}
	})

	t.Run("supplies a default empty pattern when none given", func(t *testing.T) {
		t.Parallel()
		p := buildPipelineWithOutputs(t, map[emit.Target][]byte{})
		if p.Diagnostics().HasErrors() {
			t.Fatalf("clean run should not have errors")
		}
	})

	t.Run("accepts caller-supplied patterns", func(t *testing.T) {
		t.Parallel()
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(stubBackendWith(nil)).
			Build().
			Run("custom-pattern")
		if p.Diagnostics().HasErrors() {
			t.Fatalf("run with explicit patterns should be clean")
		}
	})

	t.Run("Fatalf when the underlying pipeline returns a non-diagnostic error", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		// WithSink(nil) leaves the pipeline without a destination
		// sink; pipeline.Pipeline.Run returns ErrNoSink, which is
		// not the diagnostic-aggregated ErrRunHadErrors and so
		// should Fatalf via the testpipe wrapper.
		p := pipelinetest.New(fake).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(stubBackendWith(nil)).
			WithSink(nil).
			Build()
		captureFatal(func() { p.Run() })
		if !fake.Failed() {
			t.Fatalf("expected Fatalf on non-diagnostic run error")
		}
	})
}

func TestPipeline_Diagnostics(t *testing.T) {
	t.Parallel()

	t.Run("returns the same diag.Sink across calls", func(t *testing.T) {
		t.Parallel()
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(stubBackendWith(nil)).
			Build()
		first := p.Diagnostics()
		second := p.Diagnostics()
		if first != second {
			t.Fatalf("Diagnostics must return the same sink across calls")
		}
	})
}

func TestPipeline_Sink(t *testing.T) {
	t.Parallel()

	t.Run("returns the same memory sink across calls", func(t *testing.T) {
		t.Parallel()
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes()).
			WithBackend(stubBackendWith(nil)).
			Build()
		first := p.Sink()
		second := p.Sink()
		if first != second {
			t.Fatalf("Sink must return the same sink across calls")
		}
	})
}

func TestPipeline_Manifest(t *testing.T) {
	t.Parallel()

	t.Run("a manifest records the emitting plugin under dry run", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "manifest.json")
		m := routedBuilder(t, &greeterGenerator{}).
			WithManifestPath(path).
			WithDryRun(true).
			Build().
			Run().
			Manifest()
		if m == nil {
			t.Fatalf("Manifest should be populated once WithManifestPath is set")
		}
		var attributed bool
		for _, o := range m.Outputs {
			for _, p := range o.Plugins {
				if p.Name == "greetergen" {
					attributed = true
				}
			}
		}
		if !attributed {
			t.Errorf("manifest should attribute an output to greetergen; got %+v", m.Outputs)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("dry run should leave %s unwritten; stat err = %v", path, err)
		}
	})

	t.Run("returns nil when no manifest path was configured", func(t *testing.T) {
		t.Parallel()
		if m := routedBuilder(t, &greeterGenerator{}).Build().Run().Manifest(); m != nil {
			t.Fatalf("Manifest should stay nil without WithManifestPath; got %+v", m)
		}
	})
}

func TestPipeline_AssertFile(t *testing.T) {
	t.Parallel()

	t.Run("returns a FileAssertion for the matching captured file", func(t *testing.T) {
		t.Parallel()
		tgt := emit.Target{Dir: "out", Filename: "a.go", Package: "out"}
		p := buildPipelineWithOutputs(t, map[emit.Target][]byte{tgt: []byte("hello")})
		got := p.AssertFile("a.go")
		if got == nil {
			t.Fatalf("AssertFile should return a FileAssertion")
		}
		if got.Target() != tgt {
			t.Fatalf("target mismatch: %+v", got.Target())
		}
	})

	t.Run("fatals when no file matches", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		p := buildPipelineWithOutputs(fake, map[emit.Target][]byte{
			{Dir: "out", Filename: "a.go"}: []byte("a"),
		})
		captureFatal(func() { p.AssertFile("missing.go") })
		if !fake.Failed() {
			t.Fatalf("expected fatal on missing file")
		}
		if !strings.Contains(strings.Join(fake.fatals, "\n"), "missing.go") {
			t.Fatalf("fatal should name the missing file; got %v", fake.fatals)
		}
	})

	t.Run("fatals when multiple files share the basename", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		p := buildPipelineWithOutputs(fake, map[emit.Target][]byte{
			{Dir: "a", Filename: "f.go"}: []byte("a"),
			{Dir: "b", Filename: "f.go"}: []byte("b"),
		})
		captureFatal(func() { p.AssertFile("f.go") })
		if !fake.Failed() {
			t.Fatalf("expected fatal on ambiguous basename")
		}
	})
}

func TestPipeline_AssertFileInDir(t *testing.T) {
	t.Parallel()

	t.Run("returns the file matching (dir, name) exactly", func(t *testing.T) {
		t.Parallel()
		p := buildPipelineWithOutputs(t, map[emit.Target][]byte{
			{Dir: "a", Filename: "f.go"}: []byte("from-a"),
			{Dir: "b", Filename: "f.go"}: []byte("from-b"),
		})
		got := p.AssertFileInDir("b", "f.go")
		if string(got.Bytes()) != "from-b" {
			t.Fatalf("got wrong file; bytes=%q", got.Bytes())
		}
	})

	t.Run("fatals when no file matches", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		p := buildPipelineWithOutputs(fake, map[emit.Target][]byte{
			{Dir: "a", Filename: "f.go"}: []byte("a"),
		})
		captureFatal(func() { p.AssertFileInDir("z", "f.go") })
		if !fake.Failed() {
			t.Fatalf("expected fatal on missing (dir, name)")
		}
	})
}

func TestPipeline_AssertNoFile(t *testing.T) {
	t.Parallel()

	t.Run("succeeds when no captured filename matches", func(t *testing.T) {
		t.Parallel()
		p := buildPipelineWithOutputs(t, map[emit.Target][]byte{
			{Dir: "out", Filename: "a.go"}: []byte("a"),
		})
		p.AssertNoFile("missing.go") // must not Errorf the surrounding t
	})

	t.Run("Errorf when a captured file matches", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		p := buildPipelineWithOutputs(fake, map[emit.Target][]byte{
			{Dir: "out", Filename: "present.go"}: []byte("x"),
		})
		p.AssertNoFile("present.go")
		if !fake.Failed() {
			t.Fatalf("expected Errorf on present file")
		}
	})

	t.Run("returns the pipeline for chaining", func(t *testing.T) {
		t.Parallel()
		p := buildPipelineWithOutputs(t, nil)
		if got := p.AssertNoFile("any.go"); got != p {
			t.Fatalf("AssertNoFile should return its receiver for chaining")
		}
	})
}

func TestPipeline_AssertFileCount(t *testing.T) {
	t.Parallel()

	t.Run("succeeds when the count matches", func(t *testing.T) {
		t.Parallel()
		p := buildPipelineWithOutputs(t, map[emit.Target][]byte{
			{Dir: "a", Filename: "x.go"}: []byte("x"),
			{Dir: "b", Filename: "y.go"}: []byte("y"),
		})
		p.AssertFileCount(2)
	})

	t.Run("Errorf when the count differs", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		p := buildPipelineWithOutputs(fake, map[emit.Target][]byte{
			{Dir: "a", Filename: "x.go"}: []byte("x"),
		})
		p.AssertFileCount(2)
		if !fake.Failed() {
			t.Fatalf("expected Errorf on count mismatch")
		}
	})

	t.Run("returns the pipeline for chaining", func(t *testing.T) {
		t.Parallel()
		p := buildPipelineWithOutputs(t, nil)
		if got := p.AssertFileCount(0); got != p {
			t.Fatalf("AssertFileCount should return its receiver for chaining")
		}
	})
}
