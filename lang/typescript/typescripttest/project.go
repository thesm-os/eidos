// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The throwaway project every toolchain assertion runs in.
//
// A TypeScript project is a directory holding the sources, a
// tsconfig deciding how strictly they are read, and — where the
// output imports anything installed — a package.json and a
// node_modules beside it. This file assembles all three; toolchain.go
// runs things in the result.

// The files this package writes into every project.
const (
	tsconfigFilename    = "tsconfig.json"
	packageJSONFilename = "package.json"
)

// defaultPackageName names a project the caller did not.
//
// Deliberately not a plausible package name: a self-reference in the
// generated output should fail to resolve rather than resolve by
// accident against whatever this happened to be called.
const defaultPackageName = "typescripttest-fixture"

// defaultCompilerOptions is the tsconfig this package checks with.
//
// The strict ones, deliberately. A generator's output is read by
// projects that turn these on, and a check run without them passes on
// output that fails for the consumer — `exactOptionalPropertyTypes`
// alone is the difference between `x?: T` and `x: T | undefined`,
// which is exactly the distinction the TypeScript backend takes care
// to render. Relaxing one is a claim a test makes on purpose through
// [Generated.WithCompilerOptions].
//
// `noEmit` because nothing here compiles the output to JavaScript:
// the type check answers the question, and
// [Generated.AssertTestsPass] runs the sources directly.
func defaultCompilerOptions() map[string]any {
	return map[string]any{
		"target":                           "ES2022",
		"module":                           "ESNext",
		"moduleResolution":                 "bundler",
		"strict":                           true,
		"exactOptionalPropertyTypes":       true,
		"noUnusedLocals":                   true,
		"noUnusedParameters":               true,
		"noEmit":                           true,
		"skipLibCheck":                     true,
		"forceConsistentCasingInFileNames": true,
	}
}

// WithCompilerOptions merges entries into the `compilerOptions` of
// the tsconfig the project is checked with.
//
// Worth setting deliberately; see [defaultCompilerOptions] for what
// is on by default and why.
func (g *Generated) WithCompilerOptions(opts map[string]any) *Generated {
	if g.options == nil {
		g.options = map[string]any{}
	}
	maps.Copy(g.options, opts)
	g.built = ""
	return g
}

// WithTarget pins the `target` and `lib` the output is checked
// against.
//
// Worth setting deliberately for the same reason a Go fixture pins
// its `go` directive: a template that starts emitting a construct
// from a later edition raises the floor of every project that runs
// the generator, and nothing else notices. `lib` follows the target
// rather than being set separately — a target with a mismatched lib
// type-checks against a runtime it will not have.
func (g *Generated) WithTarget(target string) *Generated {
	return g.WithCompilerOptions(map[string]any{
		"target": target,
		"lib":    []string{target},
	})
}

// WithPackageName names the project, which is what a self-reference
// resolves against.
//
// TypeScript's counterpart to pinning a module path. Output that
// imports its own package by name rather than by relative path —
// `import { User } from "@acme/models"` inside `@acme/models` — is
// resolved through the package's own name, so a project named
// anything else reports the import unresolved and the failure names
// the specifier rather than the setting.
//
// Defaults to a fixture name no real package uses, which is the right
// answer for the common case: output that imports only relative
// paths does not care, and one that guessed a plausible name would
// make a self-reference resolve by accident.
func (g *Generated) WithPackageName(name string) *Generated {
	g.packageName = name
	g.built = ""
	return g
}

// WithDependency makes a local package importable from the generated
// code.
//
// Links dir into the project's `node_modules` under the given name
// and records it in package.json, so output importing the
// generator's *own* runtime library resolves against the checkout on
// disk rather than a published release of it.
//
// The gap between the two things this package could already do.
// Output importing nothing but its own graph is served by
// [Generated.WithSource]; output importing a whole tree is served by
// [Generated.InProject], which copies an existing project wholesale.
// Neither serves the common case — a runtime library sitting in the
// same repository — and every consumer that hit it hand-assembled the
// same node_modules.
//
// Repeatable; each call adds one package. Not combinable with
// [Generated.InProject], which brings a node_modules this would have
// to merge into rather than compose with.
func (g *Generated) WithDependency(name, dir string) *Generated {
	g.deps = append(g.deps, dependency{name: name, dir: dir})
	g.built = ""
	return g
}

// InProject assembles inside a copy of an existing project directory,
// so the generated files are checked against its tsconfig and its
// installed packages.
//
// The escape hatch for output that imports a real dependency tree —
// a framework's decorators, a validation library's types — which
// neither a synthetic tsconfig nor a linked directory can stand in
// for. The copy is what keeps the assertion from writing into the
// caller's own checkout.
//
// The base project's tsconfig is used as it stands:
// [Generated.WithCompilerOptions] does not compose with this, because
// the point is to check against what the consumer actually has.
func (g *Generated) InProject(dir string) *Generated {
	g.baseProject = dir
	g.built = ""
	return g
}

// WithTestRunner declares the command [Generated.AssertTestsPass]
// runs the generated suite with.
//
// For output whose suite imports a framework — `vitest`, `jest` —
// rather than `node:test`, and for output Node's type-stripping
// cannot execute. The command runs in the project root; make the
// framework reachable with [Generated.WithDependency] or
// [Generated.InProject].
func (g *Generated) WithTestRunner(name string, args ...string) *Generated {
	g.runner = append([]string{name}, args...)
	return g
}

// dependency is one local package the generated code imports.
type dependency struct {
	// name is the package name the generated code imports under.
	name string

	// dir is the checkout the link points at.
	dir string
}

// projectDir returns the directory holding the assembled project,
// reusing a previous one when the caller allows it.
//
// The cache is keyed on the TB that filled it, because
// [testing.TB.TempDir] ties the directory's lifetime to that TB: a
// fixture shared across sibling subtests — one asserting the output
// type-checks, the next that its suite passes, which is how this
// package's own docs say to spend the budget — has the first
// subtest's directory removed before the second runs, and reusing the
// path would run a tool in a directory that no longer exists.
//
// Held under the lock for its whole length rather than around the
// cache read alone: two parallel subtests arriving together would
// otherwise each assemble a directory and race to record it, and the
// one whose record lost would still be running a tool in a directory
// nothing was tracking. The tool invocation stays outside the lock,
// so parallel subtests overlap on the seconds that matter.
func (g *Generated) projectDir(tb testing.TB, cacheable bool) string {
	tb.Helper()
	g.mu.Lock()
	defer g.mu.Unlock()
	if cacheable && g.built != "" && g.builtFor == tb {
		return g.built
	}

	dir := tb.TempDir()
	switch {
	case g.baseProject != "" && len(g.deps) > 0:
		// Composing them would mean merging into a node_modules this
		// package did not assemble. Reported rather than silently
		// dropping one, since either outcome runs and only one is what
		// the caller asked for.
		tb.Fatalf("typescripttest: InProject and WithDependency cannot both be set — " +
			"the base project already has its own node_modules; install the package " +
			"there, or drop InProject")
		return ""
	case g.baseProject != "":
		copyTree(tb, g.baseProject, dir)
	default:
		g.writeTSConfig(tb, dir)
	}
	g.writePackageJSON(tb, dir)
	g.linkDependencies(tb, dir)

	for _, f := range append(cloned(g.support), g.files...) {
		writeFile(tb, dir, f)
	}
	if cacheable {
		g.built, g.builtFor = dir, tb
	}
	return dir
}

// writeTSConfig writes the project's tsconfig.
func (g *Generated) writeTSConfig(tb testing.TB, dir string) {
	tb.Helper()
	opts := defaultCompilerOptions()
	maps.Copy(opts, g.options)
	reconcile(opts)
	writeJSON(tb, dir, tsconfigFilename, map[string]any{
		"compilerOptions": opts,
		"include":         []string{"**/*.ts"},
	})
}

// writePackageJSON declares the project a module and names whatever
// was linked into it.
//
// `"type": "module"` because every specifier this package composes is
// an ES import, and a directory without it is read as CommonJS — a
// difference that surfaces only when something runs the output, which
// is exactly when it is hardest to diagnose.
//
// Skipped for a base project, which brought its own.
func (g *Generated) writePackageJSON(tb testing.TB, dir string) {
	tb.Helper()
	if g.baseProject != "" {
		return
	}
	deps := map[string]any{}
	for _, d := range g.deps {
		deps[d.name] = "file:" + d.dir
	}
	name := g.packageName
	if name == "" {
		name = defaultPackageName
	}
	pkg := map[string]any{
		"name":    name,
		"private": true,
		"type":    "module",
	}
	if len(deps) > 0 {
		pkg["dependencies"] = deps
	}
	writeJSON(tb, dir, packageJSONFilename, pkg)
}

// linkDependencies places each declared package under node_modules.
//
// Copied rather than symlinked, and installed rather than fetched:
// the directory on disk is what gets checked, which is what a test of
// a generator wants — the runtime it is developed against, not a
// published release of it. A copy also means the assertion cannot
// write into the caller's own checkout through the link.
func (g *Generated) linkDependencies(tb testing.TB, dir string) {
	tb.Helper()
	for _, d := range g.deps {
		copyTree(tb, d.dir, filepath.Join(dir, "node_modules", filepath.FromSlash(d.name)))
	}
}

// reconcile drops options tsc refuses in combination with what the
// caller asked for.
//
// `exactOptionalPropertyTypes` is rejected outright without
// `strictNullChecks`, which `strict: false` turns off — so a test
// relaxing strictness would otherwise get TS5052 about a tsconfig it
// did not write, instead of the relaxed check it asked for. The
// default is dropped rather than the caller's request overridden:
// relaxing an option is a claim a test makes on purpose, and a
// harness that quietly re-imposed it would make the claim
// unexpressible.
func reconcile(opts map[string]any) {
	if opts["exactOptionalPropertyTypes"] != true {
		return
	}
	if opts["strictNullChecks"] == true {
		return
	}
	if opts["strict"] != true {
		delete(opts, "exactOptionalPropertyTypes")
	}
}

// writeJSON encodes body into one of the project's config files.
//
// Sorted keys, which [encoding/json] gives for a map, so two runs
// over one fixture write the same bytes — a config that churned would
// make every failure dump differ from the last.
func writeJSON(tb testing.TB, dir, name string, body map[string]any) {
	tb.Helper()
	encoded, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		tb.Fatalf("typescripttest: cannot encode %s: %v", name, err)
	}
	writeFile(tb, dir, File{Path: name, Src: append(encoded, '\n')})
}

// copyTree copies a directory into the project.
//
// [os.CopyFS] rather than a hand-rolled walk: it resolves every path
// inside the source root, so a symlink in a fixture cannot reach
// outside it, and it is the operation this needs rather than an
// assembly of four that happen to compose into it.
func copyTree(tb testing.TB, src, dst string) {
	tb.Helper()
	if err := os.MkdirAll(dst, 0o750); err != nil {
		tb.Fatalf("typescripttest: create %s: %v", dst, err)
	}
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		tb.Fatalf("typescripttest: copy %s: %v", src, err)
	}
}

// cloned copies a file list, so appending to it cannot disturb the
// receiver's own.
func cloned(in []File) []File { return slices.Clone(in) }

// writeFile writes one file under root, creating its directory.
func writeFile(tb testing.TB, root string, f File) {
	tb.Helper()
	full := filepath.Join(root, filepath.FromSlash(f.Path))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		tb.Fatalf("typescripttest: create %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, f.Src, 0o600); err != nil {
		tb.Fatalf("typescripttest: write %s: %v", full, err)
	}
}
