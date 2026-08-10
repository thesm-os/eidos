// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golangtest_test

import (
	"slices"
	"sync"
	"testing"

	"go.thesmos.sh/eidos/eidostest/golangtest"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// recordingGenerator appends its name to a shared log when the
// pipeline runs it, which is the only externally visible proof that
// [golangtest.Driver] registered it at all — a generator dropped on
// the floor produces exactly the same rendered output as one that
// ran and emitted nothing.
type recordingGenerator struct {
	name string
	log  *generatorLog
}

func (g *recordingGenerator) Name() string { return g.name }

func (g *recordingGenerator) Generate(*plugin.GeneratorContext) error {
	g.log.record(g.name)
	return nil
}

// generatorLog collects generator names under a mutex. The pipeline
// may run the generate phase in parallel, so an unguarded slice here
// would make this file's own failures depend on the scheduler.
type generatorLog struct {
	mu    sync.Mutex
	names []string
}

func (l *generatorLog) record(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.names = append(l.names, name)
}

func (l *generatorLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.names)
}

// renderPkg is the fixture package every driver test loads. Its path
// is what [golangtest.Generated] derives the throwaway module path
// from, so it is spelled like a real one.
func renderPkg() *node.Package {
	return &node.Package{Name: "storepkg", Path: "example.com/storepkg"}
}

// renderBackend writes one file, which is enough to tell a run that
// reached the backend from one that stopped at Build.
func renderBackend() *writingBackend {
	return &writingBackend{writes: map[emit.Target][]byte{
		{Filename: "printer_stub.gen.go", ImportPath: "example.com/storepkg"}: []byte(goodDouble),
	}}
}

func TestRender(t *testing.T) {
	t.Parallel()

	t.Run("adopts what the backend wrote", func(t *testing.T) {
		t.Parallel()
		golangtest.Render(t, renderBackend(), renderPkg()).
			AssertPaths(t, "printer_stub.gen.go")
	})

	t.Run("runs the pipeline rather than only building it", func(t *testing.T) {
		t.Parallel()
		log := &generatorLog{}
		golangtest.Render(t, renderBackend(), renderPkg(),
			&recordingGenerator{name: "first", log: log})
		if got := log.snapshot(); !slices.Equal(got, []string{"first"}) {
			t.Fatalf("Render generator log = %v, want the generator to have run once", got)
		}
	})

	t.Run("registers every generator it was given", func(t *testing.T) {
		t.Parallel()
		log := &generatorLog{}
		golangtest.Render(t, renderBackend(), renderPkg(),
			&recordingGenerator{name: "first", log: log},
			&recordingGenerator{name: "second", log: log})
		got := log.snapshot()
		slices.Sort(got)
		if !slices.Equal(got, []string{"first", "second"}) {
			t.Fatalf("Render generator log = %v, want both generators to have run", got)
		}
	})

	t.Run("carries the run's import path into the module path", func(t *testing.T) {
		t.Parallel()
		golangtest.Render(t, renderBackend(), renderPkg()).
			Primary(t).
			AssertPackage(t, "storepkg")
	})
}

func TestDriver(t *testing.T) {
	t.Parallel()

	t.Run("stops before Build so an option can still be applied", func(t *testing.T) {
		t.Parallel()
		p := golangtest.Driver(t, renderBackend(), renderPkg()).
			WithBrand("driven").
			Build().
			Run("./...")
		if got := len(p.Sink().Files()); got != 1 {
			t.Fatalf("Driver run wrote %d file(s), want 1", got)
		}
	})

	t.Run("registers the frontend so the fixture package reaches the store", func(t *testing.T) {
		t.Parallel()
		log := &generatorLog{}
		golangtest.Driver(t, renderBackend(), renderPkg(),
			&recordingGenerator{name: "only", log: log}).
			Build().
			Run("./...")
		if got := log.snapshot(); !slices.Equal(got, []string{"only"}) {
			t.Fatalf("Driver generator log = %v, want the generator to have run once", got)
		}
	})
}
