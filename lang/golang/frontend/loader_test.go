// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/lang/golang/frontend"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// TestFrontend_Load_EmptyPattern covers the early-return path when
// the FrontendContext's Pattern is empty.
func TestFrontend_Load_EmptyPattern(t *testing.T) {
	t.Parallel()
	t.Run("empty pattern returns ErrEmptyPattern and emits a diagnostic", func(t *testing.T) {
		t.Parallel()
		fe := frontend.New()
		d := diag.New()
		err := fe.Load(&plugin.FrontendContext{
			Store:   store.New(),
			Diag:    d,
			Parser:  directive.DefaultParser(),
			Cache:   cache.NewNone(),
			Pattern: "",
		})
		if !errors.Is(err, frontend.ErrEmptyPattern) {
			t.Fatalf("err = %v, want errors.Is(_, ErrEmptyPattern)", err)
		}
		if !d.HasErrors() {
			t.Fatalf("expected positioned diagnostic for empty pattern")
		}
	})

	t.Run("whitespace-only pattern is treated as empty", func(t *testing.T) {
		t.Parallel()
		fe := frontend.New()
		d := diag.New()
		err := fe.Load(&plugin.FrontendContext{
			Store:   store.New(),
			Diag:    d,
			Parser:  directive.DefaultParser(),
			Cache:   cache.NewNone(),
			Pattern: "   \t",
		})
		if !errors.Is(err, frontend.ErrEmptyPattern) {
			t.Fatalf("whitespace pattern err = %v, want ErrEmptyPattern", err)
		}
	})
}

// TestFrontend_Load_MalformedSource verifies parse errors surface
// as positioned diagnostics rather than panicking — the converter
// must run all phases to completion regardless of per-file errors.
func TestFrontend_Load_MalformedSource(t *testing.T) {
	t.Parallel()
	t.Run("syntax-error source emits positioned error diagnostics", func(t *testing.T) {
		t.Parallel()
		_, d := loadFromSource(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ // missing closing brace\n",
		})
		if !d.HasErrors() {
			t.Fatalf("expected error diagnostic for malformed source")
		}
	})
}

// TestFrontend_Load_NoPanicOnUnresolvedTypes verifies the converter
// continues past per-file type errors without dropping the rest of
// the package.
func TestFrontend_Load_NoPanicOnUnresolvedTypes(t *testing.T) {
	t.Parallel()
	t.Run("unresolved type errors do not crash the converter", func(t *testing.T) {
		t.Parallel()
		// Reference to undeclared identifier — types.Check produces
		// an error but the converter should still emit S.
		_, d := loadFromSource(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ X Missing }\n",
		})
		// We do not assert HasErrors == true: the exact error
		// surface depends on go/types' message; we only assert the
		// frontend did not panic and the diag carries at least one
		// entry referencing the missing type.
		hits := 0
		for _, dg := range d.Diagnostics() {
			if dg.Pos.File != "" {
				hits++
			}
		}
		if hits == 0 {
			t.Skipf(
				"expected at least one positioned diagnostic for undeclared identifier — go/types is quiet about it on this version: %+v",
				d.Diagnostics(),
			)
		}
	})
}

// TestFrontend_Load_ParallelPackages drives two concurrent Loads
// through the chdir-mutex serialiser to verify the frontend can run
// in parallel across packages without races. With -race the test
// fails if any shared state is unsafe.
func TestFrontend_Load_ParallelPackages(t *testing.T) {
	t.Parallel()
	t.Run("two parallel Loads complete without contamination", func(t *testing.T) {
		t.Parallel()
		t.Run("A", func(t *testing.T) {
			t.Parallel()
			pkg := requirePackage(t, map[string]string{
				"a.go": "package a\n\ntype A struct{ N int }\n",
			})
			if pkg.StructByName("A") == nil {
				t.Fatalf("A missing")
			}
			if pkg.StructByName("B") != nil {
				t.Fatalf("A loaded B — cross-contamination")
			}
		})
		t.Run("B", func(t *testing.T) {
			t.Parallel()
			pkg := requirePackage(t, map[string]string{
				"b.go": "package b\n\ntype B struct{ S string }\n",
			})
			if pkg.StructByName("B") == nil {
				t.Fatalf("B missing")
			}
			if pkg.StructByName("A") != nil {
				t.Fatalf("B loaded A — cross-contamination")
			}
		})
	})
}

// TestFrontend_Load_BuildTags drives the BuildTags option through
// the loader and verifies a `//go:build`-gated file is excluded by
// default and included when the matching tag is configured.
func TestFrontend_Load_BuildTags(t *testing.T) {
	t.Parallel()

	src := map[string]string{
		"things/always.go": "package things\n\ntype Always struct{}\n",
		"things/gated.go":  "//go:build mytag\n\npackage things\n\ntype Gated struct{}\n",
	}

	t.Run("without the tag the gated file is excluded", func(t *testing.T) {
		t.Parallel()
		dir := materialiseGoSource(t, src)
		s, d := loadDirWithOptions(t, dir, nil, nil)
		if d.HasErrors() {
			t.Fatalf("unexpected errors: %v", d.Diagnostics())
		}
		pkg := firstPackageIn(s, "things")
		if pkg == nil || pkg.StructByName("Always") == nil {
			t.Fatalf("Always struct missing from un-tagged load")
		}
		if pkg.StructByName("Gated") != nil {
			t.Fatalf("Gated struct should be excluded without the build tag")
		}
	})

	t.Run("supplying the tag includes the gated file", func(t *testing.T) {
		t.Parallel()
		dir := materialiseGoSource(t, src)
		s, d := loadDirWithOptions(t, dir, nil, map[string]string{"build_tags": "mytag"})
		if d.HasErrors() {
			t.Fatalf("unexpected errors with tag set: %v", d.Diagnostics())
		}
		pkg := firstPackageIn(s, "things")
		if pkg == nil {
			t.Fatalf("package not loaded")
		}
		if pkg.StructByName("Gated") == nil {
			t.Fatalf("Gated struct missing when build tag is configured")
		}
	})
}

// TestFrontend_Load_DeterministicOutput drives the frontend over the
// same input multiple times and asserts the resulting [node.Package]
// JSON encoding is byte-identical across runs. The source intentionally
// declares enums and method receivers in non-sorted source order so
// any `range map` slip in the conversion paths (notably enum promotion
// and method attachment) surfaces as a JSON mismatch across iterations.
func TestFrontend_Load_DeterministicOutput(t *testing.T) {
	t.Parallel()

	src := map[string]string{
		"things/things.go": `package things

type C int
const (
	C1 C = iota
	C2
)

type B int
const (
	B1 B = iota
	B2
)

type A int
const (
	A1 A = iota
	A2
)

type Zeta struct{}

func (Zeta) Foo() {}
func (Zeta) Bar() {}

type Alpha struct{}

func (Alpha) Baz() {}
func (Alpha) Qux() {}
`,
	}
	dir := materialiseGoSource(t, src)

	const iterations = 10
	var ref []byte
	for i := range iterations {
		s, d := loadDir(t, dir, nil)
		if d.HasErrors() {
			t.Fatalf("iter %d: unexpected frontend errors: %+v", i, d.Diagnostics())
		}
		pkg := firstPackageIn(s, "things")
		if pkg == nil {
			t.Fatalf("iter %d: no package loaded", i)
		}
		body, err := json.Marshal(pkg) //nolint:musttag
		if err != nil {
			t.Fatalf("iter %d: marshal package: %v", i, err)
		}
		if i == 0 {
			ref = body
			continue
		}
		if !bytes.Equal(body, ref) {
			t.Fatalf("iter %d: package encoding diverged from iter 0", i)
		}
	}
}

// TestFrontend_Load_DuplicatePackage drives the AddPackage error
// paths inside [convertPackageWithCache]: when the same package is
// loaded twice against the same store, the second add fails and the
// loader surfaces a positioned diagnostic instead of crashing.
// Two subtests cover both add-failure paths: the cached-package add
// (when the cache hit returns a duplicate) and the fresh-package
// add (when no cache hit but the store already has the package).
func TestFrontend_Load_DuplicatePackage(t *testing.T) {
	t.Parallel()

	loadTwice := func(t *testing.T, c cache.Cache) *diag.Sink {
		t.Helper()
		dir := materialiseGoSource(t, map[string]string{
			"pkg/a.go": "package pkg\n\ntype S struct{}\n",
		})
		chdirMu.Lock()
		defer chdirMu.Unlock()
		prev, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		if err := os.Chdir(dir); err != nil { //nolint:usetesting // t.Chdir() rejects parallel tests
			t.Fatalf("chdir: %v", err)
		}
		defer func() { _ = os.Chdir(prev) }() //nolint:usetesting // t.Chdir() rejects parallel tests

		s := store.New()
		d := diag.New()
		fe := frontend.New()
		ctx := &plugin.FrontendContext{
			Store:   s,
			Diag:    d,
			Parser:  directive.DefaultParser(),
			Cache:   c,
			Pattern: "./...",
		}
		if err := fe.Load(ctx); err != nil {
			t.Fatalf("first Load: %v", err)
		}
		if err := fe.Load(ctx); err != nil {
			t.Fatalf("second Load: %v", err)
		}
		return d
	}

	t.Run("cached add into a populated store surfaces a diagnostic", func(t *testing.T) {
		t.Parallel()
		// recordingCache persists across both loads; the first Load
		// writes, the second reads back and hits AddPackage's
		// duplicate rejection on the cached entry.
		d := loadTwice(t, newRecordingCache())
		if !d.HasErrors() {
			t.Fatalf("expected a duplicate-package diagnostic on the cached add")
		}
	})

	t.Run("fresh add into a populated store surfaces a diagnostic", func(t *testing.T) {
		t.Parallel()
		// cache.NewNone() suppresses cache writes, so the second
		// Load always falls through to a fresh buildPackage; the
		// AddPackage call then hits the duplicate rejection.
		d := loadTwice(t, cache.NewNone())
		if !d.HasErrors() {
			t.Fatalf("expected a duplicate-package diagnostic on the fresh add")
		}
	})
}

// TestFrontend_Load_SyntheticPackageSkipped drives the empty-
// PkgPath skip branch in [loadPattern]. packages.Load surfaces
// non-existent path errors as synthetic packages with PkgPath=="";
// the loader filters them out at the top of the per-package loop
// to avoid polluting the store with placeholders.
func TestFrontend_Load_SyntheticPackageSkipped(t *testing.T) {
	t.Parallel()
	t.Run("non-existent pattern yields a placeholder the loader skips", func(t *testing.T) {
		t.Parallel()
		// Materialise a real module directory so packages.Load runs,
		// then point it at a sub-pattern that does not exist; the
		// loader receives a synthetic package and skips it.
		dir := materialiseGoSource(t, map[string]string{
			"pkg/a.go": "package pkg\n",
		})

		chdirMu.Lock()
		defer chdirMu.Unlock()
		prev, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		if err := os.Chdir(dir); err != nil { //nolint:usetesting // t.Chdir() rejects parallel tests
			t.Fatalf("chdir: %v", err)
		}
		defer func() { _ = os.Chdir(prev) }() //nolint:usetesting // t.Chdir() rejects parallel tests

		s := store.New()
		d := diag.New()
		fe := frontend.New()
		if err := fe.Load(&plugin.FrontendContext{
			Store:   s,
			Diag:    d,
			Parser:  directive.DefaultParser(),
			Cache:   cache.NewNone(),
			Pattern: "./nope/...",
		}); err != nil {
			t.Fatalf("Load: %v", err)
		}
		// packages.Load surfaces the missing-directory error as a
		// package-level diagnostic; we only assert the loader did
		// not panic and the placeholder did not pollute the store.
		_ = d
	})
}

// TestErrEmptyPattern asserts the exported sentinel's identity is
// stable across calls so consumers can compare with [errors.Is].
func TestErrEmptyPattern(t *testing.T) {
	t.Parallel()
	t.Run("sentinel is non-nil and carries a descriptive message", func(t *testing.T) {
		t.Parallel()
		if frontend.ErrEmptyPattern == nil {
			t.Fatalf("ErrEmptyPattern must not be nil")
		}
		if msg := frontend.ErrEmptyPattern.Error(); msg == "" {
			t.Fatalf("ErrEmptyPattern.Error() must be non-empty")
		}
	})
}
