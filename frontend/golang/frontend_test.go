// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/frontend/golang"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

func TestFrontendName(t *testing.T) {
	t.Parallel()

	t.Run("matches the canonical plugin name", func(t *testing.T) {
		t.Parallel()
		if golang.FrontendName != "golang" {
			t.Fatalf("FrontendName = %q, want %q", golang.FrontendName, "golang")
		}
	})
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("returns a non-nil Frontend", func(t *testing.T) {
		t.Parallel()
		if golang.New() == nil {
			t.Fatalf("New returned nil")
		}
	})
}

func TestFrontend_Name(t *testing.T) {
	t.Parallel()

	t.Run("returns FrontendName", func(t *testing.T) {
		t.Parallel()
		if got := golang.New().Name(); got != golang.FrontendName {
			t.Fatalf("Name = %q, want %q", got, golang.FrontendName)
		}
	})
}

func TestFrontend_Version(t *testing.T) {
	t.Parallel()

	t.Run("returns FrontendVersion", func(t *testing.T) {
		t.Parallel()
		if got := golang.New().Version(); got != golang.FrontendVersion {
			t.Fatalf("Version = %q, want %q", got, golang.FrontendVersion)
		}
	})
}

func TestFrontend_EmitVersions(t *testing.T) {
	t.Parallel()

	t.Run("contains the in-tree emit major", func(t *testing.T) {
		t.Parallel()
		majors := golang.New().EmitVersions()
		want := emit.Major()
		if !slices.Contains(majors, want) {
			t.Fatalf("EmitVersions = %v, expected to include %q", majors, want)
		}
	})

	t.Run("returns an independent copy", func(t *testing.T) {
		t.Parallel()
		fe := golang.New()
		first := fe.EmitVersions()
		if len(first) == 0 {
			t.Fatalf("EmitVersions must report at least one major")
		}
		first[0] = "tampered"
		second := fe.EmitVersions()
		if second[0] == "tampered" {
			t.Fatalf("EmitVersions returned an aliased slice; mutation leaked back into the frontend")
		}
	})
}

// TestConformance runs the framework's plugin-conformance suite
// against this package's plugin. The suite pins the standard
// framework contracts (stable Name, role-interface compliance,
// deterministic capability ordering, unique directive schema
// names, non-empty Versioned version) plus the per-role
// frontend contracts (empty-pattern panic recovery, determinism
// across two runs of the same source fixture).
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, golang.New())
	})

	t.Run("frontend contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunFrontendSuite(
			t,
			golang.New(),
			[]plugintest.FrontendFixture{
				{
					Name:    "basic_struct fixture",
					Pattern: "./...",
					Options: map[string]string{
						"dir": filepath.Join(goldenRoot, "basic_struct"),
					},
				},
				{
					Name:    "interface_with_methods fixture",
					Pattern: "./...",
					Options: map[string]string{
						"dir": filepath.Join(goldenRoot, "interface_with_methods"),
					},
				},
			},
		)
	})
}

// BenchmarkFrontend_Load measures one full frontend pass over an
// in-tree golden fixture: the `go list` fork and type-check inside
// [packages.Load], the AST → [node] conversion of every declaration,
// the `go.*` meta stamping, and the store write.
//
// The whole pass is the unit because it is what a pipeline pays per
// pattern, and no stage of it is separable in practice — the converter
// cannot run without the type information the load produces. The
// number this reports is the floor on any run of the framework: every
// pipeline pays it once per pattern before a single plugin executes.
//
// multi_file_package is the fixture with the most declarations spread
// over more than one file, so the per-file assembly the converter does
// is inside the measurement rather than being a single-file special
// case.
//
// Deliberately outside the timed region: fixture-path resolution,
// option decoding and the directive parser, all of which a real
// pipeline builds once per run. Deliberately inside it: the fresh
// [store.Store] and [diag.Sink] per iteration. The store cannot be
// reused — a second AddPackage for the same import path is rejected,
// so every iteration after the first would measure the error path
// instead of a conversion. Their construction is a rounding error next
// to the loader's subprocess.
//
// The pre-flight load asserts the fixture actually converts:
// [packages.Load] reports a pattern that matches nothing as an empty
// result, not an error, so a benchmark over a broken fixture path
// would report a fast, clean, meaningless number.
func BenchmarkFrontend_Load(b *testing.B) {
	b.ReportAllocs()

	fe := benchmarkFrontend(b, filepath.Join(goldenRoot, "multi_file_package"), false)
	parser := directive.DefaultParser()
	assertBenchmarkLoadConverts(b, fe, parser, 2)

	for b.Loop() {
		if err := fe.Load(&plugin.FrontendContext{
			Store:   store.New(),
			Diag:    diag.New(),
			Parser:  parser,
			Pattern: "./...",
		}); err != nil {
			b.Fatalf("Load: %v", err)
		}
	}
}

// BenchmarkFrontend_Load_Declarations scales the number of top-level
// declarations in a single loaded package.
//
// The converter walks every declaration and, for each one, stamps meta
// keys that ask go/types questions — [types.Implements] for the
// stringer check, [types.Comparable], the interface and context
// probes. Those are per-declaration answers derived from
// package-global type information, which is exactly the shape that
// turns quadratic without anyone noticing: the per-declaration cost
// grows with the package it lives in. Loading one fixture cannot
// reveal that; four sizes an order of magnitude apart can.
//
// The package under load is the basic_struct fixture's declaration
// shape repeated — a documented struct with one named field — so the
// only variable across sizes is how many of them there are. The
// module is written to a temp directory rather than testdata because
// checking a thousand generated declarations into the repository
// would be a fixture nobody can read; `ignore_workspace` keeps
// `go list` inside that module rather than resolving it against the
// enclosing go.work.
//
// Each size writes its module and runs one validating load before the
// timed region, so a size whose generated source failed to compile
// fails the benchmark instead of reporting a suspiciously flat curve.
//
// Read allocs/op, not ns/op, when watching this for a regression.
// Every size pays the same ~20ms `go list` fork, which swamps the
// per-declaration cost until the package is large; allocations carry
// no such constant and track the conversion directly, so a change that
// made the converter quadratic would show there one order of magnitude
// before it showed in the timings.
func BenchmarkFrontend_Load_Declarations(b *testing.B) {
	b.ReportAllocs()

	parser := directive.DefaultParser()
	for _, decls := range []int{1, 10, 100, 1000} {
		b.Run(strconv.Itoa(decls), func(b *testing.B) {
			b.ReportAllocs()

			dir := writeBenchmarkModule(b, benchmarkDeclSource(b, decls))
			fe := benchmarkFrontend(b, dir, true)
			assertBenchmarkLoadConverts(b, fe, parser, decls)

			for b.Loop() {
				if err := fe.Load(&plugin.FrontendContext{
					Store:   store.New(),
					Diag:    diag.New(),
					Parser:  parser,
					Pattern: "./...",
				}); err != nil {
					b.Fatalf("Load: %v", err)
				}
			}
		})
	}
}

// benchmarkFrontend returns a frontend pinned to dir. Options are
// decoded once here rather than per iteration because a pipeline
// decodes them once at Build time; leaving the decode in the loop
// would fold reflection cost into the load measurement.
func benchmarkFrontend(b *testing.B, dir string, ignoreWorkspace bool) *golang.Frontend {
	b.Helper()
	fe := golang.New()
	values := map[string]string{"dir": dir}
	if ignoreWorkspace {
		values["ignore_workspace"] = "true"
	}
	if err := fe.SetOptions(opt.New(fe.OptionsSchema(), values)); err != nil {
		b.Fatalf("SetOptions: %v", err)
	}
	return fe
}

// assertBenchmarkLoadConverts runs one untimed load and fails the
// benchmark unless it produced a package carrying wantStructs struct
// declarations and no diagnostics.
//
// This is the guard that separates "the load is fast" from "the load
// did nothing": an unresolvable directory, a fixture moved out from
// under the benchmark, or generated source that does not compile all
// surface as an empty store plus diagnostics rather than as an error
// from Load, and would otherwise be reported as a benchmark result.
func assertBenchmarkLoadConverts(b *testing.B, fe *golang.Frontend, parser *directive.Parser, wantStructs int) {
	b.Helper()
	s, d := store.New(), diag.New()
	if err := fe.Load(&plugin.FrontendContext{Store: s, Diag: d, Parser: parser, Pattern: "./..."}); err != nil {
		b.Fatalf("Load: %v", err)
	}
	if d.HasErrors() {
		b.Fatalf("fixture load reported diagnostics: %+v", d.Diagnostics())
	}
	pkg := firstPackageIn(s, "")
	if pkg == nil {
		b.Fatalf("fixture load produced no package")
	}
	if len(pkg.Structs) != wantStructs {
		b.Fatalf("fixture %q converted %d structs, want %d", pkg.Path, len(pkg.Structs), wantStructs)
	}
}

// benchmarkDeclSource builds a single-file package holding decls
// copies of the basic_struct fixture's declaration shape. Names are
// zero-padded so the source order the frontend sees does not depend
// on the number of digits in the index.
func benchmarkDeclSource(b *testing.B, decls int) string {
	b.Helper()
	var src strings.Builder
	src.WriteString("// Package fixture is generated by BenchmarkFrontend_Load_Declarations.\npackage fixture\n")
	for i := range decls {
		fmt.Fprintf(&src, "\n// User%04d is a basic struct with one named field.\ntype User%04d struct {\n"+
			"\t// Name is the user's display name.\n\tName string\n}\n", i, i)
	}
	return src.String()
}

// writeBenchmarkModule materialises source as a self-contained module
// in a fresh temp directory and returns its path. The module reuses
// the test suite's go.mod template so the generated package resolves
// the same way a fixture module does.
func writeBenchmarkModule(b *testing.B, source string) string {
	b.Helper()
	dir := b.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(defaultGoMod), 0o600); err != nil {
		b.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fixture.go"), []byte(source), 0o600); err != nil {
		b.Fatalf("write fixture.go: %v", err)
	}
	return dir
}

// ExampleFrontend_Load drives the frontend the way a pipeline does:
// point it at a directory, hand it a pattern, and read the converted
// declarations back out of the store it wrote into.
//
// The directory here is one of the repository's golden fixtures, which
// is the only part a caller would replace — everything else is the
// real sequence. Note that Load reports per-package failures as
// diagnostics on the supplied [diag.Sink] and reserves its error
// return for the case where nothing could be loaded at all, so a
// caller that only checks the error will miss a package that failed to
// type-check.
func ExampleFrontend_Load() {
	fe := golang.New()
	if err := fe.SetOptions(opt.New(fe.OptionsSchema(), map[string]string{
		"dir": filepath.Join(goldenRoot, "basic_struct"),
	})); err != nil {
		fmt.Println("SetOptions:", err)
		return
	}

	s, d := store.New(), diag.New()
	if err := fe.Load(&plugin.FrontendContext{
		Store:   s,
		Diag:    d,
		Parser:  directive.DefaultParser(),
		Pattern: "./...",
	}); err != nil {
		fmt.Println("Load:", err)
		return
	}

	pkg := firstPackageIn(s, "basic_struct")
	fmt.Println("errors:", d.HasErrors())
	fmt.Println("package:", pkg.Name)
	fmt.Println("field:", pkg.StructByName("User").FieldByName("Name").Type.Name)

	// Output:
	// errors: false
	// package: fixture
	// field: string
}

// BenchmarkFrontend_Load_NoCache measures the configuration
// `--no-cache` actually runs.
//
// Every other benchmark here builds its FrontendContext without a
// Cache field, so ctx.Cache is nil and the cache write path is
// skipped by the nil guard that has always been there. A real run
// never passes nil: Builder.Build substitutes cache.NewNone when none
// was supplied, and the CLI returns the same type for the explicit
// off switch. The path that hashes every source file and marshals the
// whole node graph before handing it to something that discards it
// was therefore worth exactly zero in every recorded number.
//
// Recorded per size so a reintroduced marshal shows up as a slope
// against the nil-cache benchmark above rather than as an absolute
// nobody has a reference for.
func BenchmarkFrontend_Load_NoCache(b *testing.B) {
	b.ReportAllocs()

	parser := directive.DefaultParser()
	for _, decls := range []int{1, 100, 1000} {
		b.Run(strconv.Itoa(decls), func(b *testing.B) {
			b.ReportAllocs()

			dir := writeBenchmarkModule(b, benchmarkDeclSource(b, decls))
			fe := benchmarkFrontend(b, dir, true)
			assertBenchmarkLoadConverts(b, fe, parser, decls)

			for b.Loop() {
				if err := fe.Load(&plugin.FrontendContext{
					Store:   store.New(),
					Diag:    diag.New(),
					Parser:  parser,
					Cache:   cache.NewNone(),
					Pattern: "./...",
				}); err != nil {
					b.Fatalf("Load: %v", err)
				}
			}
		})
	}
}
