// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// toolTimeout bounds one external invocation.
//
// A compiler or runner that hangs takes the whole package's test
// binary with it, and `go test`'s own timeout reports a goroutine
// dump rather than the command that stopped responding.
const toolTimeout = 2 * time.Minute

// toolchainEnv names the variable that turns an absent toolchain from
// a skip into a failure.
//
// Any non-empty value demands one; the CI workflow sets `required`.
// Non-empty rather than that one spelling because a typo'd value
// reverting to a skip would reopen the hole this closes, and nobody
// sets the variable meaning anything else.
//
// Both readings are right, in different places. A contributor without
// Node should still get the parse assertions rather than a red suite
// for a tool the repository does not require. A job that installed
// Node on purpose needs the opposite: there a skip means the install
// broke and the type check quietly stopped running, which is the one
// failure a green build cannot show. Nothing inside the process tells
// the two apart, so the job that installed the toolchain says so.
const toolchainEnv = "EIDOS_TYPESCRIPT_TOOLCHAIN"

// AssertParses fails when any generated file contains a syntax error.
//
// The floor every other assertion stands on, and the one that always
// runs: tree-sitter is linked into this package, so unlike the
// assertions below it needs nothing installed.
func (g *Generated) AssertParses(tb testing.TB) *Generated {
	tb.Helper()
	if len(g.files) == 0 {
		tb.Error("typescripttest: the run produced no files, so there is nothing to parse")
		return g
	}
	for _, f := range g.files {
		Parse(tb, f).AssertParses(tb)
	}
	return g
}

// AssertTypeChecks runs `tsc` over the generated output and the
// module it imports from.
//
// The only assertion that catches a dropped `?`, a wrong return type
// or a missing import — all three produce output that parses
// perfectly and fails in the consumer's build.
//
// Skips with a message when no TypeScript compiler answers, unless
// EIDOS_TYPESCRIPT_TOOLCHAIN is set, which turns that into a failure.
// See the package doc for why the default is a deliberate asymmetry
// rather than a soft assertion.
//
// # Cost
//
// Shells out: seconds, not milliseconds. Call it once per fixture
// rather than once per subtest, and let the [Source] assertions carry
// the fine-grained work. The assembled project is cached on the
// receiver, so several toolchain assertions over one [Generated] pay
// the setup once.
func (g *Generated) AssertTypeChecks(tb testing.TB) *Generated {
	tb.Helper()
	g.checkWith(tb, "type-check")
	return g
}

// Satisfaction is one "this type implements that contract" claim.
type Satisfaction struct {
	// Type is the generated type, named as the primary output
	// exports it.
	Type string

	// Interface is the contract it must satisfy.
	Interface string

	// From is the module the contract is imported from, as a path
	// relative to the project root — `user.ts`, or `./user`. Empty
	// means "wherever the project exports it", which is resolved by
	// looking.
	//
	// Set it when two modules export the same name, which is the one
	// case the resolution refuses to guess at. The declaring file has
	// to be reachable either way, which is what
	// [Generated.WithSource] is for.
	From string
}

// AssertSatisfies fails when the generated type does not implement
// the named contract.
//
// Type-checks `const _: Iface = null as unknown as Type` beside the
// output, which is the only assertion that catches the failures a
// double has that still parse: a property the contract says may be
// absent rendered as required, an optional method gone missing, a
// rest parameter flattened to an array so the double takes one
// argument where the contract takes many. Every one of those is valid
// TypeScript in isolation and satisfies nothing.
func (g *Generated) AssertSatisfies(tb testing.TB, typeName, iface string) *Generated {
	tb.Helper()
	return g.AssertSatisfiesAll(tb, Satisfaction{Type: typeName, Interface: iface})
}

// AssertSatisfiesAll proves several satisfaction claims in one check.
//
// A generator emitting one double per contract makes N of these
// claims per run, and [Generated.AssertSatisfies] pays a fresh
// project directory for each — the extra file bypasses the cache, so
// N claims cost N compiler runs for what is one file of N lines. At a
// second or three apiece that is the difference between a suite that
// keeps the assertion and one that drops it.
func (g *Generated) AssertSatisfiesAll(tb testing.TB, claims ...Satisfaction) *Generated {
	tb.Helper()
	if len(claims) == 0 {
		tb.Errorf("typescripttest: AssertSatisfiesAll was given no claims, so it proves nothing")
		return g
	}
	if !checkClaims(tb, claims) {
		return g
	}
	file, ok := g.satisfactionFile(tb, claims)
	if !ok {
		return g
	}
	g.checkWith(tb, "satisfy the claimed contracts", file)
	return g
}

// AssertInterfaceSatisfies fails when the generated *interface* does
// not cover the named contract.
//
// A generator that emits an interface is making a promise to a
// consumer who already has one: their hand-written port, a
// framework's, the DOM's. Nothing else states that relation — the
// claim is between two types rather than about a value a constructor
// produced — so every plugin in this position hand-wrote the
// assignment into a support file, where the failure surfaces as a
// type error attributed to the fixture rather than as a named
// assertion.
//
// The same file shape answers it, because TypeScript's assignability
// is structural: a value typed as the generated interface being
// assignable to the contract is exactly the claim, and there is no
// pointer form to take as there is in Go.
func (g *Generated) AssertInterfaceSatisfies(tb testing.TB, iface, contract string) *Generated {
	tb.Helper()
	return g.AssertSatisfiesAll(tb, Satisfaction{Type: iface, Interface: contract})
}

// AssertDoesNotSatisfy fails when the generated type *does* implement
// the named contract.
//
// The direction that carries the weight for a detector or a
// heuristic. [Generated.AssertSatisfies] proves one type passes; what
// a shape detector actually claims is that every near miss does not,
// and a near miss is by construction something that looks right. A
// frontend records a rest parameter by its element type, so
// `write(...p: string[])` reaches a projection spelled exactly like
// `write(p: string[])` — and a detector that accepted it wrote a
// conformance for a type no consumer can pass. No structural
// assertion sees that: the shape is right, the names are right, and
// only the type checker knows the two differ.
//
// The check is run twice, and that is the point. An assertion file
// that fails to compile is not evidence of anything on its own — a
// misspelled contract, a support module that stopped compiling, an
// import the fixture forgot all fail it just as well, and each of
// them makes this assertion pass for the wrong reason. So the output
// must type-check without the assertion first, and the failure that
// follows must be the checker rejecting the assignment rather than
// any other kind.
func (g *Generated) AssertDoesNotSatisfy(tb testing.TB, typeName, iface string) *Generated {
	tb.Helper()
	claims := []Satisfaction{{Type: typeName, Interface: iface}}
	if !checkClaims(tb, claims) {
		return g
	}
	file, ok := g.satisfactionFile(tb, claims)
	if !ok {
		return g
	}
	tsc, ok := g.compiler(tb)
	if !ok {
		return g
	}

	if out, err := g.exec(tb, tsc); err != nil {
		tb.Errorf("typescripttest: the generated output does not type-check on its own, so a "+
			"failing %s/%s assertion would prove nothing about their shapes: %v\n"+
			"--- tsc ---\n%s%s", typeName, iface, err, out, g.listing())
		return g
	}

	out, err := g.exec(tb, tsc, file)
	switch {
	case err == nil:
		tb.Errorf("typescripttest: %s satisfies %s, which it must not — a near miss the "+
			"projection was supposed to reject type-checks as the real thing%s",
			typeName, iface, g.listing())
	case !mentionsAssignability(out):
		tb.Errorf("typescripttest: the %s/%s assertion failed for a reason other than the "+
			"two shapes, so it says nothing about whether %s satisfies %s: %v\n"+
			"--- tsc ---\n%s%s", typeName, iface, typeName, iface, err, out, g.listing())
	}
	return g
}

// AssertTestsPass runs the generated test files.
//
// The loop nothing else closes. A generator whose output is a test
// suite has that suite as its real contract, and asserting on the
// text of a case — that a `test` of some name exists — passes just as
// well when the case is empty, wrong, or asserts nothing at all.
//
// Runs `node --test` by default, which needs no dependency because
// the runner is in the standard library. Node executes TypeScript by
// *stripping* types rather than transforming them, so a suite that
// reaches a declaration TypeScript compiles to runtime code — an
// `enum`, a `namespace`, a parameter property — cannot run under it.
// That is reported as the runner's limit rather than as a failing
// test, because the two call for different fixes. Point the assertion
// at a transpiling runner with [Generated.WithTestRunner] when the
// output needs one.
//
// Skips when no Node answers, on the same terms as
// [Generated.AssertTypeChecks] — and fails instead under
// EIDOS_TYPESCRIPT_TOOLCHAIN.
func (g *Generated) AssertTestsPass(tb testing.TB) *Generated {
	tb.Helper()
	if !g.hasTests() {
		tb.Errorf("typescripttest: the run produced no test file, so there is no generated "+
			"suite to run (have %v)", g.paths())
		return g
	}
	runner, ok := g.testRunner(tb)
	if !ok {
		return g
	}
	out, err := g.exec(tb, runner)
	switch {
	case err == nil:
		return g
	case strings.Contains(out, unsupportedSyntaxMarker):
		tb.Errorf("typescripttest: %s strips TypeScript types rather than transforming them, "+
			"so it cannot execute this output — declare a transpiling runner with "+
			"WithTestRunner, or assert on the suite structurally:\n%s",
			filepath.Base(runner[0]), strings.TrimSpace(out))
	default:
		tb.Errorf("typescripttest: the generated test suite failed:\n%s%s",
			strings.TrimSpace(out), g.listing())
	}
	return g
}

// unsupportedSyntaxMarker is what Node raises for a construct
// type-stripping cannot remove.
const unsupportedSyntaxMarker = "ERR_UNSUPPORTED_TYPESCRIPT_SYNTAX"

// assignabilityCodes are the diagnostics tsc raises when an
// assignment fails on the two types' shapes rather than on anything
// else.
//
// Matched on the codes rather than the prose, which is reworded
// between releases; and on a set rather than one, because the checker
// picks the most specific it can — a missing property reports TS2739
// where a wrong one reports TS2322, and both are the answer this
// assertion asks for. Anything outside the set is a different
// failure, and treating it as proof of non-satisfaction is the
// vacuity [Generated.AssertDoesNotSatisfy] exists to close.
var assignabilityCodes = []string{ //nolint:gochecknoglobals // immutable lookup table
	"TS2322", // Type 'X' is not assignable to type 'Y'.
	"TS2375", // ... with 'exactOptionalPropertyTypes: true'.
	"TS2379", // Argument of type 'X' is not assignable ... with exactOptional.
	"TS2416", // Property 'p' is not assignable to the same property in the base type.
	"TS2420", // Class 'X' incorrectly implements interface 'Y'.
	"TS2739", // Type 'X' is missing the following properties from type 'Y'.
	"TS2741", // Property 'p' is missing in type 'X' but required in type 'Y'.
}

// mentionsAssignability reports whether tsc's output names a
// shape-mismatch diagnostic.
func mentionsAssignability(out string) bool {
	return slices.ContainsFunc(assignabilityCodes, func(code string) bool {
		return strings.Contains(out, code)
	})
}

// checkClaims reports whether every claim names something the
// assertion file can import, failing tb with the reason when one
// cannot.
//
// Both names are written verbatim into a file this package generates,
// so a caller passing something else gets a compiler error about a
// file they did not write, attributed to a line they cannot see.
// Checked here rather than left to tsc because this is the one
// failure in the file that is the caller's error rather than the
// generator's, and the whole point of the assertion is that a check
// failure means the shapes disagree.
func checkClaims(tb testing.TB, claims []Satisfaction) bool {
	tb.Helper()
	ok := true
	for _, c := range claims {
		ok = checkTypeName(tb, "Type", c.Type) && ok
		ok = checkTypeName(tb, "Interface", c.Interface) && ok
	}
	return ok
}

// checkTypeName reports whether name can appear where the assertion
// file puts it.
//
// A name rather than an expression: the file imports it, and an
// import binds one identifier. `Box<string>` is a legal type and not
// a legal import, and `./user.User` is a path where a name belongs —
// the observed mistake, and one that reaches tsc as a syntax error
// naming neither the assertion nor the argument.
func checkTypeName(tb testing.TB, role, name string) bool {
	tb.Helper()
	trimmed := strings.TrimSpace(name)
	switch {
	case trimmed == "":
		tb.Errorf("typescripttest: %s is empty, so there is nothing to assert about", role)
		return false
	case strings.ContainsAny(trimmed, "<>[]|&(){}.,/ "):
		tb.Errorf("typescripttest: %s %q is a type expression or a path, not a name. The "+
			"assertion imports it and an import binds one identifier — name the module "+
			"with Satisfaction.From, and declare an alias for anything compound",
			role, name)
		return false
	}
	return true
}

// satisfiesFilename names the file this package writes its
// satisfaction assertions into.
//
// Prefixed so it cannot collide with generated output, and named in
// failures so a reader knows which lines the generator did not write.
// At the project root, so the specifiers it composes are the paths
// the caller already holds rather than a relative walk from wherever
// the primary output landed.
const satisfiesFilename = "typescripttest_satisfies.ts"

// satisfactionFile writes the file that makes the checker answer a
// satisfaction question, resolving each name to the module that
// exports it.
//
// `null as unknown as T` rather than a constructed value: the claim
// is about the two types' shapes, and a constructor call would drag
// in whatever the type needs to be built — which is not what is being
// asked, and which a type carrying required members cannot supply at
// all. The double assertion is what gets a value of any type past the
// checker without claiming one exists.
func (g *Generated) satisfactionFile(tb testing.TB, claims []Satisfaction) (File, bool) {
	tb.Helper()
	byModule := map[string]map[string]struct{}{}
	ok := true
	record := func(module, name string) {
		if module == "" {
			ok = false
			return
		}
		names, seen := byModule[module]
		if !seen {
			names = map[string]struct{}{}
			byModule[module] = names
		}
		names[name] = struct{}{}
	}
	for _, c := range claims {
		record(g.moduleOf(tb, c.Type, ""), c.Type)
		record(g.moduleOf(tb, c.Interface, c.From), c.Interface)
	}
	if !ok {
		return File{}, false
	}

	var b strings.Builder
	b.WriteString("// Written by typescripttest. DO NOT EDIT.\n\n")
	for _, module := range slices.Sorted(maps.Keys(byModule)) {
		names := slices.Sorted(maps.Keys(byModule[module]))
		fmt.Fprintf(&b, "import type { %s } from '%s';\n", strings.Join(names, ", "), module)
	}
	for i, c := range claims {
		fmt.Fprintf(&b, "\n// Asks whether %s satisfies %s.\n", c.Type, c.Interface)
		fmt.Fprintf(&b, "const claim%d: %s = null as unknown as %s;\n", i, c.Interface, c.Type)
		// Referenced so noUnusedLocals does not reject the file for
		// declaring the very binding the assertion is made of.
		fmt.Fprintf(&b, "void claim%d;\n", i)
	}
	return File{Path: satisfiesFilename, Src: []byte(b.String())}, true
}

// moduleOf returns the specifier name is imported under, empty when
// nothing exports it or several things do.
//
// Resolved by looking rather than assumed, because the common shape
// puts the two names in different modules: the generated double is in
// the output and the contract it satisfies is in the module a
// consumer wrote. A harness that assumed one module would make the
// two-argument form unusable for exactly the case it exists for, and
// the failure — `Module has no exported member` against a file the
// caller never wrote — names neither the assertion nor the fix.
//
// An explicit From short-circuits it, which is what settles the one
// case looking cannot: two modules exporting one name.
func (g *Generated) moduleOf(tb testing.TB, name, from string) string {
	tb.Helper()
	if strings.TrimSpace(from) != "" {
		return moduleSpecifier(from)
	}

	var found []string
	for _, f := range append(cloned(g.files), g.support...) {
		if f.IsTestFile() || !exportsName(tb, f, name) {
			continue
		}
		found = append(found, moduleSpecifier(f.Path))
	}
	switch len(found) {
	case 1:
		return found[0]
	case 0:
		tb.Errorf("typescripttest: no module in the project exports %s, so there is nothing "+
			"to assert about — make its module reachable with WithSource", name)
	default:
		tb.Errorf("typescripttest: %s is exported by %v, so importing it would pick one at "+
			"random — name the module with Satisfaction.From", name, found)
	}
	return ""
}

// exportsName reports whether the file exports a top-level
// declaration of that name.
func exportsName(tb testing.TB, f File, name string) bool {
	tb.Helper()
	s := Parse(tb, f)
	if s == nil {
		return false
	}
	for _, n := range s.topLevel() {
		if declName(n, s.src) == name && exported(n) {
			return true
		}
	}
	return false
}

// moduleSpecifier turns a project-relative path into the specifier an
// import writes.
//
// The extension is dropped and `./` prepended, because a specifier
// with neither is a *package* name under every resolver — `user`
// resolves in node_modules, `./user` resolves on disk — and that
// difference is the whole failure a caller passing a bare path hits.
func moduleSpecifier(path string) string {
	spec := strings.TrimSuffix(filepath.ToSlash(path), ".ts")
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		return spec
	}
	return "./" + spec
}

// hasTests reports whether the run produced anything a runner would
// execute.
func (g *Generated) hasTests() bool {
	return slices.ContainsFunc(g.files, File.IsTestFile)
}

// checkWith assembles the project and runs the compiler over it, with
// any extra files written on top.
//
// The extra files bypass the cache deliberately: they vary per
// assertion, and reusing a directory that holds a previous
// assertion's file would check something the caller did not ask for.
func (g *Generated) checkWith(tb testing.TB, what string, extra ...File) {
	tb.Helper()

	// Assembled before the compiler is probed for, so a fixture that
	// cannot be written fails rather than skipping: "no compiler" is
	// the wrong diagnosis for a support file with an unwritable path,
	// and it is the one a caller would act on.
	g.projectDir(tb, len(extra) == 0)

	tsc, ok := g.compiler(tb)
	if !ok {
		return
	}
	if out, err := g.exec(tb, tsc, extra...); err != nil {
		tb.Errorf("typescripttest: the generated TypeScript does not %s:\n%s\n%s",
			what, strings.TrimSpace(out), g.listing())
	}
}

// compiler resolves the TypeScript compiler's argv, skipping the test
// when none answers.
//
// The project is named as `.` rather than by path, because the
// command already runs in it and an absolute `--project` reports
// every diagnostic against a temp path the reader cannot open.
func (*Generated) compiler(tb testing.TB) ([]string, bool) {
	tb.Helper()
	tsc, ok := findTSC(tb)
	if !ok {
		return nil, false
	}
	return append(slices.Clone(tsc), "--project", "."), true
}

// testRunner resolves the argv that runs the generated suite.
func (g *Generated) testRunner(tb testing.TB) ([]string, bool) {
	tb.Helper()
	if len(g.runner) > 0 {
		return g.runner, true
	}
	node, err := exec.LookPath("node")
	if err != nil {
		reportMissing(tb, os.Getenv(toolchainEnv),
			"no Node answered, so the generated suite cannot be run",
			"install Node, or declare a runner with WithTestRunner")
		return nil, false
	}
	return []string{node, "--test"}, true
}

// exec assembles the project and runs one command in it, returning
// what it printed rather than reporting.
//
// Split out from [Generated.checkWith] for the assertions whose
// subject is the failure itself: [Generated.AssertDoesNotSatisfy]
// needs to read the output and decide whether the checker rejected
// the shapes or something else entirely, which it cannot do once the
// failure has already been reported as the test's.
func (g *Generated) exec(tb testing.TB, argv []string, extra ...File) (string, error) {
	tb.Helper()
	dir := g.projectDir(tb, len(extra) == 0)
	for _, f := range extra {
		writeFile(tb, dir, f)
	}

	ctx, cancel := context.WithTimeout(tb.Context(), toolTimeout)
	defer cancel()

	//nolint:gosec // a resolved tool path
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// listing renders every file in the project with line numbers.
//
// Attached to every toolchain failure because the compiler's message
// names a position in code the author never wrote, in a temp
// directory that no longer exists by the time they read it. Without
// this the assertion is worse than the substring check it replaces.
func (g *Generated) listing() string {
	var b strings.Builder
	for _, f := range append(cloned(g.files), g.support...) {
		fmt.Fprintf(&b, "\n--- %s ---\n%s", f.Path, numbered(f.Src))
	}
	return b.String()
}

// findTSC resolves the TypeScript compiler, skipping the test when
// none answers.
//
// Three places, in the order a developer would look: the project's
// own `node_modules`, PATH, and `npx` — which resolves a workspace
// install this package cannot see the layout of. Each is probed
// rather than assumed, because a probe that guessed wrong reports the
// generator broken when the toolchain is merely absent.
func findTSC(tb testing.TB) ([]string, bool) {
	tb.Helper()

	if local, err := filepath.Abs(filepath.Join("node_modules", ".bin", "tsc")); err == nil {
		if info, statErr := os.Stat(local); statErr == nil && !info.IsDir() {
			return []string{local}, true
		}
	}
	if path, err := exec.LookPath("tsc"); err == nil {
		return []string{path}, true
	}
	if path, err := exec.LookPath("npx"); err == nil && npxHasTSC(path) {
		return []string{path, "--no-install", "tsc"}, true
	}

	reportMissing(tb, os.Getenv(toolchainEnv),
		"no TypeScript compiler answered",
		"install one with `npm i -D typescript`, or put tsc on PATH")
	return nil, false
}

// npxHasTSC reports whether npx can resolve tsc without installing
// it.
//
// `--no-install` is what makes the probe honest: without it npx
// downloads the compiler on first use, which turns a skipped
// assertion into a test that hangs on a cold cache and fails with no
// network.
//
// The output is read as well as the exit status, because `npx tsc`
// resolves the npm package named `tsc` — a 2016 stub, not the
// compiler, which ships as `typescript`. A cache holding one would
// otherwise satisfy the probe and every type check would run against
// it. Real tsc answers `--version` with `Version 5.9.3`.
func npxHasTSC(npx string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, npx, "--no-install", "tsc", "--version") //nolint:gosec // a resolved npx path
	out, err := cmd.Output()
	return err == nil && isVersionLine(string(out))
}

// isVersionLine reports whether out is what `tsc --version` prints.
func isVersionLine(out string) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(out), "Version ")
	if !ok || rest == "" {
		return false
	}
	return rest[0] >= '0' && rest[0] <= '9'
}

// reportMissing states that a tool did not answer — as a skip, or as
// a failure when the environment demands one.
//
// The setting is passed in rather than read here so both branches are
// reachable from a test: these assertions run in parallel, and
// [testing.T.Setenv] refuses to run beside [testing.T.Parallel].
func reportMissing(tb testing.TB, setting, what, fix string) {
	tb.Helper()
	if setting != "" {
		tb.Fatalf("typescripttest: %s, and %s=%s demands one. The job setting that variable "+
			"installs the toolchain, so this is a broken install rather than an absent "+
			"one — %s", what, toolchainEnv, setting, fix)
		return
	}
	tb.Skip("typescripttest: " + what + " — " + fix + ". The parse assertions still ran.")
}
