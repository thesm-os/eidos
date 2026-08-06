// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backendtest_test

import (
	"testing"

	"go.thesmos.sh/eidos/eidostest/backendtest"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
)

// TestRun_PassesEmitGraphToBackend pins the happy path: the
// harness seeds the store with the caller's emit packages,
// invokes Backend.Render, and the backend's writes land on the
// returned Sink. The stub backend writes a per-target marker so
// the assertion can confirm the wiring without depending on a
// real template surface.
func TestRun_PassesEmitGraphToBackend(t *testing.T) {
	t.Parallel()

	t.Run("backend renders against pre-built emit packages with Targets resolved", func(t *testing.T) {
		t.Parallel()
		target := emit.Target{Dir: "out", Filename: "x.go", Package: "out"}
		stub := &writingBackend{lang: "stub"}
		result := backendtest.Run(t, backendtest.RunOptions{
			Backend: stub,
			EmitPackages: []*emit.Package{{
				Name: "out", Path: "out",
				Structs: []*emit.Struct{{
					Name: "X", Package: "out", Target: target,
				}},
			}},
		})
		if result.Diag.HasErrors() {
			t.Fatalf("unexpected error diagnostics: %+v", result.Diag.Diagnostics())
		}
		mem, ok := result.Sink.(*sink.Memory)
		if !ok {
			t.Fatalf("expected default *sink.Memory; got %T", result.Sink)
		}
		body, ok := mem.Get(target)
		if !ok {
			t.Fatalf("sink missing the routed target; got files=%v", mem.Files())
		}
		if string(body) != "rendered:out/x.go" {
			t.Fatalf("sink body = %q, want %q", body, "rendered:out/x.go")
		}
	})

	t.Run("supplies the configured language to the backend", func(t *testing.T) {
		t.Parallel()
		stub := &writingBackend{lang: "fixturelang"}
		_ = backendtest.Run(t, backendtest.RunOptions{
			Backend: stub,
			EmitPackages: []*emit.Package{{
				Name: "x", Path: "x",
				Structs: []*emit.Struct{{
					Name: "X", Package: "x",
					Target: emit.Target{Dir: "x", Filename: "x.go", Package: "x"},
				}},
			}},
		})
		if stub.observedLang != "fixturelang" {
			t.Fatalf("backend saw Lang=%q on BackendContext; want %q",
				stub.observedLang, "fixturelang")
		}
	})
}

// writingBackend is the fixture backend the tests use. It writes
// a per-target marker so assertions can verify the harness's
// wiring without depending on a real template surface.
type writingBackend struct {
	lang         string
	observedLang string
}

// Name returns the fixture identifier.
func (*writingBackend) Name() string { return "stub-backend" }

// Language returns the configured language.
func (b *writingBackend) Language() string { return b.lang }

// EmitVersions satisfies the emit-versioned contract every
// backend implements.
func (*writingBackend) EmitVersions() []string { return []string{emit.Major()} }

// Render walks the byTarget index, records the BackendContext's
// configured Lang, and writes a per-target marker through the
// supplied sink.
func (b *writingBackend) Render(ctx *plugin.BackendContext) error {
	b.observedLang = ctx.Lang
	for _, target := range ctx.Store.Emit().ByTarget().Keys() {
		if err := ctx.Sink.Write(target, []byte("rendered:"+target.Dir+"/"+target.Filename)); err != nil {
			return err
		}
	}
	return nil
}

// ExampleRun shows the shape a backend author's test assembles: a
// pre-built emit graph with every [emit.Target] already resolved,
// handed to [Run] alongside the backend under test, and the rendered
// bytes read back off the returned Sink.
//
// The pre-populated Target is the contract that trips people up. The
// harness skips the Layout phase entirely, so nothing derives a
// target for a decl that arrives without one and the decl silently
// renders nowhere. Tests that mean to exercise routing decisions
// belong at the pipeline level; this harness is for backend-internal
// contracts — template selection, import resolution, formatting,
// slot composition.
//
// [Run] takes the enclosing test's `*testing.T` for its fatal path,
// which an [Example] function is never given, so the body below is
// written as the helper a backend author calls from their own
// `func TestMyBackend(t *testing.T)` and is not invoked here. There is deliberately no `// Output:` block: without one Go
// compiles and type-checks the example without running it, and the
// compile check is what the package docblock's prose snippet cannot
// offer. Execution is covered by TestRun_PassesEmitGraphToBackend
// above, which drives the same call against the same fixture.
func ExampleRun() {
	assertRendersUserRepo := func(t *testing.T) {
		t.Helper()

		target := emit.Target{Dir: "users", Filename: "user_gen.go", Package: "users"}

		// A real test passes its own backend here.
		result := backendtest.Run(t, backendtest.RunOptions{
			Backend: &writingBackend{lang: "golang"},
			EmitPackages: []*emit.Package{{
				Name: "users",
				Path: "example.com/users",
				Structs: []*emit.Struct{{
					Name:    "UserRepo",
					Package: "users",
					Target:  target,
				}},
			}},
		})

		if result.Diag.HasErrors() {
			t.Fatalf("render diagnostics: %+v", result.Diag.Diagnostics())
		}
		mem, ok := result.Sink.(*sink.Memory)
		if !ok {
			t.Fatalf("expected the default in-memory sink; got %T", result.Sink)
		}
		body, ok := mem.Get(target)
		if !ok {
			t.Fatalf("backend wrote nothing at %s; sink holds %v", target.JoinPath(), mem.Files())
		}
		t.Logf("%s:\n%s", target.JoinPath(), body)
	}

	// An Example has no *testing.T to hand over; see the docblock.
	_ = assertRendersUserRepo
}
