// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golangtest

import (
	"bytes"
	"fmt"
	"go/parser"
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

// goBuild names the subcommand every assertion whose question is
// "does this type-check" runs. The answer each of them wants is the
// compiler's exit status rather than anything it produces, so none of
// them needs an install or a link step.
const goBuild = "build"

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
	g.run(tb, "build", []string{goBuild, allPackages})
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
	return g.AssertSatisfiesAll(tb, Satisfaction{Type: typeName, Interface: iface})
}

// Satisfaction is one "this type implements that interface" claim.
type Satisfaction struct {
	// Type is the generated type, named as the primary output's
	// package spells it. Checked in its pointer form, which is the
	// form a generated double is handed round as.
	Type string

	// Interface is the interface it must satisfy, spelled as the
	// primary output's package sees it — qualified for one from
	// elsewhere, whose declaring file has to be reachable.
	Interface string
}

// AssertSatisfiesAll proves several satisfaction claims in one build.
//
// A generator emitting one double per interface makes N of these
// claims per run, and [Generated.AssertSatisfies] pays a fresh module
// directory for each — the extra file bypasses the cache, so N claims
// cost N builds for what is one file of N lines. At a second or three
// apiece that is the difference between a suite that keeps the
// assertion and one that drops it.
func (g *Generated) AssertSatisfiesAll(tb testing.TB, claims ...Satisfaction) *Generated {
	tb.Helper()
	if len(claims) == 0 {
		tb.Errorf("golangtest: AssertSatisfiesAll was given no claims, so it proves nothing")
		return g
	}
	if !checkClaims(tb, claims) {
		return g
	}
	pkg, dir := g.primaryPackage(tb)
	if pkg == "" {
		return g
	}
	g.runWith(tb, "satisfies", []string{goBuild, allPackages}, satisfactionFile(pkg, dir, claims))
	return g
}

// AssertDoesNotSatisfy fails when the generated type *does* implement
// the named interface.
//
// The direction that carries the weight for a detector or a
// heuristic. [Generated.AssertSatisfies] proves one type passes;
// what a shape detector actually claims is that every near miss does
// not, and a near miss is by construction something that looks right.
// A frontend records a variadic parameter by its element type, so
// `Write(p ...[]byte)` reaches a projection spelled exactly like
// `Write(p []byte)` — and a detector that accepted it wrote an
// io.Writer conformance for a type no consumer can pass to one. No
// structural assertion sees that: the shape is right, the names are
// right, and only the compiler knows the method sets differ.
//
// The build is run twice, and that is the point. A satisfaction file
// that fails to compile is not evidence of anything on its own — a
// misspelled interface, a support package that stopped compiling, an
// import the fixture forgot all fail it just as well, and each of
// them makes this assertion pass for the wrong reason. So the output
// must build without the assertion first, and the failure that
// follows must be the type checker rejecting the assignment rather
// than any other kind.
func (g *Generated) AssertDoesNotSatisfy(tb testing.TB, typeName, iface string) *Generated {
	tb.Helper()
	if !checkClaims(tb, []Satisfaction{{Type: typeName, Interface: iface}}) {
		return g
	}
	pkg, dir := g.primaryPackage(tb)
	if pkg == "" {
		return g
	}
	if out, err := g.exec(tb, []string{goBuild, allPackages}); err != nil {
		tb.Errorf("golangtest: the generated output does not build on its own, so a failing "+
			"%s/%s assertion would prove nothing about their method sets: %v\n"+
			"--- go build ---\n%s%s", typeName, iface, err, out, g.listing())
		return g
	}

	claims := []Satisfaction{{Type: typeName, Interface: iface}}
	out, err := g.exec(tb, []string{goBuild, allPackages}, satisfactionFile(pkg, dir, claims))
	switch {
	case err == nil:
		tb.Errorf("golangtest: *%s implements %s, which it must not — a near miss the "+
			"projection was supposed to reject type-checks as the real thing%s",
			typeName, iface, g.listing())
	case !strings.Contains(out, notImplementedMarker):
		tb.Errorf("golangtest: the %s/%s assertion failed to build for a reason other than "+
			"the method set, so it says nothing about whether %s implements %s: %v\n"+
			"--- go build ---\n%s%s", typeName, iface, typeName, iface, err, out, g.listing())
	}
	return g
}

// notImplementedMarker is the phrase the type checker uses when an
// assignment fails on the method set rather than on anything else —
// `*T does not implement I (wrong type for method M)`, and the
// missing-method and pointer-receiver variants of the same sentence.
//
// Matching on the compiler's prose is unlovely, but the alternative
// is treating every build failure as proof of non-satisfaction, which
// is the vacuity [Generated.AssertDoesNotSatisfy] exists to close. A
// toolchain that reworded this fails the assertion loudly rather than
// passing it quietly, which is the right way round for the mistake to
// go.
const notImplementedMarker = "does not implement"

// checkClaims reports whether every claim names something that can
// stand as a type, failing tb with the reason when one cannot.
//
// Both names are written verbatim into a generated file, so a caller
// passing something that is not a type expression gets a compiler
// error about a file they did not write, attributed to a line they
// cannot see. The observed mistake is passing an interface's *import
// path* where its spelling in the output package belongs —
// `example.com/storepkg.Store`, which surfaces as `syntax error:
// unexpected / after top level declaration` and names neither the
// assertion nor the argument.
//
// Checked here rather than left to the compiler because this is the
// one failure in the file that is the caller's error rather than the
// generator's, and the whole point of the assertion is that a build
// failure means the method sets disagree.
func checkClaims(tb testing.TB, claims []Satisfaction) bool {
	tb.Helper()
	ok := true
	for _, c := range claims {
		ok = checkTypeExpr(tb, "Type", c.Type) && ok
		ok = checkTypeExpr(tb, "Interface", c.Interface) && ok
	}
	return ok
}

// checkTypeExpr reports whether name can appear where the
// satisfaction file puts it.
//
// The slash is tested before the parser because an import path parses
// cleanly — `example.com/storepkg.Store` is a division of two selector
// expressions, a valid Go expression and never a type — so the parser
// alone accepts exactly the input this guard exists to reject.
func checkTypeExpr(tb testing.TB, role, name string) bool {
	tb.Helper()
	switch {
	case strings.TrimSpace(name) == "":
		tb.Errorf("golangtest: %s is empty, so there is nothing to assert about", role)
		return false
	case strings.Contains(name, "/"):
		tb.Errorf("golangtest: %s %q looks like an import path, not a type. Spell it as the "+
			"output package sees it — `storepkg.Store` — and make the declaring file "+
			"reachable with WithSource; a path here compiles to a division and reports a "+
			"syntax error against a file golangtest wrote", role, name)
		return false
	}
	if _, err := parser.ParseExpr(name); err != nil {
		tb.Errorf("golangtest: %s %q is not a Go type expression: %v", role, name, err)
		return false
	}
	return true
}

// satisfactionFile writes the file that makes the compiler answer a
// satisfaction question.
//
// Landed in the primary output's own directory and package, because
// both names are resolved from there: an unexported type could not be
// named from anywhere else, and an interface from elsewhere is
// spelled qualified against that file's imports.
func satisfactionFile(pkg, dir string, claims []Satisfaction) File {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n", pkg)
	for _, c := range claims {
		fmt.Fprintf(&b, "\n// Written by golangtest to ask whether %s implements %s.\n",
			c.Type, c.Interface)
		fmt.Fprintf(&b, "var _ %s = (*%s)(nil)\n", c.Interface, c.Type)
	}
	return File{
		Path: filepath.ToSlash(filepath.Join(dir, satisfiesFilename)),
		Src:  []byte(b.String()),
	}
}

// satisfiesFilename names the file golangtest writes its satisfaction
// assertions into. Prefixed so it cannot collide with generated
// output, and named in failures so a reader knows which lines the
// generator did not write.
const satisfiesFilename = "golangtest_satisfies.go"

// AssertInterfaceSatisfies fails when the generated *interface* does
// not cover the named one.
//
// A generator that emits an interface is making a promise to a
// consumer who already has one: their hand-written port, the
// framework's, the standard library's. Nothing else states that
// relation — [Generated.AssertSatisfies] takes the pointer form of a
// concrete type, which an interface has no useful version of, so
// every plugin in this position hand-wrote `var _ Contract =
// (Generated)(nil)` into a support file, where the failure surfaces
// as a compile error attributed to the fixture rather than as a named
// assertion.
func (g *Generated) AssertInterfaceSatisfies(tb testing.TB, iface, contract string) *Generated {
	tb.Helper()
	pkg, dir := g.primaryPackage(tb)
	if pkg == "" {
		return g
	}
	assertion := File{
		Path: filepath.ToSlash(filepath.Join(dir, satisfiesFilename)),
		Src: fmt.Appendf(nil,
			"package %s\n\n// Written by golangtest to prove %s covers %s.\nvar _ %s = (%s)(nil)\n",
			pkg, iface, contract, contract, iface),
	}
	g.runWith(tb, "satisfies", []string{goBuild, allPackages}, assertion)
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
	if out, err := g.exec(tb, args, extra...); err != nil {
		tb.Errorf("golangtest: go %s failed on the generated output: %v\n"+
			"--- go %s ---\n%s%s", label, err, label, out, g.listing())
	}
}

// exec assembles the module and runs one go subcommand in it,
// returning what it printed rather than reporting.
//
// Split out from [Generated.runWith] for the assertions whose subject
// is the failure itself: [Generated.AssertDoesNotSatisfy] needs to
// read the output and decide whether the compiler rejected the method
// set or something else entirely, which it cannot do once the failure
// has already been reported as the test's.
func (g *Generated) exec(tb testing.TB, args []string, extra ...File) (string, error) {
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

	err := cmd.Run()
	return out.String(), err
}

// moduleDir returns the directory holding the assembled module,
// reusing a previous one when the caller allows it.
//
// The cache is keyed on the TB that filled it, because [testing.TB.TempDir]
// ties the directory's lifetime to that TB: a fixture shared across
// sibling subtests — one asserting the output compiles, the next that
// it vets, which is how this package's own docs say to spend the
// budget — has the first subtest's directory removed before the second
// runs, and reusing the path would run `go` in a directory that no
// longer exists. Rebuilding for a new TB costs a few file writes; the
// `go` invocation that dominates the cost happens either way.
//
// Held under the lock for its whole length rather than around the
// cache read alone: two parallel subtests arriving together would
// otherwise each assemble a directory and race to record it, and the
// one whose record lost would still be running `go` in a directory
// nothing was tracking. The `go` invocation stays outside the lock,
// so parallel subtests overlap on the seconds that matter.
func (g *Generated) moduleDir(tb testing.TB, cacheable bool) string {
	tb.Helper()
	g.mu.Lock()
	defer g.mu.Unlock()
	if cacheable && g.built != "" && g.builtFor == tb {
		return g.built
	}
	dir := tb.TempDir()
	switch {
	case g.baseModule != "" && len(g.requires) > 0:
		// Composing them would mean rewriting a go.mod this package did
		// not write. Reported rather than silently dropping one, since
		// either outcome builds and only one is what the caller asked
		// for.
		tb.Fatalf("golangtest: InModule and WithRequire cannot both be set — " +
			"the base module's go.mod already declares its dependencies; " +
			"add the requirement there, or drop InModule")
		return ""
	case g.baseModule != "":
		copyTree(tb, g.baseModule, dir)
	default:
		writeFile(tb, dir, File{Path: goModFilename, Src: g.goModSrc(tb)})
	}
	for _, f := range append(cloned(g.support), g.files...) {
		writeFile(tb, dir, f)
	}
	if cacheable {
		g.built, g.builtFor = dir, tb
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
