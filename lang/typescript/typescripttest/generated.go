// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/lang/typescript"
)

// File is one TypeScript source file, generated or hand-written.
//
// Path is relative to the project root and carries the directory a
// multi-module generator routed into, because two files that landed
// in different directories have to stay there for a relative
// specifier between them to resolve the way it will for a consumer.
type File struct {
	// Path is the file's location relative to the project root —
	// `user_stub.gen.ts`, or `stubs/user_stub.gen.ts`.
	Path string

	// Src is the file's content.
	Src []byte

	// ImportPath is the module specifier the run resolved for the
	// file, when it resolved one. Recorded so a cross-module
	// reference in the generated code can be checked against where
	// its target actually landed.
	ImportPath string
}

// TSFile is a [File] over a TypeScript source constant.
//
// Sugar, but the kind worth having: every fixture's support module is
// a raw string literal, and spelling the struct and the []byte
// conversion at each one buries the source — which is the part a
// reader has to check against the generated output — in punctuation.
//
// Its two parameters are exactly what [tsfixture.Builder.TSSource]
// returns, so the projected pair reaches [Generated.WithSource] with
// no adapter between them.
func TSFile(path, src string) File {
	return File{Path: path, Src: []byte(src)}
}

// Dir returns the directory portion of the file's path, empty at the
// project root.
func (f File) Dir() string {
	if d := path.Dir(f.Path); d != "." {
		return d
	}
	return ""
}

// IsDeclaration reports whether the file is a `.d.ts` declaration
// file, which carries types and emits no JavaScript.
func (f File) IsDeclaration() bool { return strings.HasSuffix(f.Path, ".d.ts") }

// IsTestFile reports whether a runner would execute the file, which
// decides whether [Generated.AssertTestsPass] has anything to run.
//
// Both conventions, because the ecosystem has two and a generator
// picks whichever its consumers' runner is configured for: `.test.ts`
// is Node's and Jest's default, `.spec.ts` is Angular's and Karma's.
// A harness recognising one would silently have nothing to run for
// half the generators it serves.
func (f File) IsTestFile() bool {
	base := strings.TrimSuffix(f.Path, ".ts")
	return strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".spec")
}

// Generated is the set of files one run produced, plus the context
// needed to type-check them.
//
// Configure it — [Generated.WithSource] and its siblings — before
// handing it to anything; from then on its assertions are safe to run
// from parallel subtests. That matters because the shape this
// package's own docs prescribe, one fixture spent across several
// subtests, is exactly the shape that races on the cached project
// directory, and a test author should not have to discover the rule
// "keep every toolchain assertion in one subtest" by reading `-race`
// output about a package they did not write.
type Generated struct {
	files   []File
	support []File
	options map[string]any
	deps    []dependency
	runner  []string

	// packageName is what [Generated.WithPackageName] set, empty for
	// the default.
	packageName string

	// baseProject is the directory [Generated.InProject] copies, empty
	// for a project this package assembles itself.
	baseProject string

	// mu guards the assembled-project cache only. The `tsc` invocation
	// itself runs outside it, so parallel subtests still overlap on
	// the seconds rather than serialising on them.
	mu       sync.Mutex
	built    string
	builtFor testing.TB
}

// Rendered adopts every file a pipeline run produced.
//
// Each file's directory is taken from the target the run resolved, so
// a generator routing its companion into a subdirectory is
// type-checked the way a consumer will load it rather than flattened
// into one directory where the relative specifier it emits would
// resolve for the wrong reason.
//
// A run that recorded an error stops the test here. [pipelinetest]
// deliberately swallows the "had errors" disposition so a test about
// diagnostics can inspect them, which means a plugin that panicked or
// bailed leaves an empty sink — and every assertion downstream of an
// empty sink passes, having looked at nothing. A test asserting on
// diagnostics holds the pipeline directly and never reaches this.
func Rendered(tb testing.TB, p *pipelinetest.Pipeline) *Generated {
	tb.Helper()
	assertRunSucceeded(tb, p)
	g := &Generated{}
	for target, body := range p.Sink().Files() {
		name := target.Filename
		if target.Dir != "" {
			name = path.Join(target.Dir, name)
		}
		g.files = append(g.files, File{
			Path:       name,
			Src:        slices.Clone(body),
			ImportPath: target.ImportPath,
		})
	}
	// Map iteration is unordered and every failure message lists the
	// files it looked at, so a stable order is what keeps two runs of
	// a failing test reporting the same thing.
	slices.SortFunc(g.files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return g
}

// assertRunSucceeded stops the test when the run that produced these
// files recorded an error diagnostic.
//
// Reported before anything is adopted, because the failure a caller
// would otherwise see is `no file at "x.gen.ts"` — which reads as a
// routing problem and sends them looking in the wrong plugin.
func assertRunSucceeded(tb testing.TB, p *pipelinetest.Pipeline) {
	tb.Helper()
	sink := p.Diagnostics()
	if sink == nil || !sink.HasErrors() {
		return
	}
	var b strings.Builder
	for _, d := range sink.Diagnostics() {
		if d.Severity < diag.Error {
			continue
		}
		fmt.Fprintf(&b, "\n  %s: %s: %s", d.Severity, d.Plugin, d.Message)
	}
	tb.Fatalf("typescripttest: the run recorded errors, so whatever it wrote is not what the "+
		"generator meant to write and every assertion over it would be vacuous:%s", b.String())
}

// Of adopts files a caller assembled itself, for a test driving a
// projection directly rather than through a pipeline.
func Of(files ...File) *Generated {
	g := &Generated{files: slices.Clone(files)}
	slices.SortFunc(g.files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return g
}

// WithSource adds the hand-written module the generated output
// imports from.
//
// The load-bearing option for [Generated.AssertTypeChecks]: a
// generated file that imports `./user` does not type-check in a
// project where nothing declares one, and the failure names the
// missing module rather than whatever the generator got wrong.
//
// Project it with [tsfixture.Builder.TSSource] rather than writing it
// out, so the support module and the graph that drove the run cannot
// disagree.
func (g *Generated) WithSource(files ...File) *Generated {
	g.support = append(g.support, files...)
	return g
}

// Files returns every file the run produced, ordered by path.
func (g *Generated) Files() []File { return slices.Clone(g.files) }

// AssertPaths fails when the run produced anything other than exactly
// the named paths.
//
// The name a consumer sees is not the name the plugin chose: Layout
// composes it from the origin's basename and the plugin's suffix, and
// a routing directive can move the whole thing. Pinning the result is
// what catches a suffix that stopped composing the way its author
// read it.
func (g *Generated) AssertPaths(tb testing.TB, want ...string) *Generated {
	tb.Helper()
	got := g.paths()
	slices.Sort(want)
	if !slices.Equal(got, want) {
		tb.Errorf("typescripttest: the run produced %v, want %v", got, want)
	}
	return g
}

// File returns the parsed file at the given path.
func (g *Generated) File(tb testing.TB, filePath string) *Source {
	tb.Helper()
	for _, f := range g.files {
		if f.Path == filePath {
			return Parse(tb, f)
		}
	}
	tb.Fatalf("typescripttest: no file at %q; the run produced %v", filePath, g.paths())
	return nil
}

// Suffixed returns the parsed file whose path ends in suffix.
//
// The addressing a plugin author already has the vocabulary for: a
// plugin declares its outputs by suffix, so a test asking for
// `.gen.ts` is asking in the terms the plugin was written in and
// stays correct when the origin it routed from is renamed.
func (g *Generated) Suffixed(tb testing.TB, suffix string) *Source {
	tb.Helper()
	var found []File
	for _, f := range g.files {
		if strings.HasSuffix(f.Path, suffix) {
			found = append(found, f)
		}
	}
	switch len(found) {
	case 0:
		tb.Fatalf("typescripttest: no file ends in %q; the run produced %v", suffix, g.paths())
	case 1:
		return Parse(tb, found[0])
	}
	tb.Fatalf("typescripttest: %d files end in %q (%v), so the assertion would be about "+
		"whichever the sink happened to order first", len(found), suffix, pathsOf(found))
	return nil
}

// Primary returns the parsed file, for the common case of a run that
// produced exactly one.
//
// Fails rather than guessing when there are several: a generator
// emitting a companion alongside its surface has two, and an
// assertion that silently picked one would be about whichever the
// sink ordered first.
func (g *Generated) Primary(tb testing.TB) *Source {
	tb.Helper()
	switch len(g.files) {
	case 0:
		tb.Fatalf("typescripttest: the run produced no files")
		return nil
	case 1:
		return Parse(tb, g.files[0])
	}
	tb.Fatalf("typescripttest: the run produced %d files (%v); name one with File or "+
		"Suffixed rather than letting the assertion pick", len(g.files), g.paths())
	return nil
}

// paths returns every produced file's path, sorted.
func (g *Generated) paths() []string { return pathsOf(g.files) }

// pathsOf reduces a file set to its sorted paths.
func pathsOf(files []File) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	slices.Sort(out)
	return out
}

// Language re-exports [typescript.Language] so a test driving this
// package need not import the language package beside it.
const Language = typescript.Language
