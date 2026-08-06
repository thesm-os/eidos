// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/store"
)

// TestRenderState_ResetBetweenTargets covers the state a worker
// carries from one file to the next.
//
// One renderState renders several targets in sequence, so everything
// per-file on it has to be cleared between them. The import set is
// now cleared in place rather than reallocated, which makes that
// clearing a property of Reset rather than of object identity — and
// same-package elision is the part that fails silently: a stale self
// makes the next file render bare, unqualified names for a package it
// does not live in.
func TestRenderState_ResetBetweenTargets(t *testing.T) {
	t.Parallel()

	s := newRenderState(loadTemplates(), nil, nil, nil)

	// First file lives in package A and references B.
	s.imports.Reset()
	s.imports.SetSelf("example.com/a", "a")
	if _, err := s.renderType(emit.External("example.com/b", "Thing")); err != nil {
		t.Fatalf("renderType: %v", err)
	}
	if _, err := s.renderType(emit.External("example.com/a", "Local")); err != nil {
		t.Fatalf("renderType: %v", err)
	}

	// Second file lives in package B and references A.
	s.imports.Reset()
	s.imports.SetSelf("example.com/b", "b")
	fromA, err := s.renderType(emit.External("example.com/a", "Local"))
	if err != nil {
		t.Fatalf("renderType: %v", err)
	}
	fromB, err := s.renderType(emit.External("example.com/b", "Thing"))
	if err != nil {
		t.Fatalf("renderType: %v", err)
	}

	t.Run("the second file elides its own package", func(t *testing.T) {
		t.Parallel()
		if fromB != "Thing" {
			t.Fatalf("own-package reference = %q, want the bare name", fromB)
		}
	})

	t.Run("the second file qualifies the first file's package", func(t *testing.T) {
		t.Parallel()
		// A stale self from the previous file would elide this one
		// instead, emitting `Local` against no import at all.
		if fromA != "a.Local" {
			t.Fatalf("cross-package reference = %q, want %q", fromA, "a.Local")
		}
	})

	t.Run("the second file's import block holds only what it used", func(t *testing.T) {
		t.Parallel()
		got := s.imports.Imports()
		if len(got) != 1 || got[0].Path != "example.com/a" {
			t.Fatalf("import set = %+v, want only example.com/a", got)
		}
	})
}

// TestRenderState_BufferStackIsReentrant covers the free list render
// draws from.
//
// render is re-entrant through the funcmap: `{{ render . }}` fires
// from inside an in-flight template, so a nested call must not write
// into its parent's buffer. A single scratch field would silently
// interleave the two. This drives the nesting directly rather than
// through a template, so it fails on the mechanism rather than on
// whichever fixture happens to nest.
func TestRenderState_BufferStackIsReentrant(t *testing.T) {
	t.Parallel()

	s := newRenderState(loadTemplates(), nil, nil, nil)

	t.Run("a nested take does not alias the outer buffer", func(t *testing.T) {
		t.Parallel()
		outer := s.takeBuffer()
		outer.WriteString("outer")
		inner := s.takeBuffer()
		inner.WriteString("inner")
		if outer == inner {
			t.Fatalf("nested take returned the parent's buffer")
		}
		if outer.String() != "outer" {
			t.Fatalf("outer buffer was written through by the nested take: %q", outer.String())
		}
		s.putBuffer(inner)
		s.putBuffer(outer)
	})

	t.Run("a returned buffer is reused empty", func(t *testing.T) {
		t.Parallel()
		st := newRenderState(loadTemplates(), nil, nil, nil)
		first := st.takeBuffer()
		first.WriteString("stale")
		st.putBuffer(first)
		second := st.takeBuffer()
		if second.Len() != 0 {
			t.Fatalf("reused buffer carried %q", second.String())
		}
	})

	t.Run("depth in equals depth out", func(t *testing.T) {
		t.Parallel()
		// The free list is bounded by nesting depth, not by call
		// count. An unbalanced put would grow it for the life of the
		// worker.
		st := newRenderState(loadTemplates(), nil, nil, nil)
		held := make([]*bytes.Buffer, 0, 8)
		for range 8 {
			held = append(held, st.takeBuffer())
		}
		for _, b := range held {
			st.putBuffer(b)
		}
		for range 8 {
			_ = st.takeBuffer()
		}
		if got := len(st.buffers); got != 0 {
			t.Fatalf("free list holds %d after balanced use, want 0", got)
		}
	})
}

// TestRenderInto_MatchesRender pins that writing through and
// materialising a string produce the same bytes.
func TestRenderInto_MatchesRender(t *testing.T) {
	t.Parallel()

	s := newRenderState(loadTemplates(), nil, nil, nil)
	n := &emit.Struct{
		Name: "User", Package: "users",
		Fields: []*emit.Field{{Name: "ID", Type: emit.Builtin("string")}},
	}

	want, err := s.render(n)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var got strings.Builder
	if err := s.renderInto(&got, n); err != nil {
		t.Fatalf("renderInto: %v", err)
	}
	if got.String() != want {
		t.Fatalf("renderInto = %q, render = %q", got.String(), want)
	}
}

// TestCollectSelfPackages_FiltersAgreeingNames pins the hoist that
// removed the render loop's only quadratic.
//
// The divergence test is target-independent — a package's declared
// name either matches its path-derived alias or it does not, whatever
// file is being rendered — so it belongs at collection time. Left in
// the per-target loop it ran for every (target, package) pair and
// continued on all of them: T×P alias derivations to perform,
// normally, zero registrations.
//
// Nothing about generated output changes either way, which is why
// this needs its own test: the writer omits an alias that matches the
// derived one, so putting the filter back would be invisible to every
// other assertion in this package.
func TestCollectSelfPackages_FiltersAgreeingNames(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, targets ...emit.Target) []selfAlias {
		t.Helper()
		s := store.New()
		pkg := &emit.Package{Name: "bench", Path: "example.com/bench"}
		for i, tgt := range targets {
			pkg.Structs = append(pkg.Structs, &emit.Struct{
				Name: "E" + strconv.Itoa(i), Package: tgt.ImportPath, Target: tgt,
			})
		}
		if err := s.Emit().AddPackage(pkg); err != nil {
			t.Fatalf("AddPackage: %v", err)
		}
		s.Emit().RebuildByTarget()
		return collectSelfPackages(s)
	}

	t.Run("a package whose name matches its path is dropped", func(t *testing.T) {
		t.Parallel()
		got := build(t,
			emit.Target{Dir: "users", Filename: "a.go", Package: "users", ImportPath: "example.com/users"},
			emit.Target{Dir: "orders", Filename: "b.go", Package: "orders", ImportPath: "example.com/orders"},
		)
		if len(got) != 0 {
			t.Fatalf("agreeing packages survived the filter: %+v", got)
		}
	})

	t.Run("a renamed package is kept", func(t *testing.T) {
		t.Parallel()
		// The pkg= override shape: directory testkit, package
		// storetest. Without an explicit alias a referring file
		// renders testkit.Thing against an import that bound
		// storetest, and does not compile.
		got := build(t,
			emit.Target{Dir: "testkit", Filename: "a.go", Package: "storetest", ImportPath: "example.com/testkit"},
		)
		if len(got) != 1 || got[0].path != "example.com/testkit" || got[0].pkg != "storetest" {
			t.Fatalf("renamed package not collected: %+v", got)
		}
	})

	t.Run("many targets in one package yield one entry", func(t *testing.T) {
		t.Parallel()
		// The map is kept for deduplication precisely because
		// ByTarget().Keys() yields one entry per target. Building
		// the slice straight from that loop would give one entry per
		// file and turn a constant-time scan into a linear one.
		targets := make([]emit.Target, 0, 20)
		for i := range 20 {
			targets = append(targets, emit.Target{
				Dir: "testkit", Filename: "f" + strconv.Itoa(i) + ".go",
				Package: "storetest", ImportPath: "example.com/testkit",
			})
		}
		if got := build(t, targets...); len(got) != 1 {
			t.Fatalf("expected one entry for one package, got %+v", got)
		}
	})
}
