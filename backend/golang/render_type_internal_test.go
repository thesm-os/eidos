// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package-internal benchmarks for the type-expression renderer.
//
// [renderState.renderType] is unexported and reachable from outside
// the package only through [Backend.Render], where template
// execution, gofmt, and the goimports pass together cost three
// orders of magnitude more. Measuring it at all therefore requires
// the internal test package; the behavioural coverage stays
// blackbox in render_type_test.go.
package golang

import (
	"strconv"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/writer"
)

// BenchmarkRenderType measures one type-expression render per
// operation, one Ref kind per sub-benchmark.
//
// renderType is the single hottest function in the backend by call
// count: every field, parameter, return, type argument, and
// constraint term in every generated file routes through it, so a
// per-call regression multiplies by the size of the emit graph
// rather than by the number of files. The existing
// BenchmarkBackend_Render cannot see that — a renderType that got
// twice as slow moves the whole-pass number by noise.
//
// Deliberately outside the timed region: [loadTemplates] and the
// per-Target [template.Template.Clone] that [newRenderState]
// performs. Neither is reachable from renderType — the state here is
// built directly with only the [writer.ImportSet] the function
// actually touches, so the numbers carry no template-tree cost at
// all.
//
// The import set is reused across iterations rather than rebuilt.
// [writer.ImportSet.Imp] is idempotent per path and every
// sub-benchmark uses a fixed path set, so the set reaches its final
// size on the first iteration and the reported allocations are
// renderType's own rather than the fixture's growth.
//
// A zero-alloc report is the correct answer for the "builtin" and
// "internal same-package" cases: both return a stored string
// unchanged, and Go's runtime returns the original backing array
// when concatenating with an empty string. Every other case builds
// a new string and must report non-zero allocations; a zero there
// means the loop body was eliminated.
func BenchmarkRenderType(b *testing.B) {
	b.ReportAllocs()

	// A same-package target carries no resolved ImportPath, so the
	// TypeRef branch returns the bare name; the cross-package twin
	// carries one and therefore pays for an Imp call plus the alias
	// qualification.
	samePackage := &emit.Struct{Name: "Inner", Package: "example.com/bench"}
	crossPackage := &emit.Struct{
		Name:    "SearcherMock",
		Package: "example.com/bench/store",
		Target: emit.Target{
			Dir: "bench/store", Filename: "store_mock.go",
			Package: "store", ImportPath: "example.com/bench/store",
		},
	}

	cases := []struct {
		name string
		ref  emit.Ref
	}{
		{"builtin", emit.Builtin("string")},
		{"external stdlib", emit.External("context", "Context")},
		{"external dot-joined name", emit.External("github.com/example/users", "User.Profile")},
		{"internal same-package", emit.Internal(samePackage)},
		{"internal cross-package", emit.Internal(crossPackage)},
		{"composite pointer", emit.Ptr(emit.Builtin("int"))},
		{"composite slice", emit.SliceOf(emit.Builtin("byte"))},
		{"composite array", emit.ArrayOf(emit.Builtin("byte"), 16)},
		{"composite map", emit.MapOf(emit.Builtin("string"), emit.Builtin("int"))},
		{
			name: "composite func",
			ref: emit.FuncOf(
				[]emit.Ref{emit.Builtin("int"), emit.Builtin("string")},
				[]emit.Ref{emit.Builtin("int"), emit.Builtin("error")},
			),
		},
		{
			name: "composite union",
			ref: emit.Union(
				emit.UnionTerm{Type: emit.Builtin("int"), Approx: true},
				emit.UnionTerm{Type: emit.Builtin("float64"), Approx: true},
				emit.UnionTerm{Type: emit.Builtin("string")},
			),
		},
		{
			name: "composite nested map of slice of pointer",
			ref: emit.MapOf(
				emit.Builtin("string"),
				emit.SliceOf(emit.Ptr(emit.External("github.com/example/users", "User"))),
			),
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			s := benchRenderState()
			var got string
			for b.Loop() {
				var err error
				got, err = s.renderType(tc.ref)
				if err != nil {
					b.Fatalf("renderType: %v", err)
				}
			}
			if got == "" {
				b.Fatalf("renderType produced no spelling for %s", tc.name)
			}
		})
	}
}

// BenchmarkRenderType_Depth measures one render of a pointer chain
// per operation, over chains of increasing depth.
//
// Composite refs recurse: each level concatenates its own prefix
// onto the fully rendered element below it, so a chain of depth n
// copies O(n²) bytes in total. Nesting that deep does not come from
// hand-written Go, but it does come from generated emit graphs
// where a plugin wraps a type once per capability, and the whole
// chain renders on every field that uses it.
//
// The scaling axis is what makes the number readable: linear growth
// in the reported ns/op would mean the concatenation is being
// absorbed somewhere, and superlinear growth quantifies exactly how
// much depth the renderer tolerates before the quadratic term
// dominates.
//
// Chain construction is hoisted per size, outside the timed region;
// only the render is measured.
func BenchmarkRenderType_Depth(b *testing.B) {
	b.ReportAllocs()

	for _, depth := range []int{1, 10, 100, 1000} {
		b.Run(strconv.Itoa(depth), func(b *testing.B) {
			b.ReportAllocs()
			s := benchRenderState()
			ref := pointerChain(depth)
			var got string
			for b.Loop() {
				var err error
				got, err = s.renderType(ref)
				if err != nil {
					b.Fatalf("renderType: %v", err)
				}
			}
			if len(got) != depth+len("int") {
				b.Fatalf("depth %d rendered %d chars, want %d", depth, len(got), depth+len("int"))
			}
		})
	}
}

// benchRenderState builds the minimal [renderState] renderType
// reads: an import set for the External and cross-package TypeRef
// branches. The template tree is deliberately absent — renderType
// and everything it dispatches to render into strings without ever
// executing a template, so cloning one would only add setup cost
// the benchmark then has to explain away.
func benchRenderState() *renderState {
	return &renderState{imports: writer.NewImportSet(nil)}
}

// pointerChain returns a Ref nesting depth pointer levels over a
// builtin `int`, so the rendered spelling is depth asterisks
// followed by "int". Used by [BenchmarkRenderType_Depth] to vary
// recursion depth without varying anything else about the ref.
func pointerChain(depth int) emit.Ref {
	var ref emit.Ref = emit.Builtin("int")
	for range depth {
		ref = emit.Ptr(ref)
	}
	return ref
}
