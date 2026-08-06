// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/writer"
)

// BenchmarkFinaliseBody measures one finalisation of a rendered
// file body, split so the two stages of the chain are separable.
//
// This is the benchmark that makes BenchmarkBackend_Render
// actionable. Render's per-target cost is template execution plus
// finalisation, and the open question — whether the sequential
// per-target loop is worth parallelising — turns on which of the
// two dominates. Template execution scales with the emit graph;
// finalisation re-parses the rendered text twice ([go/format.Source]
// and the goimports pass, which itself re-parses the output a third
// time in [importPaths]) and is pure CPU with no shared state, so if
// it dominates then the loop is embarrassingly parallel and worth
// splitting.
//
// Three sub-benchmarks, one operation each:
//
//   - "format.Source" times [runGoFormat] on the raw body.
//   - "goimports" times [runGoImports] on already-formatted bytes,
//     because that is what it receives in the chain — feeding it
//     unformatted input would measure a reformat the chain never
//     pays for.
//   - "full chain" times [finaliseBody], the composition the
//     backend actually calls.
//
// Deliberately outside the timed region: body generation, the
// tracked-import list, the pre-format for the goimports stage, and
// the diagnostic sink.
//
// The fixture is chosen so no diagnostic fires: the body is
// gofmt-parseable and every import it declares is tracked. That is
// not cosmetic — a warning per iteration would append to the sink
// forever, and the reported allocations would measure the growing
// diagnostic slice rather than the format pass. The post-loop
// assertion enforces it.
func BenchmarkFinaliseBody(b *testing.B) {
	b.ReportAllocs()

	body, tracked, target := benchFinaliseFixture(16)
	formatted, _ := runGoFormat(body, target, diag.New().For(Name))

	b.Run("format.Source", func(b *testing.B) {
		b.ReportAllocs()
		d := diag.New()
		ps := d.For(Name)
		var out []byte
		for b.Loop() {
			got, ok := runGoFormat(body, target, ps)
			if !ok {
				b.Fatalf("runGoFormat rejected the fixture body")
			}
			out = got
		}
		assertBenchClean(b, d, out)
	})

	b.Run("goimports", func(b *testing.B) {
		b.ReportAllocs()
		d := diag.New()
		ps := d.For(Name)
		paths := trackedPaths(tracked)
		var out []byte
		for b.Loop() {
			out = runGoImports(formatted, target, ps, paths)
		}
		assertBenchClean(b, d, out)
	})

	b.Run("full chain", func(b *testing.B) {
		b.ReportAllocs()
		d := diag.New()
		ps := d.For(Name)
		var out []byte
		for b.Loop() {
			out = finaliseBody(body, target, ps, tracked)
		}
		assertBenchClean(b, d, out)
	})
}

// BenchmarkFinaliseBody_Decls measures one full finalisation per
// operation over files of increasing declaration count.
//
// Both stages parse the whole file, and the goimports pass walks
// every identifier looking for unresolved references, so the cost
// could plausibly grow faster than the byte count. The scaling axis
// is what distinguishes "finalisation is a fixed per-file tax" from
// "finalisation punishes large generated files" — and the second
// answer would argue for splitting big targets rather than for
// parallelising the loop.
//
// Fixture construction is hoisted per size; only the finalisation
// is timed.
func BenchmarkFinaliseBody_Decls(b *testing.B) {
	b.ReportAllocs()

	for _, decls := range []int{1, 10, 100, 1000} {
		b.Run(strconv.Itoa(decls), func(b *testing.B) {
			b.ReportAllocs()
			body, tracked, target := benchFinaliseFixture(decls)
			d := diag.New()
			ps := d.For(Name)
			var out []byte
			for b.Loop() {
				out = finaliseBody(body, target, ps, tracked)
			}
			assertBenchClean(b, d, out)
		})
	}
}

// benchFinaliseFixture builds a rendered-body fixture holding decls
// struct declarations plus a method apiece, along with the tracked
// import list and routing target the finalisation chain expects.
//
// The shape mirrors what the backend's templates emit for a struct
// with builtin and external fields: a package clause, an unsorted
// import block that goimports will regroup, and one decl per
// entity. Only stdlib imports appear — a third-party path would
// send goimports to the module cache and make the measurement a
// filesystem benchmark.
//
// The body is deliberately not gofmt-clean (single-space indents,
// unsorted imports) so [runGoFormat] has real work to do; it is
// still parseable, which is what keeps the chain warning-free.
func benchFinaliseFixture(decls int) ([]byte, []writer.Import, emit.Target) {
	var buf strings.Builder
	buf.WriteString("package bench\n\nimport (\n \"fmt\"\n \"context\"\n \"strings\"\n)\n\n")
	for i := range decls {
		fmt.Fprintf(&buf, `
type Entity%[1]d struct {
 ID string
 Count int
 Tags []string
 Ctx context.Context
}

func (e *Entity%[1]d) String() string {
 return fmt.Sprintf("Entity%[1]d(%%s)", strings.ToUpper(e.ID))
}
`, i)
	}
	tracked := []writer.Import{
		{Path: "context", Alias: "context"},
		{Path: "fmt", Alias: "fmt"},
		{Path: "strings", Alias: "strings"},
	}
	target := emit.Target{
		Dir:        "bench",
		Filename:   "entities.go",
		Package:    "bench",
		ImportPath: "example.com/bench",
	}
	return []byte(buf.String()), tracked, target
}

// assertBenchClean fails the benchmark when the finalisation chain
// produced output or diagnostics the fixture was built to avoid.
//
// Empty output means the timed call degraded to its fallback path
// and the number measured an error path rather than the format
// pass. A non-empty sink means a warning fired on every iteration,
// which both distorts the allocation report and invalidates the
// fixture's premise that every import is tracked.
func assertBenchClean(b *testing.B, d *diag.Sink, out []byte) {
	b.Helper()
	if len(out) == 0 {
		b.Fatalf("finalisation produced no bytes")
	}
	if got := d.Diagnostics(); len(got) != 0 {
		b.Fatalf("fixture should finalise without diagnostics; got %+v", got)
	}
}

// TestRunGoFormat_InvalidBodyIsAnError pins the severity of a
// formatter failure.
//
// Reported as a Warn, a run wrote unparseable Go and exited
// successfully: the pipeline counts only Errors toward failure, so
// the defect surfaced later at `go build`, inside generated code,
// with nothing pointing back at the generator. A file the formatter
// cannot parse is never a file the compiler can, so the run must
// fail — while still emitting the bytes, since reading the broken
// output is how a template bug gets diagnosed.
func TestRunGoFormat_InvalidBodyIsAnError(t *testing.T) {
	t.Parallel()

	const invalid = "package p\n\ntype T struct {\n\tField if-absent.Value\n}\n"
	target := emit.Target{Dir: "x", Filename: "broken.go", Package: "p"}

	run := func(t *testing.T) (*diag.Sink, []byte, bool) {
		t.Helper()
		d := diag.New()
		out, ok := runGoFormat([]byte(invalid), target, d.For(Name))
		return d, out, ok
	}

	t.Run("the failure is reported at Error severity", func(t *testing.T) {
		t.Parallel()
		d, _, _ := run(t)
		if !d.HasErrors() {
			t.Fatalf("unparseable output did not fail the run; diags=%+v", d.Diagnostics())
		}
	})

	t.Run("the diagnostic names the offending file", func(t *testing.T) {
		t.Parallel()
		// The whole point is attribution: a build error in generated
		// code is useless without knowing which target produced it.
		d, _, _ := run(t)
		for _, g := range d.Diagnostics() {
			if strings.Contains(g.Message, target.JoinPath()) {
				return
			}
		}
		t.Fatalf("no diagnostic named %q; got %+v", target.JoinPath(), d.Diagnostics())
	})

	t.Run("the unformatted bytes are still returned", func(t *testing.T) {
		t.Parallel()
		// Failing the run must not also withhold the evidence.
		_, out, ok := run(t)
		if ok {
			t.Fatalf("runGoFormat reported success on unparseable input")
		}
		if string(out) != invalid {
			t.Fatalf("fallback bytes were altered")
		}
	})

	t.Run("valid input stays clean", func(t *testing.T) {
		t.Parallel()
		// Guards against the Error being raised unconditionally,
		// which would satisfy every assertion above.
		d := diag.New()
		if _, ok := runGoFormat([]byte("package p\n"), target, d.For(Name)); !ok || d.HasErrors() {
			t.Fatalf("valid input reported ok=%v errors=%v", ok, d.Diagnostics())
		}
	})
}
