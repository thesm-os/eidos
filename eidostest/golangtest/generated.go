// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golangtest

import (
	"path"
	"runtime"
	"slices"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/lang/golang"
)

// File is one Go source file, generated or hand-written.
//
// Path is relative to the module root and carries the directory a
// multi-package generator routed into, because two files that landed
// in different packages have to stay in different directories for
// the toolchain to see them the way a consumer will.
type File struct {
	// Path is the file's location relative to the module root —
	// `store_stub.gen.go`, or `storetest/store_stub.gen.go`.
	Path string

	// Src is the file's content.
	Src []byte

	// ImportPath is the canonical import path of the package the
	// file lands in, when the run resolved one. Used to derive the
	// throwaway module's path so a cross-package reference in the
	// generated code resolves exactly as it will for a consumer.
	ImportPath string
}

// Dir returns the directory portion of the file's path, empty at the
// module root.
func (f File) Dir() string {
	if d := path.Dir(f.Path); d != "." {
		return d
	}
	return ""
}

// IsTest reports whether the file is a Go test file, which decides
// whether `go build` will look at it — it will not — and whether
// [Generated.AssertTestsPass] has anything to run.
func (f File) IsTest() bool { return strings.HasSuffix(f.Path, "_test.go") }

// Generated is the set of files one run produced, plus the context
// needed to build them.
//
// Not safe for concurrent use: it caches a built module directory
// across assertions, which is what keeps a fixture's toolchain cost
// to one setup rather than one per subtest.
type Generated struct {
	files      []File
	support    []File
	modulePath string
	goVersion  string
	baseModule string
	built      string
}

// Rendered adopts every file a pipeline run produced.
//
// The module path and each file's directory are taken from the
// targets the run resolved, so a generator routing its companion
// into a subpackage is built the way a consumer will build it rather
// than flattened into one directory where the cross-package
// reference it emits would resolve for the wrong reason.
func Rendered(tb testing.TB, p *pipelinetest.Pipeline) *Generated {
	tb.Helper()
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

// Of adopts files a caller assembled itself, for a test driving a
// projection directly rather than through a pipeline.
func Of(files ...File) *Generated {
	g := &Generated{files: slices.Clone(files)}
	slices.SortFunc(g.files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return g
}

// WithSource adds the hand-written package the generated output
// references.
//
// The load-bearing option for every toolchain assertion: a generated
// double names the interface it doubles, a generated companion names
// the type it checks, and neither compiles alone. Supplying that
// source as Go rather than as a fixture builder also documents the
// input in the form a consumer actually writes.
func (g *Generated) WithSource(files ...File) *Generated {
	g.support = append(g.support, files...)
	g.built = ""
	return g
}

// WithModulePath overrides the module path the throwaway module
// declares. Defaults to the path derived from the run's resolved
// import paths.
func (g *Generated) WithModulePath(p string) *Generated {
	g.modulePath, g.built = p, ""
	return g
}

// WithGoVersion pins the `go` directive of the throwaway module,
// which is the floor every consumer is held to.
//
// Worth setting deliberately: a template that starts emitting a
// builtin or a syntax form from a later release raises the minimum
// Go version of every project that runs the generator, and nothing
// else in a generator's tests would notice. Defaults to the version
// running the test, which notices nothing.
func (g *Generated) WithGoVersion(v string) *Generated {
	g.goVersion, g.built = v, ""
	return g
}

// InModule builds inside a copy of an existing module directory, so
// the generated code can reference third-party packages.
//
// The escape hatch for output that imports a runtime library — a
// generated test double calling into its framework, say. The
// directory's go.mod, go.sum and any files it holds are copied, then
// the generated and support files are written over the top. Prefer
// [Generated.WithSource] where the dependencies are stdlib-only: it
// needs no fixture module to maintain.
func (g *Generated) InModule(dir string) *Generated {
	g.baseModule, g.built = dir, ""
	return g
}

// Files returns every file the run produced, ordered by path.
func (g *Generated) Files() []File { return slices.Clone(g.files) }

// File returns the parsed file at the given path.
func (g *Generated) File(tb testing.TB, filePath string) *Source {
	tb.Helper()
	for _, f := range g.files {
		if f.Path == filePath {
			return Parse(tb, f)
		}
	}
	tb.Fatalf("golangtest: no file at %q; the run produced %v", filePath, g.paths())
	return nil
}

// Suffixed returns the parsed file whose path ends in suffix.
//
// The addressing a plugin author already has the vocabulary for:
// they declared the suffix, and Layout composed the rest of the name
// from a source basename they would otherwise have to reproduce.
func (g *Generated) Suffixed(tb testing.TB, suffix string) *Source {
	tb.Helper()
	var matches []File
	for _, f := range g.files {
		if strings.HasSuffix(f.Path, suffix) {
			matches = append(matches, f)
		}
	}
	switch len(matches) {
	case 1:
		return Parse(tb, matches[0])
	case 0:
		tb.Fatalf("golangtest: no file ending in %q; the run produced %v", suffix, g.paths())
	default:
		tb.Fatalf("golangtest: %d files end in %q (%v); address one by path",
			len(matches), suffix, pathsOf(matches))
	}
	return nil
}

// Primary returns the parsed non-test file, for the common case of a
// generator whose primary output is its only one that is not a test.
func (g *Generated) Primary(tb testing.TB) *Source {
	tb.Helper()
	var matches []File
	for _, f := range g.files {
		if !f.IsTest() {
			matches = append(matches, f)
		}
	}
	switch len(matches) {
	case 1:
		return Parse(tb, matches[0])
	case 0:
		tb.Fatalf("golangtest: the run produced no non-test file (have %v)", g.paths())
	default:
		tb.Fatalf("golangtest: %d non-test files (%v); address one by path or suffix",
			len(matches), pathsOf(matches))
	}
	return nil
}

// paths lists every produced file, for a failure message.
func (g *Generated) paths() []string { return pathsOf(g.files) }

// pathsOf projects a file list's paths.
func pathsOf(files []File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

// modulePathOf returns the module path the throwaway module declares.
//
// Derived by stripping a file's directory from the import path the
// run resolved for it: a companion routed into `storetest/` with
// import path `example.com/storepkg/storetest` belongs to the module
// `example.com/storepkg`, and declaring that is what makes the
// generated cross-package import resolve rather than resolve by
// accident from a flattened directory.
func (g *Generated) modulePathOf() string {
	if g.modulePath != "" {
		return g.modulePath
	}
	for _, f := range g.files {
		if f.ImportPath == "" {
			continue
		}
		dir := f.Dir()
		if dir == "" {
			return f.ImportPath
		}
		if trimmed, ok := strings.CutSuffix(f.ImportPath, "/"+dir); ok {
			return trimmed
		}
	}
	return defaultModulePath
}

// defaultModulePath names the throwaway module when nothing in the
// run resolved an import path. Not a registrable domain, so a
// generated file that reached the network for it would fail loudly
// rather than resolve something.
const defaultModulePath = "golangtest.invalid/generated"

// goVersionOf returns the `go` directive for the throwaway module,
// defaulting to the toolchain running the test.
func (g *Generated) goVersionOf() string {
	if g.goVersion != "" {
		return g.goVersion
	}
	return GoVersionFrom(runtime.Version())
}

// GoVersionFrom reduces a toolchain version to the form go.mod
// accepts, and is what [Generated.WithGoVersion] defaults to.
//
// Split out from its caller so the shapes that cannot be produced on
// demand — a devel build, a gccgo string — are testable rather than
// taken on trust. [runtime.Version] returns `go1.26.5` on a release
// toolchain and something else on every other kind.
func GoVersionFrom(raw string) string {
	parts := strings.SplitN(strings.TrimPrefix(raw, "go"), ".", 3)
	if len(parts) < 2 || !isNumeric(parts[0]) || !isNumeric(parts[1]) {
		return fallbackGoVersion
	}
	return parts[0] + "." + parts[1]
}

// fallbackGoVersion is used when the running toolchain reports a
// version go.mod would reject — a devel or gccgo build. Low enough
// that the toolchain picks its own floor rather than refusing.
const fallbackGoVersion = "1.24"

// isNumeric reports whether s is a non-empty run of ASCII digits.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Language re-exports [golang.Language] so a test driving this
// package does not import the lang adapter for one string.
const Language = golang.Language
