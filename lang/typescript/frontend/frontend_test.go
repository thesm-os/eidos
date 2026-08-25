// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/lang/typescript/frontend"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// project writes files into a fresh directory and returns its path.
func project(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

// run loads pattern against root and returns the populated store.
func run(t *testing.T, root, pattern string, c cache.Cache) (*store.Store, *diag.Sink) {
	t.Helper()
	f := frontend.New()
	if err := f.SetOptions(opt.New(f.OptionsSchema(), map[string]string{"dir": root})); err != nil {
		t.Fatalf("SetOptions: %v", err)
	}

	st, sink := store.New(), diag.New()
	ctx := &plugin.FrontendContext{
		Store:       st,
		Diag:        sink,
		Registry:    directive.NewRegistry(),
		Parser:      directive.DefaultParser(),
		Cache:       c,
		Pattern:     pattern,
		Fingerprint: "test-fingerprint",
	}
	if err := f.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return st, sink
}

// declaredNames returns the name of every type declaration in the
// store, across both the interface and struct buckets.
//
// Spanning both because a test asking "did this declaration reach the
// store" does not care which bucket it landed in — a TypeScript
// interface is a node.Interface and a class is a node.Struct, and a
// helper keyed on one of them silently answers "no" for the other.
func declaredNames(st *store.Store) map[string]bool {
	out := map[string]bool{}
	for _, p := range packages(st) {
		for _, s := range p.Structs {
			out[s.Name] = true
		}
		for _, i := range p.Interfaces {
			out[i.Name] = true
		}
	}
	return out
}

// packages returns every package the store holds.
func packages(st *store.Store) []*node.Package {
	var out []*node.Package
	st.Nodes().Packages().Range(func(p *node.Package) bool {
		out = append(out, p)
		return true
	})
	return out
}

func TestFrontendLoad(t *testing.T) {
	t.Parallel()

	t.Run("declarations reach the store under one package per directory", func(t *testing.T) {
		t.Parallel()
		root := project(t, map[string]string{
			"src/user.ts": "export interface User { id: string; }\nexport type ID = string;\n",
			"src/role.ts": "export enum Role { Admin = 'admin' }\n",
		})

		st, sink := run(t, root, "./src/...", cache.NewNone())
		if errs := errorsIn(sink); len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		pkgs := packages(st)
		if len(pkgs) != 1 {
			t.Fatalf("packages = %d, want 1 (per directory)", len(pkgs))
		}
		p := pkgs[0]
		if len(p.Interfaces) != 1 || len(p.Aliases) != 1 || len(p.Enums) != 1 {
			t.Fatalf("interfaces=%d aliases=%d enums=%d, want 1 each",
				len(p.Interfaces), len(p.Aliases), len(p.Enums))
		}
		if p.InterfaceByName("User") == nil {
			t.Fatal("InterfaceByName cannot reach a declared interface")
		}
		if len(p.Files) != 2 {
			t.Fatalf("Files = %d, want 2", len(p.Files))
		}
	})

	t.Run("every package carries the frontend marker", func(t *testing.T) {
		t.Parallel()
		// A pipeline reading both Go and TypeScript writes into one
		// store; this is what tells the two apart.
		root := project(t, map[string]string{"a.ts": "export interface A {}\n"})
		st, _ := run(t, root, "./...", cache.NewNone())

		got, ok := frontend.MetaFrontend.Get(packages(st)[0].Meta())
		if !ok || got != frontend.FrontendName {
			t.Fatalf("frontend marker = %q (present: %v), want %q",
				got, ok, frontend.FrontendName)
		}
	})

	t.Run("separate directories become separate packages", func(t *testing.T) {
		t.Parallel()
		root := project(t, map[string]string{
			"a/one.ts": "export interface A {}\n",
			"b/two.ts": "export interface B {}\n",
		})
		st, _ := run(t, root, "./...", cache.NewNone())
		if got := len(packages(st)); got != 2 {
			t.Fatalf("packages = %d, want 2", got)
		}
	})

	t.Run("node_modules is skipped without descending into it", func(t *testing.T) {
		t.Parallel()
		root := project(t, map[string]string{
			"src/a.ts":                        "export interface A {}\n",
			"src/node_modules/pkg/index.ts":   "export interface Vendored {}\n",
			"src/node_modules/pkg/other.d.ts": "export declare const x: number;\n",
		})
		st, _ := run(t, root, "./src/...", cache.NewNone())

		if declaredNames(st)["Vendored"] {
			t.Fatal("a node_modules declaration reached the store")
		}
	})

	t.Run("test files are excluded by default", func(t *testing.T) {
		t.Parallel()
		root := project(t, map[string]string{
			"a.ts":      "export interface Real {}\n",
			"a.test.ts": "export interface FromTest {}\n",
			"a.spec.ts": "export interface FromSpec {}\n",
		})
		st, _ := run(t, root, "./...", cache.NewNone())

		names := declaredNames(st)
		if !names["Real"] {
			t.Error("production declaration missing")
		}
		if names["FromTest"] || names["FromSpec"] {
			t.Error("a test file contributed declarations")
		}
	})

	t.Run("a generated file is not re-parsed as fresh source", func(t *testing.T) {
		t.Parallel()
		root := project(t, map[string]string{
			"hand.ts": "export interface Hand {}\n",
			"gen.ts":  "// Code generated by eidos. DO NOT EDIT.\n\nexport interface Gen {}\n",
		})
		st, _ := run(t, root, "./...", cache.NewNone())

		if declaredNames(st)["Gen"] {
			t.Fatal("generated file was parsed")
		}
	})

	t.Run("a syntax error warns and keeps the rest of the file", func(t *testing.T) {
		t.Parallel()
		// tree-sitter recovers, so declarations either side of the
		// error still convert. Dropping the file would discard them.
		root := project(t, map[string]string{
			"broken.ts": "export interface Good { a: string; }\nexport interface { \nexport interface AlsoGood { b: string; }\n",
		})
		st, sink := run(t, root, "./...", cache.NewNone())

		if len(warningsIn(sink)) == 0 {
			t.Error("syntax error produced no diagnostic")
		}
		if !declaredNames(st)["Good"] {
			t.Error("declarations before the syntax error were lost")
		}
	})

	t.Run("an empty pattern is rejected", func(t *testing.T) {
		t.Parallel()
		f := frontend.New()
		err := f.Load(&plugin.FrontendContext{
			Store: store.New(), Diag: diag.New(),
			Registry: directive.NewRegistry(), Parser: directive.DefaultParser(),
			Cache: cache.NewNone(), Pattern: "  ",
		})
		if err == nil {
			t.Fatal("empty pattern accepted")
		}
	})

	t.Run("a pattern matching nothing is an error not a silent no-op", func(t *testing.T) {
		t.Parallel()
		root := project(t, map[string]string{"a.ts": "export interface A {}\n"})
		f := frontend.New()
		if err := f.SetOptions(opt.New(f.OptionsSchema(), map[string]string{"dir": root})); err != nil {
			t.Fatalf("SetOptions: %v", err)
		}
		err := f.Load(&plugin.FrontendContext{
			Store: store.New(), Diag: diag.New(),
			Registry: directive.NewRegistry(), Parser: directive.DefaultParser(),
			Cache: cache.NewNone(), Pattern: "./nonexistent/...",
		})
		if err == nil {
			t.Fatal("a pattern matching nothing was accepted")
		}
	})

	t.Run("the package import view is deduplicated across files", func(t *testing.T) {
		t.Parallel()
		// node.Package.Imports is documented as the deduplicated union
		// over the package's files, and ImportByPath answers with one
		// entry per path. Per-file imports keep their own copies.
		root := project(t, map[string]string{
			"a.ts": "import { X } from './shared';\nexport interface A { x: X }\n",
			"b.ts": "import { X } from './shared';\nexport interface B { x: X }\n",
			"c.ts": "import { Y } from './other';\nexport interface C { y: Y }\n",
		})
		st, _ := run(t, root, "./...", cache.NewNone())
		p := packages(st)[0]

		if got := len(p.Imports); got != 2 {
			paths := make([]string, 0, got)
			for _, i := range p.Imports {
				paths = append(paths, i.Path)
			}
			t.Fatalf("package imports = %v, want one entry per distinct module", paths)
		}
		if p.ImportByPath("./shared") == nil || p.ImportByPath("./other") == nil {
			t.Fatal("ImportByPath cannot reach a deduplicated entry")
		}

		perFile := 0
		for _, f := range p.Files {
			perFile += len(f.Imports)
		}
		if perFile != 3 {
			t.Fatalf("per-file imports = %d, want 3 — dedup must not touch File.Imports", perFile)
		}
	})

	t.Run("tsx and declaration files contribute declarations", func(t *testing.T) {
		t.Parallel()
		root := project(t, map[string]string{
			"c.tsx":  "export interface Props { title: string }\nexport const V = (p: Props) => <div>{p.title}</div>;\n",
			"d.d.ts": "export declare const version: string;\n",
		})
		st, sink := run(t, root, "./...", cache.NewNone())
		if errs := errorsIn(sink); len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		names := declaredNames(st)
		for _, p := range packages(st) {
			for _, c := range p.Constants {
				names[c.Name] = true
			}
		}
		for _, want := range []string{"Props", "V", "version"} {
			if !names[want] {
				t.Errorf("%s missing; have %v", want, slices.Sorted(maps.Keys(names)))
			}
		}
	})

	t.Run("declaration files can be excluded by option", func(t *testing.T) {
		t.Parallel()
		root := project(t, map[string]string{
			"a.ts":   "export interface Kept {}\n",
			"b.d.ts": "export declare const dropped: string;\n",
		})
		f := frontend.New()
		if err := f.SetOptions(opt.New(f.OptionsSchema(), map[string]string{
			"dir":                  root,
			"include_declarations": "false",
		})); err != nil {
			t.Fatalf("SetOptions: %v", err)
		}
		st := store.New()
		if err := f.Load(&plugin.FrontendContext{
			Store: st, Diag: diag.New(),
			Registry: directive.NewRegistry(), Parser: directive.DefaultParser(),
			Cache: cache.NewNone(), Pattern: "./...", Fingerprint: "t",
		}); err != nil {
			t.Fatalf("Load: %v", err)
		}
		for _, p := range packages(st) {
			for _, c := range p.Constants {
				if c.Name == "dropped" {
					t.Fatal("a .d.ts declaration survived include_declarations=false")
				}
			}
		}
	})

	t.Run("a second load hits the cache and produces the same graph", func(t *testing.T) {
		t.Parallel()
		root := project(t, map[string]string{
			"a.ts": "/** Docs. */\nexport interface A { x?: string; }\n",
		})
		c := cache.NewDisk(t.TempDir())

		first, _ := run(t, root, "./...", c)
		second, _ := run(t, root, "./...", c)

		a, b := packages(first)[0], packages(second)[0]
		if len(a.Interfaces) != 1 || len(b.Interfaces) != 1 {
			t.Fatalf("interfaces: first=%d second=%d", len(a.Interfaces), len(b.Interfaces))
		}
		if a.Interfaces[0].Name != b.Interfaces[0].Name {
			t.Fatalf("names differ: %q vs %q", a.Interfaces[0].Name, b.Interfaces[0].Name)
		}
		// Owner back-pointers are dropped by the cache's JSON round
		// trip and rebuilt on read; a cached graph missing them looks
		// correct until something walks upward. An interface's fields
		// are the newest arm in RewireOwners, so this is the case that
		// would go unnoticed.
		if b.Interfaces[0].Fields[0].Owner == nil {
			t.Error("cached graph came back with a nil field Owner")
		}
		if opt, _ := typescript.MetaOptional.Get(b.Interfaces[0].Fields[0].Meta()); !opt {
			t.Error("metadata did not survive the cache round trip")
		}
	})
}

// errorsIn returns the Error-severity diagnostics in s.
func errorsIn(s *diag.Sink) []diag.Diag {
	var out []diag.Diag
	for _, d := range s.Diagnostics() {
		if d.Severity == diag.Error {
			out = append(out, d)
		}
	}
	return out
}

// warningsIn returns the Warn-severity diagnostics in s.
func warningsIn(s *diag.Sink) []diag.Diag {
	var out []diag.Diag
	for _, d := range s.Diagnostics() {
		if d.Severity == diag.Warn {
			out = append(out, d)
		}
	}
	return out
}
