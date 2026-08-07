// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golangtest

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// allPackages is the pattern every toolchain invocation runs over: a
// generator routing a companion into a subpackage produces more than
// one, and building only the root would skip exactly the file whose
// cross-package reference is worth checking.
const allPackages = "./..."

// AssertCompiles fails when the generated files do not build.
//
// The assertion every other one in this package is a proxy for. A
// substring check passes against a template that renders an unused
// local, a redeclared name, a call at the wrong arity, or a field the
// template still references — all of which surface only here.
//
// `go build` does not look at `_test.go` files;
// [Generated.AssertVets] and [Generated.AssertTestsPass] are what
// cover a generated companion.
//
// # Cost
//
// Shells out to the toolchain: seconds, not milliseconds. Call it
// once per fixture rather than once per subtest, and let the [Source]
// assertions carry the fine-grained work. The built module is cached
// on the receiver, so several toolchain assertions over one
// [Generated] pay the setup once.
func (g *Generated) AssertCompiles(tb testing.TB) *Generated {
	tb.Helper()
	g.run(tb, "build", []string{"build", allPackages})
	return g
}

// AssertVets fails when the generated files do not pass `go vet`.
//
// Broader than a build in two ways that matter: vet compiles
// `_test.go` files, which `go build` skips, and it catches the
// mistakes a generator makes most — a Printf verb that disagrees with
// its argument, a lock copied by value, an unreachable return.
func (g *Generated) AssertVets(tb testing.TB) *Generated {
	tb.Helper()
	g.run(tb, "vet", []string{"vet", allPackages})
	return g
}

// AssertTestsPass compiles and runs the generated test files.
//
// The loop nothing else closes. A generator whose output is a test
// suite has that suite as its real contract, and asserting on the
// text of a check — that a `t.Run` of some name exists — passes just
// as well when the check is empty, wrong, or asserts nothing at all.
//
// Runs with the race detector off and caching disabled: a generated
// suite is rewritten between runs, and a cached pass would report the
// previous generation's result.
func (g *Generated) AssertTestsPass(tb testing.TB) *Generated {
	tb.Helper()
	if !g.hasTests() {
		tb.Errorf("golangtest: the run produced no _test.go file, so there is no "+
			"generated suite to run (have %v)", g.paths())
		return g
	}
	g.run(tb, "test", []string{"test", "-count=1", allPackages})
	return g
}

// AssertSatisfies fails when the generated type does not implement
// the named interface.
//
// Compiles `var _ Iface = (*Type)(nil)` beside the output, which is
// the only assertion that catches the failures a double has that
// still compile: a dropped variadic marker declares `Print(args
// string)` where the interface wants `Print(args ...string)`, a
// method promoted through an embed can go missing entirely, and a
// method landing on the wrong receiver form leaves the value type
// short. Every one of those type-checks in isolation and satisfies
// nothing.
//
// Both names are resolved in the package the primary output declares,
// so an interface from elsewhere is spelled qualified — and the file
// declaring it has to be reachable, which is what
// [Generated.WithSource] is for.
func (g *Generated) AssertSatisfies(tb testing.TB, typeName, iface string) *Generated {
	tb.Helper()
	pkg, dir := g.primaryPackage(tb)
	if pkg == "" {
		return g
	}
	assertion := File{
		Path: filepath.ToSlash(filepath.Join(dir, "golangtest_satisfies.go")),
		Src: fmt.Appendf(
			nil,
			"package %s\n\n// Written by golangtest to prove %s implements %s.\nvar _ %s = (*%s)(nil)\n",
			pkg,
			typeName,
			iface,
			iface,
			typeName,
		),
	}
	g.runWith(tb, "satisfies", []string{"build", allPackages}, assertion)
	return g
}

// hasTests reports whether the run produced anything `go test` would
// execute.
func (g *Generated) hasTests() bool {
	for _, f := range g.files {
		if f.IsTest() {
			return true
		}
	}
	return false
}

// primaryPackage returns the package clause and directory of the
// first non-test generated file, which is where a satisfaction
// assertion has to land to reach the type it names.
func (g *Generated) primaryPackage(tb testing.TB) (pkg, dir string) {
	tb.Helper()
	for _, f := range g.files {
		if f.IsTest() {
			continue
		}
		return Parse(tb, f).file.Name.Name, f.Dir()
	}
	tb.Errorf("golangtest: the run produced no non-test file to assert satisfaction against "+
		"(have %v)", g.paths())
	return "", ""
}

// run builds the module and runs one go subcommand in it.
func (g *Generated) run(tb testing.TB, label string, args []string) {
	tb.Helper()
	g.runWith(tb, label, args)
}

// runWith is [Generated.run] with extra files written over the module.
//
// The extra files bypass the cache deliberately: they vary per
// assertion, and reusing a directory that holds a previous
// assertion's file would build something the caller did not ask for.
func (g *Generated) runWith(tb testing.TB, label string, args []string, extra ...File) {
	tb.Helper()
	dir := g.moduleDir(tb, len(extra) == 0)
	for _, f := range extra {
		writeFile(tb, dir, f)
	}

	cmd := exec.CommandContext(tb.Context(), "go", args...)
	cmd.Dir = dir
	// GOFLAGS and GOWORK are inherited from the process running the
	// test, where a workspace is usually active; a temp module outside
	// it would be rejected outright. GOPROXY is off because every
	// dependency has to be present already — a fixture that reached
	// the network would pass or fail on connectivity.
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out

	if err := cmd.Run(); err != nil {
		tb.Errorf("golangtest: go %s failed on the generated output: %v\n"+
			"--- go %s ---\n%s%s", label, err, label, out.String(), g.listing())
	}
}

// moduleDir returns the directory holding the assembled module,
// reusing a previous one when the caller allows it.
func (g *Generated) moduleDir(tb testing.TB, cacheable bool) string {
	tb.Helper()
	if cacheable && g.built != "" {
		return g.built
	}
	dir := tb.TempDir()
	if g.baseModule != "" {
		copyTree(tb, g.baseModule, dir)
	} else {
		writeFile(tb, dir, File{
			Path: "go.mod",
			Src: fmt.Appendf(nil, "module %s\n\ngo %s\n",
				g.modulePathOf(), g.goVersionOf()),
		})
	}
	for _, f := range append(cloned(g.support), g.files...) {
		writeFile(tb, dir, f)
	}
	if cacheable {
		g.built = dir
	}
	return dir
}

// cloned copies a file list, so appending to it cannot disturb the
// receiver's own.
func cloned(in []File) []File {
	out := make([]File, len(in))
	copy(out, in)
	return out
}

// listing renders every generated file with line numbers.
//
// Attached to every toolchain failure because the compiler's message
// names a position in code the author never wrote, in a temp
// directory that no longer exists by the time they read it. Without
// this the assertion is worse than the substring check it replaces.
func (g *Generated) listing() string {
	var b strings.Builder
	for _, f := range g.files {
		fmt.Fprintf(&b, "\n--- %s ---\n%s", f.Path, numbered(f.Src))
	}
	return b.String()
}

// writeFile writes one file under root, creating its directory.
func writeFile(tb testing.TB, root string, f File) {
	tb.Helper()
	full := filepath.Join(root, filepath.FromSlash(f.Path))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		tb.Fatalf("golangtest: create %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, f.Src, 0o600); err != nil {
		tb.Fatalf("golangtest: write %s: %v", full, err)
	}
}

// copyTree copies a module directory into the throwaway root.
//
// [os.CopyFS] rather than a hand-rolled walk: it resolves every path
// inside the source root, so a symlink in a fixture module cannot
// reach outside it, and it is the operation this needs rather than an
// assembly of four that happen to compose into it.
func copyTree(tb testing.TB, src, dst string) {
	tb.Helper()
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		tb.Fatalf("golangtest: copy module %s: %v", src, err)
	}
}
