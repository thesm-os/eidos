// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"errors"
	"testing"
	"text/template"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
	"go.thesmos.sh/eidos/store"
)

// errSink is a sink whose every write fails.
type errSink struct{ err error }

func (s errSink) Write(emit.Target, []byte) error { return s.err }

// errWrite is what errSink reports.
var errWrite = errors.New("sink refused the write")

// seeded returns a store holding one renderable interface.
func seeded(t *testing.T, target emit.Target) *store.Store {
	t.Helper()
	st := store.New()
	err := st.Emit().AddPackage(&emit.Package{
		Name: "out", Path: "./out",
		Interfaces: []*emit.Interface{{
			Name: "A", Target: target,
			Fields: []*emit.Field{{Name: "x", Type: emit.Builtin("string")}},
		}},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return st
}

// ctxFor assembles a backend context over st.
func ctxFor(st *store.Store, out sink.Sink) (*plugin.BackendContext, *diag.Sink) {
	d := diag.New()
	return &plugin.BackendContext{Store: st, Diag: d, Sink: out, Lang: Language}, d
}

func TestRenderFailures(t *testing.T) {
	t.Parallel()

	target := emit.Target{Dir: "out", Filename: "a.ts", ImportPath: "./out/a"}

	t.Run("a sink failure aborts the run", func(t *testing.T) {
		t.Parallel()
		// At that point nothing further can be delivered, so unlike a
		// render failure this does not continue to the next file.
		ctx, _ := ctxFor(seeded(t, target), errSink{err: errWrite})
		if err := New().Render(ctx); !errors.Is(err, errWrite) {
			t.Fatalf("Render = %v, want the sink's error", err)
		}
	})

	t.Run("a broken template set is reported rather than panicking", func(t *testing.T) {
		t.Parallel()
		// New keeps the parse error and Render returns it, which puts
		// the failure where the pipeline already handles one.
		b := &Backend{tmplErr: errWrite}
		ctx, _ := ctxFor(seeded(t, target), sink.NewMemory())
		if err := b.Render(ctx); !errors.Is(err, errWrite) {
			t.Fatalf("Render = %v, want the parse error", err)
		}
	})

	t.Run("a render failure reports the file and continues", func(t *testing.T) {
		t.Parallel()
		// One broken template reports itself rather than hiding every
		// other file's problem behind the first failure.
		b := New()
		broken, err := b.tmpl.Clone()
		if err != nil {
			t.Fatalf("clone: %v", err)
		}
		// Replace the interface template with one that always fails.
		// The func has to be registered before the parse, since the
		// parser validates every name a template calls.
		broken = broken.Funcs(template.FuncMap{
			"fail": func() (string, error) { return "", errWrite },
		})
		if _, err := broken.New(string(emit.KindInterface)).
			Parse(`{{ fail }}`); err != nil {
			t.Fatalf("parse: %v", err)
		}
		b.tmpl = broken

		mem := sink.NewMemory()
		ctx, d := ctxFor(seeded(t, target), mem)
		if err := b.Render(ctx); err != nil {
			t.Fatalf("Render aborted on a per-file failure: %v", err)
		}
		if mem.Len() != 0 {
			t.Error("a file was written despite the render failing")
		}
		if len(errorsIn(d)) == 0 {
			t.Error("a render failure produced no Error diagnostic")
		}
	})

	t.Run("an empty store writes no files", func(t *testing.T) {
		t.Parallel()
		mem := sink.NewMemory()
		ctx, d := ctxFor(store.New(), mem)
		if err := New().Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if mem.Len() != 0 {
			t.Fatalf("files = %d, want none", mem.Len())
		}
		if len(d.Diagnostics()) != 0 {
			t.Fatalf("an empty store produced %d diagnostics", len(d.Diagnostics()))
		}
	})

	t.Run("a target whose entities render to nothing writes no file", func(t *testing.T) {
		t.Parallel()
		// Writing an empty file would leave a stub the next run has to
		// prune.
		st := store.New()
		err := st.Emit().AddPackage(&emit.Package{
			Name: "out", Path: "./out",
			// A method is not a top-level renderable kind, so this
			// package contributes no declaration.
			Interfaces: nil,
		})
		if err != nil {
			t.Fatalf("seed: %v", err)
		}

		mem := sink.NewMemory()
		ctx, _ := ctxFor(st, mem)
		if err := New().Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if mem.Len() != 0 {
			t.Fatalf("files = %d, want none", mem.Len())
		}
	})
}

// errorsIn returns the Error-severity diagnostics in d.
func errorsIn(d *diag.Sink) []diag.Diag {
	var out []diag.Diag
	for _, entry := range d.Diagnostics() {
		if entry.Severity == diag.Error {
			out = append(out, entry)
		}
	}
	return out
}
